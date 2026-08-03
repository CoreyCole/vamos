package agentchat

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/CoreyCole/vamos/pkg/safecomponent"
)

type HermesPlanIdentity string

type HermesPromptAuthority struct {
	PrincipalType  string `json:"principal_type"`
	PrincipalValue string `json:"principal_value"`
}

var hermesConversationReferencePattern = regexp.MustCompile(`^vhc1_[0-9a-f]{64}$`)

func ResolveHermesPlanIdentity(
	thoughtsRoot, selectedPlanPath, planHint string,
) (HermesPlanIdentity, string, error) {
	root, err := resolveExistingDirectory(thoughtsRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolve thoughts root: %w", err)
	}
	selected, err := resolvePlanSelection(root, selectedPlanPath, false)
	if err != nil {
		return "", "", fmt.Errorf("resolve selected plan: %w", err)
	}
	if !pathWithinRoot(selected, root) || selected == root {
		return "", "", errors.New("selected plan escapes thoughts root")
	}
	if strings.TrimSpace(planHint) != "" {
		hint, err := resolvePlanSelection(root, planHint, true)
		if err != nil {
			return "", "", fmt.Errorf("resolve plan hint: %w", err)
		}
		selectedInfo, err := os.Stat(selected)
		if err != nil {
			return "", "", err
		}
		hintInfo, err := os.Stat(hint)
		if err != nil {
			return "", "", err
		}
		if !os.SameFile(selectedInfo, hintInfo) {
			return "", "", errors.New("plan hint does not identify selected plan")
		}
	}
	rel, err := filepath.Rel(root, selected)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", errors.New("selected plan is not a thoughts descendant")
	}
	identity := HermesPlanIdentity(norm.NFC.String(filepath.ToSlash(rel)))
	if err := ValidateHermesPlanIdentity(identity); err != nil {
		return "", "", err
	}
	return identity, selected, nil
}

func resolveExistingDirectory(value string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("path is not a directory")
	}
	return filepath.Clean(resolved), nil
}

func resolvePlanSelection(root, value string, allowLegacyPrefix bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("plan path is required")
	}
	if allowLegacyPrefix {
		value = strings.TrimPrefix(filepath.ToSlash(value), "thoughts/")
	}
	candidate := value
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, filepath.FromSlash(candidate))
	}
	resolved, err := resolveExistingDirectory(candidate)
	if err != nil && !norm.NFC.IsNormalString(candidate) {
		resolved, err = resolveExistingDirectory(norm.NFC.String(candidate))
	}
	if err != nil {
		return "", err
	}
	if !pathWithinRoot(resolved, root) {
		return "", errors.New("plan path escapes thoughts root")
	}
	return resolved, nil
}

func ValidateHermesPlanIdentity(identity HermesPlanIdentity) error {
	value := string(identity)
	if value == "" || !utf8.ValidString(value) || !norm.NFC.IsNormalString(value) {
		return errors.New("plan identity must be nonempty canonical UTF-8 NFC")
	}
	if strings.HasPrefix(value, "thoughts/") || strings.HasPrefix(value, "/") ||
		strings.Contains(value, `\`) || filepath.VolumeName(value) != "" ||
		(len(value) >= 2 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':') {
		return errors.New("plan identity is not canonical relative form")
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return errors.New("plan identity contains an invalid segment")
		}
		for _, character := range part {
			if character == 0 || unicode.Is(unicode.Cc, character) {
				return errors.New("plan identity contains a control character")
			}
		}
	}
	return nil
}

func HermesConversationReference(plan HermesPlanIdentity, threadID string) (string, error) {
	if err := ValidateHermesPlanIdentity(plan); err != nil {
		return "", err
	}
	if err := safecomponent.ValidateBounded(threadID); err != nil {
		return "", err
	}
	planBytes := []byte(plan)
	threadBytes := []byte(threadID)
	hash := sha256.New()
	_, _ = hash.Write([]byte("vamos-hermes-conversation-v1\x00"))
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(planBytes)))
	_, _ = hash.Write(length[:])
	_, _ = hash.Write(planBytes)
	binary.BigEndian.PutUint32(length[:], uint32(len(threadBytes)))
	_, _ = hash.Write(length[:])
	_, _ = hash.Write(threadBytes)
	return "vhc1_" + hex.EncodeToString(hash.Sum(nil)), nil
}

func ValidateHermesConversationReference(value string) error {
	if !hermesConversationReferencePattern.MatchString(value) {
		return errors.New("invalid Hermes conversation reference")
	}
	return nil
}

func normalizeHermesAuthorityEmail(value string) string {
	return strings.ToLower(strings.Trim(value, " \t\r\n\v\f"))
}
