package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type session struct {
	Email string `json:"e"`
	Exp   int64  `json:"x"`
}

func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// Sign builds a compact stateless session token: base64(payload) + "." + base64(hmac).
func Sign(email string, secret []byte, ttl time.Duration) (string, error) {
	payload, err := json.Marshal(session{Email: email, Exp: time.Now().Add(ttl).Unix()})
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + mac(body, secret), nil
}

func Verify(token string, secret []byte) (string, error) {
	i := strings.LastIndexByte(token, '.')
	if i < 0 {
		return "", errors.New("malformed session")
	}
	body, sig := token[:i], token[i+1:]
	if !hmac.Equal([]byte(sig), []byte(mac(body, secret))) {
		return "", errors.New("bad signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return "", err
	}
	var s session
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", err
	}
	if time.Now().Unix() > s.Exp {
		return "", errors.New("session expired")
	}
	return s.Email, nil
}

func mac(body string, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
