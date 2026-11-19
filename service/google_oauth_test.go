package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func testGoogleIDToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return "eyJhbGciOiJSUzI1NiJ9." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func validGoogleClaims(now time.Time) map[string]any {
	return map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            "client-1",
		"sub":            "1234567890",
		"exp":            now.Add(time.Hour).Unix(),
		"nonce":          "nonce-1",
		"email":          "alice@example.com",
		"email_verified": true,
		"name":           "Alice",
	}
}

func TestParseGoogleIDTokenAcceptsVerifiedIdentity(t *testing.T) {
	now := time.Now()
	identity, err := ParseGoogleIDToken(testGoogleIDToken(t, validGoogleClaims(now)), "client-1", "nonce-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Subject != "1234567890" || identity.Email != "alice@example.com" || identity.Name != "Alice" {
		t.Fatalf("unexpected identity %+v", identity)
	}
}

func TestParseGoogleIDTokenAcceptsLegacyStringEmailVerified(t *testing.T) {
	now := time.Now()
	claims := validGoogleClaims(now)
	claims["email_verified"] = "true"
	claims["iss"] = "accounts.google.com"
	claims["aud"] = []string{"other", "client-1"}
	if _, err := ParseGoogleIDToken(testGoogleIDToken(t, claims), "client-1", "nonce-1", now); err != nil {
		t.Fatal(err)
	}
}

func TestParseGoogleIDTokenRejectsInvalidClaims(t *testing.T) {
	now := time.Now()
	cases := map[string]func(map[string]any){
		"wrong audience": func(c map[string]any) { c["aud"] = "client-2" },
		"wrong issuer":   func(c map[string]any) { c["iss"] = "https://evil.example" },
		"wrong nonce":    func(c map[string]any) { c["nonce"] = "replayed" },
		"expired":        func(c map[string]any) { c["exp"] = now.Add(-time.Hour).Unix() },
		"missing sub":    func(c map[string]any) { delete(c, "sub") },
		"missing email":  func(c map[string]any) { delete(c, "email") },
		"oversized sub":  func(c map[string]any) { c["sub"] = strings.Repeat("1", 129) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			claims := validGoogleClaims(now)
			mutate(claims)
			_, err := ParseGoogleIDToken(testGoogleIDToken(t, claims), "client-1", "nonce-1", now)
			if !errors.Is(err, ErrGoogleIDTokenInvalid) {
				t.Fatalf("expected ErrGoogleIDTokenInvalid, got %v", err)
			}
		})
	}
	if _, err := ParseGoogleIDToken("not-a-jwt", "client-1", "nonce-1", now); !errors.Is(err, ErrGoogleIDTokenInvalid) {
		t.Fatalf("malformed token: got %v", err)
	}
}

func TestParseGoogleIDTokenRequiresVerifiedEmail(t *testing.T) {
	now := time.Now()
	for _, value := range []any{false, "false", nil} {
		claims := validGoogleClaims(now)
		claims["email_verified"] = value
		_, err := ParseGoogleIDToken(testGoogleIDToken(t, claims), "client-1", "nonce-1", now)
		if !errors.Is(err, ErrGoogleEmailNotVerified) {
			t.Fatalf("email_verified=%v: expected ErrGoogleEmailNotVerified, got %v", value, err)
		}
	}
}

func TestGooglePKCEChallengeMatchesRFC7636Example(t *testing.T) {
	// RFC 7636 Appendix B.
	if got := GooglePKCEChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("unexpected challenge %s", got)
	}
}

func TestGoogleAccountBindingContextIsStrict(t *testing.T) {
	bind := func(context string) error {
		_, err := BindVerificationOperation(VerificationOperation{Scope: VerificationScopeAccountBind, Context: json.RawMessage(context)})
		return err
	}
	if err := bind(`{"provider":"google"}`); err != nil {
		t.Fatalf("google binding context rejected: %v", err)
	}
	for _, context := range []string{`{"provider":"google","email":"a@b.c"}`, `{"provider":"google","code":"x"}`, `{"provider":"github"}`} {
		if err := bind(context); !errors.Is(err, ErrVerificationContextInvalid) {
			t.Fatalf("%s: expected ErrVerificationContextInvalid, got %v", context, err)
		}
	}
}
