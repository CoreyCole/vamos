package hermescmd

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/CoreyCole/vamos/pkg/hermes/sessioningress"
)

const (
	managedHandoffFDEnvironment = "VAMOS_HERMES_HANDOFF_FD"
	firstNonstandardDescriptor  = 3
)

var managedEnvironmentDenyPrefixes = []string{
	"HERMES_",
	"VAMOS_HERMES_",
	"VAMOS_MANAGER_",
	"VAMOS_MANAGER_WAKE_",
	"VAMOS_INTERNAL_CALLBACK_",
}

var managedEnvironmentDenyNames = map[string]struct{}{
	"VAMOS_CONFIG":                         {},
	"VAMOS_HERMES_CONFIG":                  {},
	"VAMOS_INTERNAL_CALLBACK_BASE_URL":     {},
	"VAMOS_MANAGER_WAKE_MANAGER_THREAD_ID": {},
	"VAMOS_MANAGER_WAKE_PI_SESSION_ID":     {},
	"VAMOS_MANAGER_WAKE_GATEWAY_URL":       {},
	"VAMOS_MANAGER_WAKE_INGRESS_TOKEN":     {},
}

type ManagedChildEnvironmentInput struct {
	Base            []string
	Overrides       []string
	Managed         bool
	HermesSessionID string
	HandoffFD       *int
}

func BuildManagedChildEnvironment(input ManagedChildEnvironmentInput) ([]string, error) {
	values := make(map[string]string, len(input.Base)+len(input.Overrides))
	for _, entries := range [][]string{input.Base, input.Overrides} {
		for _, entry := range entries {
			name, value, ok := strings.Cut(entry, "=")
			if !ok || name == "" || strings.IndexByte(name, 0) >= 0 ||
				strings.IndexByte(value, 0) >= 0 || strings.Contains(name, "=") {
				return nil, errors.New("invalid child environment entry")
			}
			values[name] = value
		}
	}
	delete(values, managedHandoffFDEnvironment)

	if input.Managed {
		if _, err := sessioningress.ValidateSessionID(input.HermesSessionID); err != nil {
			return nil, fmt.Errorf("validate Hermes session ID: %w", err)
		}
		for name := range values {
			if deniedManagedEnvironmentName(name) {
				delete(values, name)
			}
		}
		values["HERMES_SESSION_ID"] = input.HermesSessionID
		if input.HandoffFD != nil {
			if *input.HandoffFD < firstNonstandardDescriptor {
				return nil, errors.New(
					"handoff descriptor must not be a standard descriptor",
				)
			}
			values[managedHandoffFDEnvironment] = strconv.Itoa(*input.HandoffFD)
		}
	}

	keys := make([]string, 0, len(values))
	for name := range values {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, name := range keys {
		environment = append(environment, name+"="+values[name])
	}

	return environment, nil
}

func deniedManagedEnvironmentName(name string) bool {
	if _, denied := managedEnvironmentDenyNames[name]; denied {
		return true
	}
	for _, prefix := range managedEnvironmentDenyPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}
