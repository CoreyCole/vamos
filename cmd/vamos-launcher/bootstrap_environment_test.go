package main

import (
	"strings"
	"testing"
)

func TestRuntimeEnvironmentSetsResolvedPackageRoot(t *testing.T) {
	t.Parallel()

	env := runtimeEnvironment(
		[]string{"HOME=/tmp/home", "VAMOS_PACKAGE_ROOT=/stale", "PATH=/bin"},
		"/configured/vamos",
	)
	var packageRoots []string
	for _, value := range env {
		if strings.HasPrefix(value, "VAMOS_PACKAGE_ROOT=") {
			packageRoots = append(packageRoots, value)
		}
	}
	if len(packageRoots) != 1 ||
		packageRoots[0] != "VAMOS_PACKAGE_ROOT=/configured/vamos" {
		t.Fatalf("package root environment = %#v", packageRoots)
	}
}
