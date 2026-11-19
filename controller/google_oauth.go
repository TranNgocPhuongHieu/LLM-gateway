package controller

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/middleware"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/TranNgocPhuongHieu/LLM-gateway/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	googleOAuthFlowPurpose   = "google_oauth"
	googleLoginTicketPurpose = "google_login"
	googleOAuthBrowserCookie = "google_oauth_browser"
	googleOAuthCookiePath    = "/api/oauth/google"
	googleOAuthFlowTTL       = 10 * time.Minute
	googleLoginTicketTTL     = 2 * time.Minute
	googleOAuthFrontendPath  = "/oauth/google"
)

// googleOAuthFlowPayload is server-side state for one consent round trip.
// Browser is an HMAC of the HttpOnly cookie set at start, which ties the
// callback to the browser that began the flow (login CSRF protection).
type googleOAuthFlowPayload struct {
	Nonce    string                     `json:"nonce"`
	Verifier string                     `json:"verifier"`
	Browser  string                     `json:"browser"`
	Identity *model.AuthSessionIdentity `json:"identity,omitempty"`
}

type googleLoginTicketPayload struct {
	Browser string `json:"browser"`
}

func googleRandomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func googleBrowserBinding(value string) string {
	return common.GenerateHMACWithKey([]byte("google-oauth-browser-v1:"+common.SessionSecret), value)
}

func setGoogleBrowserCookie(c *gin.Context, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     googleOAuthBrowserCookie,
		Value:    value,
		Path:     googleOAuthCookiePath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   common.SessionCookieSecure,
		// Lax is required: the callback is a top-level navigation from Google.
		SameSite: http.SameSiteLaxMode,
	})
}

func googleBrowserMatches(c *gin.Context, expected string) bool {
	value, err := c.Cookie(googleOAuthBrowserCookie)
	if err != nil || value == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(googleBrowserBinding(value)), []byte(expected)) == 1
}

// startGoogleFlow persists the flow and returns the Google consent URL.
func startGoogleFlow(c *gin.Context, intent string, identity *model.AuthSessionIdentity) (string, error) {
	nonce, err := googleRandomToken()
	if err != nil {
		return "", err
	}
	verifier, err := googleRandomToken()
	if err != nil {
		return "", err
	}
	browser, err := googleRandomToken()
	if err != nil {
		return "", err
	}
	payload, err := common.Marshal(googleOAuthFlowPayload{
		Nonce: nonce, Verifier: verifier, Browser: googleBrowserBinding(browser), Identity: identity,
	})
	if err != nil {
		return "", err
	}
	create := model.AuthFlowCreate{
		Purpose:   googleOAuthFlowPurpose,
		Provider:  model.ExternalIdentityProviderGoogle,
		Intent:    intent,
		Payload:   string(payload),
		ExpiresAt: time.Now().Add(googleOAuthFlowTTL),
	}
	if identity != nil {
		create.UserId = identity.UserID
		create.SessionId = identity.SessionID
	}
	state, _, err := model.CreateAuthFlow(create)
	if err != nil {
		return "", err
	}
	setGoogleBrowserCookie(c, browser, int(googleOAuthFlowTTL/time.Second))
	return service.GoogleAuthorizationURL(state, nonce, verifier), nil
}

// GoogleOAuthStart begins Google sign-in or sign-up.
func GoogleOAuthStart(c *gin.Context) {
	if !service.GoogleOAuthConfigured() {
		common.ApiErrorMsg(c, service.ErrGoogleOAuthDisabled.Error())
		return
	}
	authURL, err := startGoogleFlow(c, model.AuthFlowIntentLogin, nil)
	if err != nil {
		common.SysError("start Google sign-in: " + err.Error())
		common.ApiErrorMsg(c, service.ErrGoogleTokenExchange.Error())
		return
	}
	setAuthNoStore(c)
	common.ApiSuccess(c, gin.H{"url": authURL})
}

// GoogleBindStart begins linking Google to the signed-in account. Like email
// binding it requires a fresh security proof, because it adds a login method.
func GoogleBindStart(c *gin.Context) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		writeSecurityOperationError(c, service.ErrAuthTokenInvalid)
		return
	}
	succeeded := false
	defer func() {
		recordUserSecurityAudit(c, identity.UserID, "user.binding_start", map[string]any{"provider": "google", "success": succeeded})
	}()
	if !service.GoogleOAuthConfigured() {
		common.ApiErrorMsg(c, service.ErrGoogleOAuthDisabled.Error())
		return
	}
	linked, err := model.HasExternalIdentity(identity.UserID, model.ExternalIdentityProviderGoogle)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if linked {
		common.ApiErrorMsg(c, "Google is already linked to this account.")
		return
	}
	context, err := common.Marshal(service.AccountBindingContext{Provider: "google"})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeAccountBind, Context: context}) == nil {
		return
	}
	authURL, err := startGoogleFlow(c, model.AuthFlowIntentBind, &identity)
	if err != nil {
		common.SysError("start Google binding: " + err.Error())
		common.ApiErrorMsg(c, service.ErrGoogleTokenExchange.Error())
		return
	}
	succeeded = true
	setAuthNoStore(c)
	common.ApiSuccess(c, gin.H{"url": authURL})
}

func redirectGoogleResult(c *gin.Context, params url.Values) {
	setAuthNoStore(c)
	c.Redirect(http.StatusFound, googleOAuthFrontendPath+"?"+params.Encode())
}

func redirectGoogleError(c *gin.Context, intent, code string) {
	setGoogleBrowserCookie(c, "", -1)
	params := url.Values{"error": {code}}
	if intent == model.AuthFlowIntentBind {
		params.Set("mode", "bind")
	}
	redirectGoogleResult(c, params)
}

// GoogleOAuthCallback receives Google's redirect, verifies the identity and
// hands the browser back to the SPA with a one-time ticket or a result code.
func GoogleOAuthCallback(c *gin.Context) {
	state := strings.TrimSpace(c.Query("state"))
	match := model.AuthFlowMatch{Purpose: googleOAuthFlowPurpose, Provider: model.ExternalIdentityProviderGoogle}
	flow, err := model.GetAuthFlow(state, match)
	if err != nil {
		redirectGoogleError(c, "", "invalid_state")
		return
	}
	var payload googleOAuthFlowPayload
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil || !googleBrowserMatches(c, payload.Browser) {
		redirectGoogleError(c, flow.Intent, "invalid_state")
		return
	}
	if _, err := model.ConsumeAuthFlow(state, match); err != nil {
		redirectGoogleError(c, flow.Intent, "invalid_state")
		return
	}
	if googleError := c.Query("error"); googleError != "" {
		code := "exchange_failed"
		if googleError == "access_denied" {
			code = "cancelled"
		}
		redirectGoogleError(c, flow.Intent, code)
		return
	}
	googleIdentity, err := service.ExchangeGoogleCode(c.Request.Context(), c.Query("code"), payload.Verifier, payload.Nonce)
	if err != nil {
		code := "exchange_failed"
		switch {
		case errors.Is(err, service.ErrGoogleEmailNotVerified):
			code = "email_unverified"
		case errors.Is(err, service.ErrGoogleOAuthDisabled):
			code = "disabled"
		}
		redirectGoogleError(c, flow.Intent, code)
		return
	}

	if flow.Intent == model.AuthFlowIntentBind {
		completeGoogleBind(c, flow, payload, googleIdentity)
		return
	}
	completeGoogleLogin(c, payload, googleIdentity)
}

func completeGoogleBind(c *gin.Context, flow *model.AuthFlow, payload googleOAuthFlowPayload, identity *service.GoogleIdentity) {
	if payload.Identity == nil || payload.Identity.UserID != flow.UserId || payload.Identity.SessionID != flow.SessionId {
		redirectGoogleError(c, model.AuthFlowIntentBind, "invalid_state")
		return
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		return model.BindGoogleForSessionWithTx(tx, *payload.Identity, identity.Subject)
	})
	recordUserSecurityAudit(c, flow.UserId, "user.binding", map[string]any{"provider": "google", "success": err == nil})
	if err != nil {
		code := "server_error"
		switch {
		case errors.Is(err, model.ErrExternalIdentityAlreadyClaimed):
			code = "identity_taken"
		case errors.Is(err, model.ErrGoogleAlreadyLinked):
			code = "already_linked"
		case errors.Is(err, model.ErrUserSessionInactive), errors.Is(err, gorm.ErrRecordNotFound):
			code = "session_changed"
		default:
			common.SysError(fmt.Sprintf("bind Google for user %d: %v", flow.UserId, err))
		}
		redirectGoogleError(c, model.AuthFlowIntentBind, code)
		return
	}
	setGoogleBrowserCookie(c, "", -1)
	redirectGoogleResult(c, url.Values{"mode": {"bind"}, "status": {"linked"}})
}

func completeGoogleLogin(c *gin.Context, payload googleOAuthFlowPayload, identity *service.GoogleIdentity) {
	user, err := model.FindUserByExternalIdentity(model.ExternalIdentityProviderGoogle, identity.Subject)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user, err = registerGoogleUser(identity)
		if err != nil {
			code := "server_error"
			switch {
			case errors.Is(err, errGoogleRegistrationDisabled):
				code = "registration_disabled"
			case errors.Is(err, model.ErrGoogleEmailBelongsToAccount):
				code = "email_exists"
			case errors.Is(err, model.ErrExternalIdentityAlreadyClaimed):
				code = "identity_taken"
			default:
				common.SysError("register Google user: " + err.Error())
			}
			redirectGoogleError(c, model.AuthFlowIntentLogin, code)
			return
		}
	} else if err != nil {
		common.SysError("find Google user: " + err.Error())
		redirectGoogleError(c, model.AuthFlowIntentLogin, "server_error")
		return
	}
	if user.Status != common.UserStatusEnabled {
		redirectGoogleError(c, model.AuthFlowIntentLogin, "account_disabled")
		return
	}
	ticketPayload, err := common.Marshal(googleLoginTicketPayload{Browser: payload.Browser})
	if err != nil {
		redirectGoogleError(c, model.AuthFlowIntentLogin, "server_error")
		return
	}
	ticket, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose:   googleLoginTicketPurpose,
		Provider:  model.ExternalIdentityProviderGoogle,
		UserId:    user.Id,
		Payload:   string(ticketPayload),
		ExpiresAt: time.Now().Add(googleLoginTicketTTL),
	})
	if err != nil {
		common.SysError("create Google login ticket: " + err.Error())
		redirectGoogleError(c, model.AuthFlowIntentLogin, "server_error")
		return
	}
	redirectGoogleResult(c, url.Values{"ticket": {ticket}})
}

var errGoogleRegistrationDisabled = errors.New("registration is disabled")

// registerGoogleUser creates the account for a first-time Google identity.
// New accounts receive QuotaForNewUser once, inside the creating transaction.
func registerGoogleUser(identity *service.GoogleIdentity) (*model.User, error) {
	if !common.RegisterEnabled {
		return nil, errGoogleRegistrationDisabled
	}
	user, err := model.CreateGoogleUser(identity.Subject, identity.Email, identity.Name)
	if err != nil {
		return nil, err
	}
	user.FinishInsert()
	if err := createDefaultUserToken(user.Id, user.Username); err != nil {
		// The account exists and can create keys itself; do not fail sign-up.
		common.SysError(fmt.Sprintf("create default token for Google user %d: %v", user.Id, err))
	}
	return user, nil
}

type googleFinishRequest struct {
	Ticket string `json:"ticket"`
}

// GoogleOAuthFinish exchanges the one-time ticket for a dashboard session,
// applying the same login verification policy as password sign-in.
func GoogleOAuthFinish(c *gin.Context) {
	var request googleFinishRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || strings.TrimSpace(request.Ticket) == "" {
		common.ApiErrorMsg(c, "Google sign-in link is invalid or expired. Please try again.")
		return
	}
	match := model.AuthFlowMatch{Purpose: googleLoginTicketPurpose, Provider: model.ExternalIdentityProviderGoogle}
	flow, err := model.ConsumeAuthFlowWithAction(strings.TrimSpace(request.Ticket), match, func(tx *gorm.DB, flow *model.AuthFlow) error {
		var payload googleLoginTicketPayload
		if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil || !googleBrowserMatches(c, payload.Browser) {
			return model.ErrAuthFlowInvalid
		}
		return nil
	})
	setGoogleBrowserCookie(c, "", -1)
	if err != nil {
		common.ApiErrorMsg(c, "Google sign-in link is invalid or expired. Please try again.")
		return
	}
	user, err := model.GetUserById(flow.UserId, true)
	if err != nil {
		common.ApiErrorMsg(c, "Google sign-in link is invalid or expired. Please try again.")
		return
	}
	c.Set("login_method", "google")
	setupLogin(user, c)
}
