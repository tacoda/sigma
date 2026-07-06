// Package session persists a conversation to disk so it can be resumed.
//
// One session per project, stored at ./.sigma/session.json. Store is the
// default file-backed adapter.
package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/tacoda/sigma/internal/message"
)

func path() string { return filepath.Join(".sigma", "session.json") }

// Store is a file-backed conversation store.
type Store struct{}

// Load reads the saved conversation. The bool is false when no session exists.
func (Store) Load() ([]message.Message, bool, error) {
	data, err := os.ReadFile(path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var messages []message.Message
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, false, err
	}
	return messages, true, nil
}

// Save writes the conversation history.
func (Store) Save(messages []message.Message) error {
	if err := os.MkdirAll(filepath.Dir(path()), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	return os.WriteFile(path(), data, 0o644)
}
