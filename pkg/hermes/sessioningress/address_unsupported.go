//go:build windows

package sessioningress

func SurfaceSupported() bool { return false }

func CurrentEUID() (int, error) { return 0, ErrSurfaceUnsupported }

func PrepareRuntimeDirectory(string, int) error { return ErrSurfaceUnsupported }

func ValidateRuntimeDirectory(string, int) error { return ErrSurfaceUnsupported }
