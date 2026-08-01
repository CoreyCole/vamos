//go:build !windows

package hermescmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func currentOwnerUID() int { return os.Geteuid() }

func requestGracefulTermination(process ManagedCommand) error {
	return process.Signal(syscall.SIGTERM)
}

func LoadSettlementEvidence(
	sessionDirectory *os.File,
	expected SettlementLoadExpectation,
) (SettlementEvidenceV1, error) {
	return loadSettlementEvidence(sessionDirectory, expected, nil)
}

func loadSettlementEvidence(
	sessionDirectory *os.File,
	expected SettlementLoadExpectation,
	afterRead func(*os.File) error,
) (SettlementEvidenceV1, error) {
	if sessionDirectory == nil {
		return SettlementEvidenceV1{}, errors.New(
			"session directory descriptor is required",
		)
	}
	if err := validateSettlementExpectation(expected); err != nil {
		return SettlementEvidenceV1{}, err
	}
	if err := validateOwnedDirectory(sessionDirectory, expected.OwnerUID); err != nil {
		return SettlementEvidenceV1{}, fmt.Errorf("validate session directory: %w", err)
	}

	sessionFD, err := checkedFileDescriptor(sessionDirectory)
	if err != nil {
		return SettlementEvidenceV1{}, err
	}
	settlementsFD, err := unix.Openat(
		sessionFD,
		"settlements",
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return SettlementEvidenceV1{}, errors.New("open settlements directory")
	}
	//nolint:gosec // Openat returned a nonnegative descriptor representable as uintptr.
	settlements := os.NewFile(uintptr(settlementsFD), "settlements")
	if settlements == nil {
		_ = unix.Close(settlementsFD)

		return SettlementEvidenceV1{}, errors.New("open settlements directory")
	}
	defer settlements.Close()
	if err := validateOwnedDirectory(settlements, expected.OwnerUID); err != nil {
		return SettlementEvidenceV1{}, fmt.Errorf(
			"validate settlements directory: %w",
			err,
		)
	}

	settlementFD, err := unix.Openat(
		settlementsFD,
		expected.Frame.MessageID+".yaml",
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return SettlementEvidenceV1{}, errors.New("open settlement evidence")
	}
	//nolint:gosec // Openat returned a nonnegative descriptor representable as uintptr.
	settlement := os.NewFile(uintptr(settlementFD), "settlement")
	if settlement == nil {
		_ = unix.Close(settlementFD)

		return SettlementEvidenceV1{}, errors.New("open settlement evidence")
	}
	defer settlement.Close()

	before, err := settlement.Stat()
	if err != nil {
		return SettlementEvidenceV1{}, errors.New("inspect settlement evidence")
	}
	if !before.Mode().IsRegular() {
		return SettlementEvidenceV1{}, errors.New(
			"settlement evidence is not a regular file",
		)
	}
	if err := validateFileOwner(before, expected.OwnerUID); err != nil {
		return SettlementEvidenceV1{}, err
	}
	if before.Size() < 1 || before.Size() > MaxSettlementEvidenceBytes {
		return SettlementEvidenceV1{}, errors.New(
			"settlement evidence size is outside bounds",
		)
	}
	data, err := io.ReadAll(io.LimitReader(settlement, MaxSettlementEvidenceBytes+1))
	if err != nil {
		return SettlementEvidenceV1{}, errors.New("read settlement evidence")
	}
	if len(data) < 1 || len(data) > MaxSettlementEvidenceBytes {
		return SettlementEvidenceV1{}, errors.New(
			"settlement evidence size is outside bounds",
		)
	}
	if afterRead != nil {
		if err := afterRead(settlement); err != nil {
			return SettlementEvidenceV1{}, errors.New(
				"exercise settlement load boundary",
			)
		}
	}
	after, err := settlement.Stat()
	if err != nil || !settlementFileUnchanged(before, after) ||
		int64(len(data)) != after.Size() {
		return SettlementEvidenceV1{}, errors.New(
			"settlement evidence changed while loading",
		)
	}

	return parseSettlementEvidence(data, expected)
}

func checkedFileDescriptor(file *os.File) (int, error) {
	descriptor := file.Fd()
	if descriptor > uintptr(^uint(0)>>1) {
		return 0, errors.New("file descriptor is outside platform bounds")
	}

	return int(descriptor), nil
}

func validateOwnedDirectory(directory *os.File, expectedUID int) error {
	info, err := directory.Stat()
	if err != nil {
		return errors.New("inspect directory descriptor")
	}
	if !info.IsDir() {
		return errors.New("descriptor is not a directory")
	}

	return validateFileOwner(info, expectedUID)
}

func validateFileOwner(info os.FileInfo, expectedUID int) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || expectedUID < 0 || uint64(stat.Uid) != uint64(expectedUID) {
		return errors.New("descriptor owner does not match launch owner")
	}

	return nil
}
