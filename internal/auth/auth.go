// Package auth loads Claude Code subscription OAuth credentials.
//
// Sigma reuses the credential Claude Code already stores, so no API key is
// required. On macOS the credential lives in the login Keychain under service
// "Claude Code-credentials"; the file fallback is ~/.claude/.credentials.json.
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const keychainService = "Claude Code-credentials"

// Credentials is the OAuth material Claude Code persists.
type Credentials struct {
	AccessToken      string   `json:"accessToken"`
	RefreshToken     string   `json:"refreshToken"`
	ExpiresAt        int64    `json:"expiresAt"` // epoch milliseconds
	Scopes           []string `json:"scopes"`
	SubscriptionType string   `json:"subscriptionType"`
}

type credentialFile struct {
	ClaudeAiOauth Credentials `json:"claudeAiOauth"`
}

// Load reads credentials from the Keychain, falling back to the creds file.
func Load() (*Credentials, error) {
	if c, err := loadKeychain(); err == nil {
		return c, nil
	}
	return loadFile()
}

// Expired reports whether the access token is past (or within a minute of) its
// expiry. Refresh is deferred (Phase 9); callers surface a re-auth message.
func (c *Credentials) Expired() bool {
	if c.ExpiresAt == 0 {
		return false
	}
	exp := time.UnixMilli(c.ExpiresAt)
	return time.Now().Add(time.Minute).After(exp)
}

func loadKeychain() (*Credentials, error) {
	out, err := exec.Command("security", "find-generic-password",
		"-s", keychainService, "-w").Output()
	if err != nil {
		return nil, fmt.Errorf("keychain read: %w", err)
	}
	return parse(out)
}

func loadFile() (*Credentials, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".claude", ".credentials.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parse(data)
}

func parse(data []byte) (*Credentials, error) {
	var f credentialFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	c := f.ClaudeAiOauth
	if c.AccessToken == "" {
		return nil, fmt.Errorf("no access token in credentials")
	}
	return &c, nil
}
