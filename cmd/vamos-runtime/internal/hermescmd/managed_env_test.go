package hermescmd

import (
	"slices"
	"strings"
	"testing"
)

func TestManagedEnvDenyWinsAndAddsOnlyExplicitAuthority(t *testing.T) {
	t.Parallel()

	fd := 9
	base := []string{
		"PATH=/bin",
		"DUPLICATE=base",
		"HERMES_TOKEN=secret",
		"VAMOS_HERMES_ENDPOINT=endpoint",
		"VAMOS_MANAGER_ROUTE=route",
		"VAMOS_MANAGER_WAKE_GATEWAY_URL=url",
		"VAMOS_INTERNAL_CALLBACK_TOKEN=callback",
		"VAMOS_CONFIG=config",
		"VAMOS_HERMES_HANDOFF_FD=77",
		"hermes_token=case-sensitive-lookalike",
	}
	overrides := []string{
		"DUPLICATE=override",
		"HERMES_TOKEN=override-secret",
		"VAMOS_CONFIG=override-config",
		"VAMOS_HERMES_HANDOFF_FD=88",
	}
	environment, err := BuildManagedChildEnvironment(ManagedChildEnvironmentInput{
		Base:            base,
		Overrides:       overrides,
		Managed:         true,
		HermesSessionID: "20260731_153837_5d85bf",
		HandoffFD:       &fd,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := "\n" + strings.Join(environment, "\n") + "\n"
	for _, forbidden := range []string{
		"HERMES_TOKEN=",
		"VAMOS_HERMES_ENDPOINT=",
		"VAMOS_MANAGER_ROUTE=",
		"VAMOS_MANAGER_WAKE_GATEWAY_URL=",
		"VAMOS_INTERNAL_CALLBACK_TOKEN=",
		"VAMOS_CONFIG=",
		"VAMOS_HERMES_HANDOFF_FD=77",
		"VAMOS_HERMES_HANDOFF_FD=88",
	} {
		if strings.Contains(joined, "\n"+forbidden) {
			t.Errorf("managed environment retained %q", forbidden)
		}
	}
	for _, wanted := range []string{
		"DUPLICATE=override",
		"HERMES_SESSION_ID=20260731_153837_5d85bf",
		"VAMOS_HERMES_HANDOFF_FD=9",
		"hermes_token=case-sensitive-lookalike",
	} {
		if !slices.Contains(environment, wanted) {
			t.Errorf("managed environment missing %q: %v", wanted, environment)
		}
	}
}

func TestManagedEnvEveryDeniedNameAndPrefixIsCaseSensitive(t *testing.T) {
	t.Parallel()

	entries := make(
		[]string,
		0,
		2*len(managedEnvironmentDenyPrefixes)+2*len(managedEnvironmentDenyNames),
	)
	for _, prefix := range managedEnvironmentDenyPrefixes {
		entries = append(
			entries,
			prefix+"TEST=value",
			strings.ToLower(prefix)+"TEST=lookalike",
		)
	}
	for name := range managedEnvironmentDenyNames {
		entries = append(
			entries,
			name+"=value",
			strings.ToLower(name)+"=lookalike",
		)
	}
	environment, err := BuildManagedChildEnvironment(ManagedChildEnvironmentInput{
		Base:            entries,
		Managed:         true,
		HermesSessionID: "opaque-manager-id",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if deniedManagedEnvironmentName(name) && name != "HERMES_SESSION_ID" {
			t.Errorf("denied name survived: %s", name)
		}
	}
	if len(
		environment,
	) != len(
		managedEnvironmentDenyPrefixes,
	)+len(
		managedEnvironmentDenyNames,
	)+1 {
		t.Fatalf("case-sensitive lookalikes were not preserved exactly: %v", environment)
	}
}

func TestManagedEnvUnmanagedRemovesInheritedHandoffFD(t *testing.T) {
	t.Parallel()

	environment, err := BuildManagedChildEnvironment(ManagedChildEnvironmentInput{
		Base: []string{
			"PATH=/bin",
			"VAMOS_HERMES_HANDOFF_FD=12",
			"HERMES_TOKEN=ordinary",
		},
		Overrides: []string{"VAMOS_HERMES_HANDOFF_FD=13"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(environment, []string{"HERMES_TOKEN=ordinary", "PATH=/bin"}) {
		t.Fatalf("unexpected unmanaged environment: %v", environment)
	}
}

func TestManagedEnvRejectsInvalidIdentityDescriptorAndEntry(t *testing.T) {
	t.Parallel()

	fd := 2
	for _, input := range []ManagedChildEnvironmentInput{
		{Managed: true, HermesSessionID: "bad\x00id"},
		{Managed: true, HermesSessionID: "valid", HandoffFD: &fd},
		{Base: []string{"MALFORMED"}},
	} {
		if _, err := BuildManagedChildEnvironment(input); err == nil {
			t.Fatalf("expected rejection for %+v", input)
		}
	}
}
