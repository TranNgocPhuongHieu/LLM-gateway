package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/system_setting"
)

const (
	GoogleOAuthCallbackPath = "/api/oauth/google/callback"
	googleOAuthScope        = "openid email profile"
	googleTokenTimeout      = 10 * time.Second
	googleTokenBodyLimit    = 1 << 20
)

// Endpoints are variables so tests can point them at a local server.
var (
	GoogleAuthorizationEndpoint = "https://accounts.google.com/o/oauth2/v2/auth"
	GoogleTokenEndpoint         = "https://oauth2.googleapis.com/token"
	googleHTTPClient            = &http.Client{Timeout: googleTokenTimeout}
)

var (
	ErrGoogleOAuthDisabled      = errors.New("Google sign-in is not enabled.")
	ErrGoogleTokenExchange      = errors.New("Google sign-in could not be completed. Please try again.")
	ErrGoogleIDTokenInvalid     = errors.New("Google returned an invalid identity.")
	ErrGoogleEmailNotVerified   = errors.New("Your Google email address is not verified.")
	googleAcceptedTokenIssuers  = map[string]bool{"https://accounts.google.com": true, "accounts.google.com": true}
	googleIDTokenClockTolerance = 2 * time.Minute
)

// GoogleIdentity is the subset of verified ID token claims the gateway uses.
type GoogleIdentity struct {
	Subject string
	Email   string
	Name    string
}

// GoogleOAuthConfigured reports whether sign-in can start. All three values
// are required; a partially configured client must not be offered to users.
func GoogleOAuthConfigured() bool {
	return common.GoogleOAuthEnabled &&
		strings.TrimSpace(common.GoogleClientId) != "" &&
		strings.TrimSpace(common.GoogleClientSecret) != "" &&
		GoogleOAuthRedirectURI() != ""
}

// GoogleOAuthRedirectURI must match the redirect URI registered in Google
// Cloud exactly, so it is derived from the configured public server address.
func GoogleOAuthRedirectURI() string {
	base := strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/")
	if base == "" {
		return ""
	}
	return base + GoogleOAuthCallbackPath
}

// GooglePKCEChallenge derives the S256 code challenge for a verifier.
func GooglePKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// GoogleAuthorizationURL builds the consent redirect. state is the one-time
// auth flow token; nonce is echoed in the ID token and checked on return.
func GoogleAuthorizationURL(state, nonce, codeVerifier string) string {
	query := url.Values{}
	query.Set("client_id", strings.TrimSpace(common.GoogleClientId))
	query.Set("redirect_uri", GoogleOAuthRedirectURI())
	query.Set("response_type", "code")
	query.Set("scope", googleOAuthScope)
	query.Set("state", state)
	query.Set("nonce", nonce)
	query.Set("code_challenge", GooglePKCEChallenge(codeVerifier))
	query.Set("code_challenge_method", "S256")
	query.Set("prompt", "select_account")
	return GoogleAuthorizationEndpoint + "?" + query.Encode()
}

type googleTokenResponse struct {
	IDToken          string `json:"id_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// ExchangeGoogleCode redeems an authorization code and returns the verified
// identity. The ID token is received directly from Google's token endpoint
// over TLS in response to our authenticated request, so per OpenID Connect
// Core 3.1.3.7 the TLS server validation stands in for signature checking;
// issuer, audience, expiry, nonce and email_verified are still enforced.
func ExchangeGoogleCode(ctx context.Context, code, codeVerifier, expectedNonce string) (*GoogleIdentity, error) {
	if !GoogleOAuthConfigured() {
		return nil, ErrGoogleOAuthDisabled
	}
	code = strings.TrimSpace(code)
	if code == "" || codeVerifier == "" || expectedNonce == "" {
		return nil, ErrGoogleTokenExchange
	}
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", strings.TrimSpace(common.GoogleClientId))
	form.Set("client_secret", strings.TrimSpace(common.GoogleClientSecret))
	form.Set("redirect_uri", GoogleOAuthRedirectURI())
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", codeVerifier)

	ctx, cancel := context.WithTimeout(ctx, googleTokenTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, GoogleTokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build Google token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := googleHTTPClient.Do(request)
	if err != nil {
		common.SysError("Google token exchange failed: " + err.Error())
		return nil, ErrGoogleTokenExchange
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, googleTokenBodyLimit))
	if err != nil {
		common.SysError("Google token response read failed: " + err.Error())
		return nil, ErrGoogleTokenExchange
	}
	var token googleTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		common.SysError(fmt.Sprintf("Google token response is not JSON (status %d)", response.StatusCode))
		return nil, ErrGoogleTokenExchange
	}
	if response.StatusCode != http.StatusOK || token.IDToken == "" {
		// error and error_description are Google's public error codes; they
		// never contain the client secret or the authorization code.
		common.SysError(fmt.Sprintf("Google token exchange rejected (status %d): %s %s", response.StatusCode, token.Error, token.ErrorDescription))
		return nil, ErrGoogleTokenExchange
	}
	return ParseGoogleIDToken(token.IDToken, strings.TrimSpace(common.GoogleClientId), expectedNonce, time.Now())
}

type googleIDTokenClaims struct {
	Issuer        string          `json:"iss"`
	Audience      json.RawMessage `json:"aud"`
	Subject       string          `json:"sub"`
	ExpiresAt     int64           `json:"exp"`
	Nonce         string          `json:"nonce"`
	Email         string          `json:"email"`
	EmailVerified json.RawMessage `json:"email_verified"`
	Name          string          `json:"name"`
}

// ParseGoogleIDToken validates the claims of an ID token obtained from the
// token endpoint. See ExchangeGoogleCode for why the signature is not checked.
func ParseGoogleIDToken(idToken, clientID, expectedNonce string, now time.Time) (*GoogleIdentity, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 || clientID == "" || expectedNonce == "" {
		return nil, ErrGoogleIDTokenInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrGoogleIDTokenInvalid
	}
	var claims googleIDTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrGoogleIDTokenInvalid
	}
	if !googleAcceptedTokenIssuers[claims.Issuer] || !googleAudienceContains(claims.Audience, clientID) {
		return nil, ErrGoogleIDTokenInvalid
	}
	if claims.ExpiresAt <= 0 || now.After(time.Unix(claims.ExpiresAt, 0).Add(googleIDTokenClockTolerance)) {
		return nil, ErrGoogleIDTokenInvalid
	}
	if claims.Nonce != expectedNonce {
		return nil, ErrGoogleIDTokenInvalid
	}
	subject := strings.TrimSpace(claims.Subject)
	email := strings.TrimSpace(claims.Email)
	if subject == "" || len(subject) > 128 || email == "" {
		return nil, ErrGoogleIDTokenInvalid
	}
	if !googleBoolClaim(claims.EmailVerified) {
		return nil, ErrGoogleEmailNotVerified
	}
	return &GoogleIdentity{Subject: subject, Email: email, Name: strings.TrimSpace(claims.Name)}, nil
}

func googleAudienceContains(raw json.RawMessage, clientID string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == clientID
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		for _, audience := range many {
			if audience == clientID {
				return true
			}
		}
	}
	return false
}

// googleBoolClaim accepts both the boolean and the legacy string form.
func googleBoolClaim(raw json.RawMessage) bool {
	var value bool
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	var text string
	return json.Unmarshal(raw, &text) == nil && text == "true"
}
