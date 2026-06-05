package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type MachineAPIActor struct {
	CredentialID string
	ActorEmail   string
	AllowedSlugs []string
}

func MachineCredentialFromBearer(r *http.Request) (keyID, secret string, err error) {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(raw, "Bearer ") {
		return "", "", errors.New("machine bearer token required")
	}
	token := strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
	if strings.HasPrefix(token, "vamos_machine_") {
		token = strings.TrimPrefix(token, "vamos_machine_")
	}
	keyID, secret, ok := strings.Cut(token, ".")
	if !ok || strings.TrimSpace(keyID) == "" || strings.TrimSpace(secret) == "" {
		return "", "", errors.New("invalid machine bearer token")
	}
	return strings.TrimSpace(keyID), strings.TrimSpace(secret), nil
}

func AuthenticateMachineAPIRequest(ctx context.Context, r *http.Request, store MachineCredentialStore) (MachineAPIActor, error) {
	if store == nil {
		return MachineAPIActor{}, errors.New("machine credentials unavailable")
	}
	keyID, secret, err := MachineCredentialFromBearer(r)
	if err != nil {
		return MachineAPIActor{}, err
	}
	credential, err := store.Authenticate(ctx, keyID, secret)
	if err != nil {
		return MachineAPIActor{}, errors.New("invalid machine credential")
	}
	email := strings.TrimSpace(credential.DefaultActorEmail)
	if email == "" {
		return MachineAPIActor{}, errors.New("machine credential actor email is required")
	}
	return MachineAPIActor{
		CredentialID: credential.ID,
		ActorEmail:   email,
		AllowedSlugs: append([]string(nil), credential.AllowedSlugs...),
	}, nil
}
