package util

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}

	if hash == "secret" {
		t.Fatal("password was stored as plaintext")
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("unexpected hash format: %q", hash)
	}
	if !IsPasswordHash(hash) {
		t.Fatal("hash was not detected")
	}
	if !PasswordMatches(hash, "secret") {
		t.Fatal("hash did not match password")
	}
	if PasswordMatches(hash, "wrong") {
		t.Fatal("hash matched wrong password")
	}
}

func TestPasswordMatchesLegacyPlaintext(t *testing.T) {
	if !PasswordMatches("legacy", "legacy") {
		t.Fatal("legacy plaintext password did not match")
	}
	if PasswordMatches("legacy", "wrong") {
		t.Fatal("legacy plaintext password matched wrong password")
	}
	if IsPasswordHash("legacy") {
		t.Fatal("legacy plaintext password detected as hash")
	}
}
