// Package session persists a conversation to disk so it can be resumed.
//
// One session per project, stored at ./.sigma/session.json.
package session

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/tacoda/sigma/internal/anthropic"
)

func path() string { return filepath.Join(".sigma", "session.json") }

// Exists reports whether a saved session is present.
func Exists() bool {
	_, err := os.Stat(path())
	return err == nil
}

// Save writes the conversation history.
func Save(messages []anthropic.Message) error {
	if err := os.MkdirAll(filepath.Dir(path()), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	return os.WriteFile(path(), data, 0o644)
}

// Load reads the saved conversation history.
func Load() ([]anthropic.Message, error) {
	data, err := os.ReadFile(path())
	if err != nil {
		return nil, err
	}
	var messages []anthropic.Message
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}
