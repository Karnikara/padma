package auth

import "testing"

func TestNewAPIKeyRoundsTrip(t *testing.T) {
	plaintext, hash := NewAPIKey()
	if plaintext == "" || hash == "" {
		t.Fatal("NewAPIKey returned an empty value")
	}
	if plaintext == hash {
		t.Fatal("plaintext must not equal its hash")
	}
	if HashAPIKey(plaintext) != hash {
		t.Fatal("HashAPIKey(plaintext) does not match the returned hash")
	}
	if !VerifyAPIKey(plaintext, hash) {
		t.Fatal("VerifyAPIKey rejected the correct key")
	}
	if VerifyAPIKey("wrong-key", hash) {
		t.Fatal("VerifyAPIKey accepted an incorrect key")
	}
}

func TestNewAPIKeyIsUnique(t *testing.T) {
	p1, _ := NewAPIKey()
	p2, _ := NewAPIKey()
	if p1 == p2 {
		t.Fatal("two API keys collided")
	}
}

func TestNewWebhookSecret(t *testing.T) {
	s1 := NewWebhookSecret()
	s2 := NewWebhookSecret()
	if s1 == "" || s2 == "" {
		t.Fatal("empty webhook secret")
	}
	if s1 == s2 {
		t.Fatal("webhook secrets collided")
	}
}
