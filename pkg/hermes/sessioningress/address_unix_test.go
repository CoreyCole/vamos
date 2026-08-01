//go:build !windows

package sessioningress

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type addressFixture struct {
	AddressVectors []struct {
		Base32Token string `json:"base32_token"`
		Basename    string `json:"basename"`
		SessionID   string `json:"session_id"`
		SHA256Hex   string `json:"sha256_hex"`
	} `json:"address_vectors"`
	PathSelectionVectors []struct {
		EUID                       int    `json:"euid"`
		FullPathUTF8Bytes          int    `json:"full_path_utf8_bytes"`
		HermesHome                 string `json:"hermes_home"`
		Name                       string `json:"name"`
		PreferredFullPathUTF8Bytes int    `json:"preferred_full_path_utf8_bytes"`
		SelectedDirectory          string `json:"selected_directory"`
	} `json:"path_selection_vectors"`
}

func loadAddressFixture(t *testing.T) addressFixture {
	t.Helper()
	raw, err := os.ReadFile("../testdata/session_ingress_protocol_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture addressFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func TestNormativeAddressVectors(t *testing.T) {
	fixture := loadAddressFixture(t)
	for _, vector := range fixture.AddressVectors {
		exact, err := ValidateSessionID(vector.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(exact)); got != vector.SHA256Hex {
			t.Fatalf("hash = %s, want %s", got, vector.SHA256Hex)
		}
		basename, err := DeriveSocketBasename(vector.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		if basename != vector.Basename ||
			strings.TrimSuffix(
				strings.TrimPrefix(basename, "v1-"),
				".sock",
			) != vector.Base32Token {
			t.Fatalf("basename = %s, want %s", basename, vector.Basename)
		}
		if len(basename) != 60 || strings.Contains(basename, "=") {
			t.Fatalf("invalid basename shape %q", basename)
		}
	}
}

func TestPathSelectionVectorsUseUTF8Bytes(t *testing.T) {
	fixture := loadAddressFixture(t)
	basename := fixture.AddressVectors[0].Basename
	for _, vector := range fixture.PathSelectionVectors {
		selected, err := SelectRuntimeDirectory(vector.HermesHome, basename, vector.EUID)
		if err != nil {
			t.Fatal(err)
		}
		if selected != vector.SelectedDirectory {
			t.Fatalf(
				"%s selected %q, want %q",
				vector.Name,
				selected,
				vector.SelectedDirectory,
			)
		}
		preferred := filepath.Join(
			vector.HermesHome,
			"run",
			"session-ingress-v1",
			basename,
		)
		wantBytes := vector.FullPathUTF8Bytes
		if wantBytes == 0 {
			wantBytes = vector.PreferredFullPathUTF8Bytes
		}
		if len([]byte(preferred)) != wantBytes {
			t.Fatalf(
				"%s preferred bytes = %d, want %d",
				vector.Name,
				len([]byte(preferred)),
				wantBytes,
			)
		}
	}
}

func TestSessionIDBoundsAndPathValidation(t *testing.T) {
	cases := []struct {
		value string
		valid bool
	}{
		{"", false},
		{"a", true},
		{strings.Repeat("é", 512), true},
		{strings.Repeat("é", 513), false},
		{"a\x00b", false},
		{"a\x1fb", false},
		{"a\x7fb", false},
		{"a\u0085b", false},
	}
	for _, test := range cases {
		_, err := ValidateSessionID(test.value)
		if (err == nil) != test.valid {
			t.Errorf(
				"ValidateSessionID(%q) valid=%v, error=%v",
				test.value,
				test.valid,
				err,
			)
		}
	}
	if _, err := SelectRuntimeDirectory("relative", "v1-test.sock", 501); err == nil {
		t.Fatal("accepted relative HERMES_HOME")
	}
	for _, basename := range []string{"", "../test.sock", "a/b.sock", "bad\x00.sock"} {
		if _, err := SelectRuntimeDirectory("/tmp/home", basename, 501); err == nil {
			t.Fatalf("accepted unsafe basename %q", basename)
		}
	}
	if _, err := SelectRuntimeDirectory(
		"/"+strings.Repeat("x", 200),
		"v1-test.sock",
		-1,
	); err == nil {
		t.Fatal("accepted invalid UID")
	}
}

func TestSecureDirectoryAndRealUnixRoundTrip(t *testing.T) {
	if !SurfaceSupported() {
		t.Fatal("POSIX surface reported unsupported")
	}
	euid, err := CurrentEUID()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("/tmp", "hsi-v1-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	directory := filepath.Join(root, "ingress")
	if err := PrepareRuntimeDirectory(directory, euid); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeDirectory(directory, euid); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeDirectory(directory, euid); err == nil {
		t.Fatal("accepted group/world-accessible runtime directory")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(t.TempDir(), "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeDirectory(symlink, euid); err == nil {
		t.Fatal("accepted symlink runtime directory")
	}
	if err := PrepareRuntimeDirectory(symlink, euid); err == nil {
		t.Fatal("prepared symlink runtime directory")
	}

	socketPath := filepath.Join(directory, "v1-test.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer connection.Close()
		payload, err := ReadFrame(connection)
		if err == nil && string(payload) != "ping" {
			err = fmt.Errorf("payload = %q", payload)
		}
		done <- err
	}()
	connection, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFrame(connection, []byte("ping")); err != nil {
		t.Fatal(err)
	}
	connection.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
