package appletruntime

import (
	"fmt"
	"net"
)

func allocateLocalPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocate applet port: %w", err)
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("allocate applet port: unexpected address %q", ln.Addr())
	}
	return addr.Port, nil
}
