package ldap

import (
	"errors"
	"testing"
)

func TestWrapAuthError(t *testing.T) {
	baseErr := errors.New("LDAP auth failed for DN uid=dillon,dc=min,dc=io")
	err := wrapAuthError(baseErr)

	if !IsAuthError(err) {
		t.Fatal("expected wrapped error to be recognized as auth failure")
	}
	if err.Error() != baseErr.Error() {
		t.Fatalf("expected error text %q, got %q", baseErr.Error(), err.Error())
	}
}

func TestWrapAuthErrorNil(t *testing.T) {
	if wrapAuthError(nil) != nil {
		t.Fatal("expected nil input to stay nil")
	}
}

func TestIsAuthErrorNegative(t *testing.T) {
	if IsAuthError(errors.New("ldap unavailable")) {
		t.Fatal("expected plain errors to not be classified as auth failures")
	}
}

func TestIsUserDNNotFoundError(t *testing.T) {
	if isUserDNNotFoundError(nil) {
		t.Fatal("expected nil error to not be detected as user-not-found")
	}
	if !isUserDNNotFoundError(errors.New("user DN not found for: dillon")) {
		t.Fatal("expected lowercase user-not-found error to be detected")
	}
	if !isUserDNNotFoundError(errors.New("User DN not found for: dillon")) {
		t.Fatal("expected legacy uppercase user-not-found error to be detected")
	}
	if isUserDNNotFoundError(errors.New("base DN (dc=min,dc=io) for user DN search does not exist")) {
		t.Fatal("expected infrastructure lookup error to not be detected as user-not-found")
	}
}
