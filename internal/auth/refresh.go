package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"os/user"
	"time"
)

// These are the public Claude Code OAuth client values. Refreshing rotates the
// refresh token, so the new credential is written back to the same Keychain
// entry Claude Code reads — keeping both tools authenticated.
const (
	tokenURL = "https://console.anthropic.com/v1/oauth/token"
	clientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
)

// EnsureValid refreshes the access token if it has expired, persisting the new
// credential to the Keychain. It is a no-op while the token is still valid.
func EnsureValid(c *Credentials) error {
	if !c.Expired() {
		return nil
	}
	return c.Refresh()
}

// Refresh exchanges the refresh token for a new access token and saves it.
func (c *Credentials) Refresh() error {
	if c.RefreshToken == "" {
		return fmt.Errorf("no refresh token; re-auth via Claude Code")
	}
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": c.RefreshToken,
		"client_id":     clientID,
	})
	req, err := http.NewRequest(http.MethodPost, tokenURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK || r.AccessToken == "" {
		return fmt.Errorf("token refresh failed (status %d): %s", resp.StatusCode, r.Error)
	}

	c.AccessToken = r.AccessToken
	if r.RefreshToken != "" {
		c.RefreshToken = r.RefreshToken
	}
	c.ExpiresAt = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second).UnixMilli()
	return c.save()
}

// save writes the credential back to the Keychain entry Claude Code uses.
func (c *Credentials) save() error {
	data, err := json.Marshal(credentialFile{ClaudeAiOauth: *c})
	if err != nil {
		return err
	}
	acct := "default"
	if u, err := user.Current(); err == nil {
		acct = u.Username
	}
	// -U updates the existing item in place.
	cmd := exec.Command("security", "add-generic-password",
		"-U", "-s", keychainService, "-a", acct, "-w", string(data))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("keychain save: %v: %s", err, out)
	}
	return nil
}
