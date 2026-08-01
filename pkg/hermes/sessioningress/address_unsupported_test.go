//go:build windows

package sessioningress

import (
	"errors"
	"testing"
)

func TestWindowsSurfaceUnsupported(t *testing.T) {
	if SurfaceSupported() {
		t.Fatal("native Windows surface reported supported")
	}
	if _, err := CurrentEUID(); !errors.Is(err, ErrSurfaceUnsupported) {
		t.Fatalf("CurrentEUID error = %v", err)
	}
	if err := PrepareRuntimeDirectory(
		`C:\temp`,
		0,
	); !errors.Is(
		err,
		ErrSurfaceUnsupported,
	) {
		t.Fatalf("PrepareRuntimeDirectory error = %v", err)
	}
}
