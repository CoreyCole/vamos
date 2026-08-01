//go:build !windows

package hermescmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func settlementExpectation() SettlementLoadExpectation {
	return SettlementLoadExpectation{
		HermesSessionID: "opaque-hermes-session",
		Frame:           validHandoffFrame(),
		OwnerUID:        os.Geteuid(),
	}
}

func settlementYAML(hermesID, piID, messageID, raw string) []byte {
	return []byte(fmt.Sprintf(
		"outcome: complete\nnext: implement\ncomplete: true\nversion: 1\nhermes_session_id: %q\npi_session_id: %q\nmessage_id: %q\nraw_response: %q\n",
		hermesID,
		piID,
		messageID,
		raw,
	))
}

func createSettlementFixture(t *testing.T, data []byte) (*os.File, string, string) {
	t.Helper()
	sessionPath := filepath.Join(t.TempDir(), "session")
	settlementsPath := filepath.Join(sessionPath, "settlements")
	if err := os.MkdirAll(settlementsPath, 0o700); err != nil {
		t.Fatal(err)
	}
	expected := settlementExpectation()
	filePath := filepath.Join(settlementsPath, expected.Frame.MessageID+".yaml")
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { directory.Close() })

	return directory, sessionPath, filePath
}

func TestSettlementLoaderReadsExactDescriptorBoundEvidence(t *testing.T) {
	t.Parallel()

	expected := settlementExpectation()
	raw := "outcome: handoff\nnext: verify\ncomplete: true\n🌰\n"
	directory, _, _ := createSettlementFixture(t, settlementYAML(
		expected.HermesSessionID,
		expected.Frame.PiSessionID,
		expected.Frame.MessageID,
		raw,
	))
	evidence, err := LoadSettlementEvidence(directory, expected)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.HermesSessionID != expected.HermesSessionID ||
		evidence.PiSessionID != expected.Frame.PiSessionID ||
		evidence.MessageID != expected.Frame.MessageID || evidence.RawResponse != raw {
		t.Fatalf("unexpected evidence: %+v", evidence)
	}
}

func TestSettlementLoaderRejectsEveryIdentityMismatch(t *testing.T) {
	t.Parallel()

	expected := settlementExpectation()
	cases := []struct{ hermes, pi, message string }{
		{"other-hermes", expected.Frame.PiSessionID, expected.Frame.MessageID},
		{expected.HermesSessionID, "other-pi", expected.Frame.MessageID},
		{expected.HermesSessionID, expected.Frame.PiSessionID, "other-message"},
	}
	for _, testCase := range cases {
		directory, _, _ := createSettlementFixture(t, settlementYAML(
			testCase.hermes, testCase.pi, testCase.message, "raw",
		))
		if _, err := LoadSettlementEvidence(directory, expected); err == nil {
			t.Fatalf("identity mismatch accepted: %+v", testCase)
		}
	}
}

func TestSettlementLoaderRejectsTraversalSymlinksTypesOwnerSizeAndMutation(t *testing.T) {
	t.Parallel()

	expected := settlementExpectation()
	valid := settlementYAML(
		expected.HermesSessionID,
		expected.Frame.PiSessionID,
		expected.Frame.MessageID,
		"raw",
	)

	t.Run("traversal", func(t *testing.T) {
		t.Parallel()

		directory, _, _ := createSettlementFixture(t, valid)
		bad := expected
		bad.Frame.MessageID = "../escape"
		if _, err := LoadSettlementEvidence(directory, bad); err == nil {
			t.Fatal("traversal identity was accepted")
		}
	})
	t.Run("file symlink", func(t *testing.T) {
		t.Parallel()

		directory, _, path := createSettlementFixture(t, valid)
		target := path + ".target"
		if err := os.Rename(path, target); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSettlementEvidence(directory, expected); err == nil {
			t.Fatal("settlement symlink was accepted")
		}
	})
	t.Run("directory symlink replacement", func(t *testing.T) {
		t.Parallel()

		directory, sessionPath, _ := createSettlementFixture(t, valid)
		settlements := filepath.Join(sessionPath, "settlements")
		if err := os.Rename(settlements, settlements+"-old"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(settlements+"-old", settlements); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSettlementEvidence(directory, expected); err == nil {
			t.Fatal("replacement directory symlink was accepted")
		}
	})
	t.Run("non regular", func(t *testing.T) {
		t.Parallel()

		directory, _, path := createSettlementFixture(t, valid)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := unix.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSettlementEvidence(directory, expected); err == nil {
			t.Fatal("FIFO was accepted")
		}
	})
	t.Run("owner", func(t *testing.T) {
		t.Parallel()

		directory, _, _ := createSettlementFixture(t, valid)
		bad := expected
		bad.OwnerUID++
		if _, err := LoadSettlementEvidence(directory, bad); err == nil {
			t.Fatal("owner mismatch was accepted")
		}
	})
	t.Run("oversized", func(t *testing.T) {
		t.Parallel()

		directory, _, path := createSettlementFixture(t, valid)
		if err := os.WriteFile(
			path,
			make([]byte, MaxSettlementEvidenceBytes+1),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSettlementEvidence(directory, expected); err == nil {
			t.Fatal("oversized settlement was accepted")
		}
	})
	t.Run("mutation", func(t *testing.T) {
		t.Parallel()

		directory, _, _ := createSettlementFixture(t, valid)
		_, err := loadSettlementEvidence(directory, expected, func(file *os.File) error {
			return file.Chmod(0o640)
		})
		if err == nil {
			t.Fatal("settlement mutation was accepted")
		}
	})
}

func TestSettlementLoaderStaysOnOpenedSessionDirectory(t *testing.T) {
	t.Parallel()

	expected := settlementExpectation()
	valid := settlementYAML(
		expected.HermesSessionID,
		expected.Frame.PiSessionID,
		expected.Frame.MessageID,
		"trusted",
	)
	directory, sessionPath, _ := createSettlementFixture(t, valid)
	moved := sessionPath + "-moved"
	if err := os.Rename(sessionPath, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sessionPath, "settlements"), 0o700); err != nil {
		t.Fatal(err)
	}
	malicious := filepath.Join(
		sessionPath,
		"settlements",
		expected.Frame.MessageID+".yaml",
	)
	if err := os.WriteFile(malicious, settlementYAML(
		expected.HermesSessionID,
		expected.Frame.PiSessionID,
		expected.Frame.MessageID,
		"replacement",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	evidence, err := LoadSettlementEvidence(directory, expected)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.RawResponse != "trusted" {
		t.Fatalf("loader followed replacement path: %q", evidence.RawResponse)
	}
}

func TestSettlementLoaderRejectsMalformedOrAmbiguousYAML(t *testing.T) {
	t.Parallel()

	for _, data := range [][]byte{
		[]byte("version: 1\nversion: 1\n"),
		[]byte("version: 1\nraw_response: &x value\ncopy: *x\n"),
		[]byte("- version: 1\n"),
		[]byte("version: true\n"),
	} {
		directory, _, _ := createSettlementFixture(t, data)
		if _, err := LoadSettlementEvidence(
			directory,
			settlementExpectation(),
		); err == nil {
			t.Fatalf("malformed settlement accepted: %q", data)
		}
	}
}
