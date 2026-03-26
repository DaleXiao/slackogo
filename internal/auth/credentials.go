package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Credentials struct {
	Token     string `json:"token"`      // xoxc-... token
	Cookie    string `json:"cookie"`     // d cookie value
	Workspace string `json:"workspace"`  // workspace domain (e.g. "myteam")
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "slackogo")
	return dir, os.MkdirAll(dir, 0700)
}

func credentialsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

func SaveCredentials(creds []Credentials) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func LoadCredentials() ([]Credentials, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var creds []Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("invalid credentials file: %w", err)
	}
	return creds, nil
}

func FindCredentials(workspace string) (*Credentials, error) {
	creds, err := LoadCredentials()
	if err != nil {
		return nil, err
	}
	if len(creds) == 0 {
		return nil, fmt.Errorf("no credentials configured. Run 'slackogo auth manual' or 'slackogo auth import'")
	}
	var cred *Credentials
	if workspace == "" {
		cred = &creds[0]
	} else {
		for i, c := range creds {
			if c.Workspace == workspace {
				cred = &creds[i]
				break
			}
		}
	}
	if cred == nil {
		return nil, fmt.Errorf("no credentials for workspace %q", workspace)
	}
	// Guard: refuse to use empty token — prevents silent auth failures,
	// retry loops, and potential F2A lockouts from repeated bad requests.
	if cred.Token == "" {
		return nil, fmt.Errorf("auth error: token is empty for workspace %q. "+
			"Run 'slackogo auth import' or 'slackogo auth manual' to set a valid xoxc- token", cred.Workspace)
	}
	return cred, nil
}

// AddOrUpdateCredentials upserts credentials by workspace
func AddOrUpdateCredentials(cred Credentials) error {
	creds, err := LoadCredentials()
	if err != nil {
		creds = nil
	}
	found := false
	for i, c := range creds {
		if c.Workspace == cred.Workspace {
			creds[i] = cred
			found = true
			break
		}
	}
	if !found {
		creds = append(creds, cred)
	}
	return SaveCredentials(creds)
}
