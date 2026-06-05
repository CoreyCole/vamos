package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMachineCredentialFromBearer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		header     string
		wantKeyID  string
		wantSecret string
		wantErr    bool
	}{
		{name: "prefixed token", header: "Bearer vamos_machine_mc_123.secret", wantKeyID: "mc_123", wantSecret: "secret"},
		{name: "plain token", header: "Bearer mc_123.secret", wantKeyID: "mc_123", wantSecret: "secret"},
		{name: "missing bearer", header: "vamos_machine_mc_123.secret", wantErr: true},
		{name: "missing dot", header: "Bearer vamos_machine_mc_123", wantErr: true},
		{name: "blank key", header: "Bearer .secret", wantErr: true},
		{name: "blank secret", header: "Bearer mc_123.", wantErr: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			keyID, secret, err := MachineCredentialFromBearer(req)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("MachineCredentialFromBearer() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("MachineCredentialFromBearer() error = %v", err)
			}
			if keyID != tc.wantKeyID || secret != tc.wantSecret {
				t.Fatalf("MachineCredentialFromBearer() = (%q, %q), want (%q, %q)", keyID, secret, tc.wantKeyID, tc.wantSecret)
			}
		})
	}
}

func TestAuthenticateMachineAPIRequest(t *testing.T) {
	t.Parallel()

	store := NewMemoryMachineCredentialStore()
	created, err := store.Create(t.Context(), CreateMachineCredentialInput{
		DefaultActorEmail: "agent@example.test",
		AllowedSlugs:      []string{"stage", "local"},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer vamos_machine_"+created.Credential.ID+"."+created.Secret)
	actor, err := AuthenticateMachineAPIRequest(t.Context(), req, store)
	if err != nil {
		t.Fatalf("AuthenticateMachineAPIRequest() error = %v", err)
	}
	if actor.CredentialID != created.Credential.ID || actor.ActorEmail != "agent@example.test" {
		t.Fatalf("actor = %+v", actor)
	}
	if len(actor.AllowedSlugs) != 2 || actor.AllowedSlugs[0] != "stage" || actor.AllowedSlugs[1] != "local" {
		t.Fatalf("AllowedSlugs = %#v", actor.AllowedSlugs)
	}
}

func TestAuthenticateMachineAPIRequestRequiresDefaultActor(t *testing.T) {
	t.Parallel()

	store := NewMemoryMachineCredentialStore()
	created, err := store.Create(t.Context(), CreateMachineCredentialInput{})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer vamos_machine_"+created.Credential.ID+"."+created.Secret)
	if _, err := AuthenticateMachineAPIRequest(t.Context(), req, store); err == nil {
		t.Fatalf("AuthenticateMachineAPIRequest() error = nil, want missing actor error")
	}
}
