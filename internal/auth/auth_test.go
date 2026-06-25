package auth

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	data := []byte(`{"claudeAiOauth":{"accessToken":"tok","refreshToken":"ref","expiresAt":123,"scopes":["user:inference"],"subscriptionType":"team"}}`)
	c, err := parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if c.AccessToken != "tok" || c.SubscriptionType != "team" {
		t.Fatalf("unexpected creds: %+v", c)
	}
}

func TestParseNoToken(t *testing.T) {
	if _, err := parse([]byte(`{"claudeAiOauth":{}}`)); err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestExpired(t *testing.T) {
	past := &Credentials{ExpiresAt: time.Now().Add(-time.Hour).UnixMilli()}
	future := &Credentials{ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}
	none := &Credentials{}
	if !past.Expired() {
		t.Error("past should be expired")
	}
	if future.Expired() {
		t.Error("future should not be expired")
	}
	if none.Expired() {
		t.Error("zero expiry treated as non-expiring")
	}
}
