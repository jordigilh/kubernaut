package auth

import (
	"strings"
	"testing"
)

func TestUserIdentity_String_NilReceiver(t *testing.T) {
	var u *UserIdentity
	if u.String() != "<nil>" {
		t.Errorf("expected <nil>, got %q", u.String())
	}
}

func TestUserIdentity_String_RedactsToken(t *testing.T) {
	u := &UserIdentity{
		Username: "alice",
		Groups:   []string{"admin"},
		Issuer:   "https://idp.example.com",
		RawToken: "super-secret-token",
	}
	s := u.String()
	if strings.Contains(s, "super-secret-token") {
		t.Error("String() must not contain the raw token")
	}
	if !strings.Contains(s, "REDACTED") {
		t.Error("String() should contain REDACTED placeholder")
	}
	if !strings.Contains(s, "alice") {
		t.Error("String() should contain the username")
	}
}
