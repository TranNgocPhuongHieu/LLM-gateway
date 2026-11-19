package controller

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/TranNgocPhuongHieu/LLM-gateway/service"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fakeGoogle is a token endpoint that issues an ID token for the identity
// configured by the test, echoing the nonce captured from the consent URL.
type fakeGoogle struct {
	mu            sync.Mutex
	subject       string
	email         string
	emailVerified bool
	nonce         string
	lastForm      url.Values
}

func (f *fakeGoogle) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse token form: %v", err)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.lastForm = r.PostForm
		claims, _ := json.Marshal(map[string]any{
			"iss": "https://accounts.google.com", "aud": "client-1", "sub": f.subject,
			"exp": time.Now().Add(time.Hour).Unix(), "nonce": f.nonce,
			"email": f.email, "email_verified": f.emailVerified, "name": "Google Person",
		})
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id_token": "h." + base64.RawURLEncoding.EncodeToString(claims) + ".s",
		})
	}
}

func setupGoogleOAuthTest(t *testing.T) (*gorm.DB, *fakeGoogle) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.User{}, &model.AuthFlow{}, &model.ExternalIdentityClaim{}, &model.Token{}, &model.Log{},
		&model.UserSession{}, &model.TwoFA{}, &model.TwoFABackupCode{}, &model.PasskeyCredential{}); err != nil {
		t.Fatal(err)
	}
	google := &fakeGoogle{subject: "google-sub-1", email: "new.person@gmail.com", emailVerified: true}
	server := httptest.NewServer(google.handler(t))

	old := struct {
		db, logDB                       *gorm.DB
		mainType, logType               common.DatabaseType
		memCache, redis                 bool
		enabled, register               bool
		clientID, secret, addr, tokenEP string
		quota                           int
	}{model.DB, model.LOG_DB, common.MainDatabaseType(), common.LogDatabaseType(), common.MemoryCacheEnabled, common.RedisEnabled,
		common.GoogleOAuthEnabled, common.RegisterEnabled, common.GoogleClientId, common.GoogleClientSecret,
		system_setting.ServerAddress, service.GoogleTokenEndpoint, common.QuotaForNewUser}
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled, common.RedisEnabled = false, false
	common.GoogleOAuthEnabled, common.RegisterEnabled = true, true
	common.GoogleClientId, common.GoogleClientSecret = "client-1", "test-secret"
	system_setting.ServerAddress = "https://gw.test"
	service.GoogleTokenEndpoint = server.URL
	common.QuotaForNewUser = 2_500_000
	t.Cleanup(func() {
		server.Close()
		model.DB, model.LOG_DB = old.db, old.logDB
		common.SetDatabaseTypes(old.mainType, old.logType)
		common.MemoryCacheEnabled, common.RedisEnabled = old.memCache, old.redis
		common.GoogleOAuthEnabled, common.RegisterEnabled = old.enabled, old.register
		common.GoogleClientId, common.GoogleClientSecret = old.clientID, old.secret
		system_setting.ServerAddress = old.addr
		service.GoogleTokenEndpoint = old.tokenEP
		common.QuotaForNewUser = old.quota
		_ = conn.Close()
	})
	return db, google
}

func googleCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == googleOAuthBrowserCookie {
			return cookie
		}
	}
	t.Fatal("browser cookie was not set")
	return nil
}

// startGoogle calls GoogleOAuthStart and returns the consent URL query and cookie.
func startGoogle(t *testing.T, google *fakeGoogle) (url.Values, *http.Cookie) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/oauth/google/start", nil)
	GoogleOAuthStart(c)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || !body.Success {
		t.Fatalf("start failed: %s", w.Body.String())
	}
	consent, err := url.Parse(body.Data.URL)
	if err != nil {
		t.Fatal(err)
	}
	query := consent.Query()
	if query.Get("redirect_uri") != "https://gw.test/api/oauth/google/callback" || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("unexpected consent URL %s", body.Data.URL)
	}
	google.mu.Lock()
	google.nonce = query.Get("nonce")
	google.mu.Unlock()
	return query, googleCookie(t, w)
}

func callGoogleCallback(t *testing.T, state string, cookie *http.Cookie) url.Values {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/oauth/google/callback?code=auth-code&state="+url.QueryEscape(state), nil)
	if cookie != nil {
		c.Request.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	GoogleOAuthCallback(c)
	if w.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d: %s", w.Code, w.Body.String())
	}
	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil || location.Path != googleOAuthFrontendPath {
		t.Fatalf("unexpected redirect %q", w.Header().Get("Location"))
	}
	return location.Query()
}

func callGoogleFinish(t *testing.T, ticket string, cookie *http.Cookie) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/oauth/google/finish", strings.NewReader(`{"ticket":"`+ticket+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		c.Request.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	GoogleOAuthFinish(c)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("finish response is not JSON: %s", w.Body.String())
	}
	return body
}

func TestGoogleSignUpCreatesAccountWithGrantOnceAndLogsIn(t *testing.T) {
	db, google := setupGoogleOAuthTest(t)

	query, cookie := startGoogle(t, google)
	result := callGoogleCallback(t, query.Get("state"), cookie)
	if result.Get("ticket") == "" {
		t.Fatalf("expected ticket, got %v", result)
	}
	if google.lastForm.Get("code_verifier") == "" || google.lastForm.Get("client_secret") != "test-secret" {
		t.Fatalf("token request missing PKCE verifier or client auth: %v", google.lastForm)
	}
	if service.GooglePKCEChallenge(google.lastForm.Get("code_verifier")) != query.Get("code_challenge") {
		t.Fatal("PKCE verifier does not match the challenge")
	}

	var user model.User
	if err := db.Where("email = ?", "new.person@gmail.com").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if user.Quota != 2_500_000 || user.Password != "" || user.Role != common.RoleCommonUser {
		t.Fatalf("unexpected new user %+v", user)
	}
	owner, err := model.FindUserByExternalIdentity(model.ExternalIdentityProviderGoogle, "google-sub-1")
	if err != nil || owner.Id != user.Id {
		t.Fatalf("identity claim missing: %v", err)
	}

	body := callGoogleFinish(t, result.Get("ticket"), cookie)
	data, _ := body["data"].(map[string]any)
	if body["success"] != true || data["access_token"] == "" || data["access_token"] == nil {
		t.Fatalf("finish did not log in: %v", body)
	}
	if replay := callGoogleFinish(t, result.Get("ticket"), cookie); replay["success"] == true {
		t.Fatal("ticket must be single use")
	}

	// Signing in again with the same Google identity must not grant again.
	query, cookie = startGoogle(t, google)
	if again := callGoogleCallback(t, query.Get("state"), cookie); again.Get("ticket") == "" {
		t.Fatalf("second sign-in failed: %v", again)
	}
	var count int64
	db.Model(&model.User{}).Count(&count)
	if err := db.First(&user, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 || user.Quota != 2_500_000 {
		t.Fatalf("second sign-in must reuse the account: users=%d quota=%d", count, user.Quota)
	}
}

func TestGoogleSignUpNeverMergesExistingEmail(t *testing.T) {
	db, google := setupGoogleOAuthTest(t)
	existing := model.User{Username: "legacy", Password: "hashed", Email: "new.person@gmail.com", Role: common.RoleCommonUser, AffCode: "a1", AuthVersion: 1}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}

	query, cookie := startGoogle(t, google)
	if result := callGoogleCallback(t, query.Get("state"), cookie); result.Get("error") != "email_exists" {
		t.Fatalf("expected email_exists, got %v", result)
	}
	if _, err := model.FindUserByExternalIdentity(model.ExternalIdentityProviderGoogle, "google-sub-1"); err == nil {
		t.Fatal("Google identity must not be attached to the existing account")
	}

	// A soft-deleted account keeps its email reserved, so the grant cannot be farmed.
	if err := db.Delete(&existing).Error; err != nil {
		t.Fatal(err)
	}
	query, cookie = startGoogle(t, google)
	if result := callGoogleCallback(t, query.Get("state"), cookie); result.Get("error") != "email_exists" {
		t.Fatalf("deleted account email: expected email_exists, got %v", result)
	}
}

func TestGoogleCallbackRejectsForeignBrowserAndUnverifiedEmail(t *testing.T) {
	db, google := setupGoogleOAuthTest(t)

	query, _ := startGoogle(t, google)
	if result := callGoogleCallback(t, query.Get("state"), nil); result.Get("error") != "invalid_state" {
		t.Fatalf("missing cookie: expected invalid_state, got %v", result)
	}
	query, cookie := startGoogle(t, google)
	if result := callGoogleCallback(t, query.Get("state"), &http.Cookie{Name: cookie.Name, Value: "attacker"}); result.Get("error") != "invalid_state" {
		t.Fatalf("foreign cookie: expected invalid_state, got %v", result)
	}
	if result := callGoogleCallback(t, "unknown-state", cookie); result.Get("error") != "invalid_state" {
		t.Fatalf("unknown state: expected invalid_state, got %v", result)
	}

	google.emailVerified = false
	query, cookie = startGoogle(t, google)
	if result := callGoogleCallback(t, query.Get("state"), cookie); result.Get("error") != "email_unverified" {
		t.Fatalf("expected email_unverified, got %v", result)
	}
	var count int64
	db.Model(&model.User{}).Count(&count)
	if count != 0 {
		t.Fatalf("no account may be created, got %d", count)
	}
}

func TestGoogleTicketRequiresOriginatingBrowser(t *testing.T) {
	_, google := setupGoogleOAuthTest(t)
	query, cookie := startGoogle(t, google)
	result := callGoogleCallback(t, query.Get("state"), cookie)
	if body := callGoogleFinish(t, result.Get("ticket"), &http.Cookie{Name: cookie.Name, Value: "other"}); body["success"] == true {
		t.Fatal("ticket must not log in a different browser")
	}
}

func TestGoogleSignUpRespectsRegistrationSwitch(t *testing.T) {
	_, google := setupGoogleOAuthTest(t)
	common.RegisterEnabled = false
	query, cookie := startGoogle(t, google)
	if result := callGoogleCallback(t, query.Get("state"), cookie); result.Get("error") != "registration_disabled" {
		t.Fatalf("expected registration_disabled, got %v", result)
	}
}

func TestGoogleBindLinksSignedInAccountOnce(t *testing.T) {
	db, google := setupGoogleOAuthTest(t)
	user := model.User{Username: "legacy", Password: "hashed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "a1", AuthVersion: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	session := model.UserSession{SID: "sid-1", UserID: user.Id, Status: model.UserSessionStatusActive, UserAuthVersion: 1, Version: 1, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	identity := model.AuthSessionIdentity{UserID: user.Id, SessionID: "sid-1", UserAuthVersion: 1, SessionVersion: 1}

	bind := func() url.Values {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		authURL, err := startGoogleFlow(c, model.AuthFlowIntentBind, &identity)
		if err != nil {
			t.Fatal(err)
		}
		consent, _ := url.Parse(authURL)
		google.nonce = consent.Query().Get("nonce")
		return callGoogleCallback(t, consent.Query().Get("state"), googleCookie(t, w))
	}

	if result := bind(); result.Get("status") != "linked" || result.Get("mode") != "bind" {
		t.Fatalf("expected linked, got %v", result)
	}
	owner, err := model.FindUserByExternalIdentity(model.ExternalIdentityProviderGoogle, "google-sub-1")
	if err != nil || owner.Id != user.Id {
		t.Fatalf("bind did not claim identity: %v", err)
	}
	google.subject = "google-sub-2"
	if result := bind(); result.Get("error") != "already_linked" {
		t.Fatalf("expected already_linked, got %v", result)
	}
}
