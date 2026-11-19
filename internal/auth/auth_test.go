package auth

import (
	"testing"
	"time"
)

func TestSignVerify(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	tok, err := Sign("a@b.c", secret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	email, err := Verify(tok, secret)
	if err != nil || email != "a@b.c" {
		t.Fatalf("verify: email=%q err=%v", email, err)
	}
	if _, err := Verify(tok, []byte("another-secret-another-secret")); err == nil {
		t.Fatal("token must not verify with a different secret")
	}
	if _, err := Verify(tok+"x", secret); err == nil {
		t.Fatal("tampered token must not verify")
	}
}

func TestPasswordHash(t *testing.T) {
	h, err := HashPassword("rahasia123")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "rahasia123") {
		t.Fatal("correct password rejected")
	}
	if CheckPassword(h, "rahasia124") {
		t.Fatal("wrong password accepted")
	}
}

func TestExpiredSession(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	tok, err := Sign("a@b.c", secret, -time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(tok, secret); err == nil {
		t.Fatal("expired token accepted")
	}
}
