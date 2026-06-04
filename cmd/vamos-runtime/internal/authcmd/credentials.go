package authcmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Profile struct {
	ManagerURL string `json:"manager_url"`
	KeyID      string `json:"key_id"`
}

type credentialFile struct {
	Profiles map[string]storedProfile `json:"profiles"`
}

type storedProfile struct {
	ManagerURL string `json:"manager_url"`
	KeyID      string `json:"key_id"`
	Secret     string `json:"secret"`
}

type CredentialStore interface {
	Save(profileName string, profile Profile, secret string) error
	Load(profileName string) (Profile, string, error)
}

type FileCredentialStore struct {
	Path string
}

func DefaultCredentialPath() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("VAMOS_CONFIG_HOME")); dir != "" {
		return filepath.Join(dir, "credentials.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".vamos", "credentials.json"), nil
}

func (s FileCredentialStore) Save(profileName string, profile Profile, secret string) error {
	profileName = normalizeProfileName(profileName)
	profile.ManagerURL = strings.TrimRight(strings.TrimSpace(profile.ManagerURL), "/")
	profile.KeyID = strings.TrimSpace(profile.KeyID)
	secret = strings.TrimSpace(secret)
	if profile.ManagerURL == "" {
		return errors.New("manager URL is required")
	}
	if profile.KeyID == "" {
		return errors.New("key id is required")
	}
	if secret == "" {
		return errors.New("machine secret is required")
	}

	path, err := s.path()
	if err != nil {
		return err
	}
	file, err := readCredentialFile(path)
	if err != nil {
		return err
	}
	if file.Profiles == nil {
		file.Profiles = map[string]storedProfile{}
	}
	file.Profiles[profileName] = storedProfile{ManagerURL: profile.ManagerURL, KeyID: profile.KeyID, Secret: secret}
	return writeCredentialFile(path, file)
}

func (s FileCredentialStore) Load(profileName string) (Profile, string, error) {
	path, err := s.path()
	if err != nil {
		return Profile{}, "", err
	}
	file, err := readCredentialFile(path)
	if err != nil {
		return Profile{}, "", err
	}
	stored, ok := file.Profiles[normalizeProfileName(profileName)]
	if !ok {
		return Profile{}, "", errors.New("machine credential profile not found; run vamos auth login-machine")
	}
	profile := Profile{ManagerURL: strings.TrimRight(strings.TrimSpace(stored.ManagerURL), "/"), KeyID: strings.TrimSpace(stored.KeyID)}
	secret := strings.TrimSpace(stored.Secret)
	if profile.KeyID == "" || secret == "" {
		return Profile{}, "", errors.New("machine credential profile is incomplete; run vamos auth login-machine")
	}
	return profile, secret, nil
}

func (s FileCredentialStore) path() (string, error) {
	if strings.TrimSpace(s.Path) != "" {
		return s.Path, nil
	}
	return DefaultCredentialPath()
}

func readCredentialFile(path string) (credentialFile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return credentialFile{Profiles: map[string]storedProfile{}}, nil
	}
	if err != nil {
		return credentialFile{}, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return credentialFile{Profiles: map[string]storedProfile{}}, nil
	}
	var file credentialFile
	if err := json.Unmarshal(data, &file); err != nil {
		return credentialFile{}, err
	}
	if file.Profiles == nil {
		file.Profiles = map[string]storedProfile{}
	}
	return file, nil
}

func writeCredentialFile(path string, file credentialFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".credentials-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func normalizeProfileName(profileName string) string {
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return "default"
	}
	return profileName
}
