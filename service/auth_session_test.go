package service

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"
)

func TestSessionSecurityLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	oldDB, oldRedis, oldEncryption, oldSecure := model.DB, common.RedisEnabled, common.PasswordLoginEncryptionEnabled, common.SessionCookieSecure
	model.DB, common.RedisEnabled, common.PasswordLoginEncryptionEnabled, common.SessionCookieSecure = db, false, false, true
	passkeySettings := system_setting.GetPasskeySettings()
	oldPasskeyEnabled := passkeySettings.Enabled
	passkeySettings.Enabled = true
	t.Cleanup(func() {
		passkeySettings.Enabled = oldPasskeyEnabled
		model.DB, common.RedisEnabled, common.PasswordLoginEncryptionEnabled, common.SessionCookieSecure = oldDB, oldRedis, oldEncryption, oldSecure
		_ = conn.Close()
	})
	if err := db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.AuthFlow{}, &model.TwoFA{}, &model.TwoFABackupCode{}, &model.PasskeyCredential{}); err != nil {
		t.Fatal(err)
	}
	hash, err := common.Password2Hash("fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{Id: 812345, Username: "session-fixture", Password: hash, AuthVersion: 1, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "session-fixture"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	bundle, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := ParseAccessToken(bundle.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidateLoginSession(identity); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	WriteRefreshCookie(c, bundle.RefreshToken)
	found := false
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == RefreshCookieName {
			found = true
			if !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/api/user/auth" {
				t.Fatal("refresh cookie lost security attributes")
			}
		}
	}
	if !found {
		t.Fatal("refresh cookie missing")
	}
	input := VerificationInput{Method: VerificationMethodPassword, Scope: VerificationScopePasskeyRegister, Password: "wrong"}
	if _, err := VerifySecurityInput(identity, input); !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("wrong password result: %v", err)
	}
	input.Password = "fixture-password"
	proof, err := VerifySecurityInput(identity, input)
	if err != nil {
		t.Fatal(err)
	}
	op := VerificationOperation{Scope: input.Scope}
	if _, err := ConsumeOperationProof(proof.ProofToken, identity, op); err != nil {
		t.Fatal(err)
	}
	if _, err := ConsumeOperationProof(proof.ProofToken, identity, op); err == nil {
		t.Fatal("security proof replay accepted")
	}
	emailContext := json.RawMessage(`{"provider":"email","email":"fixture@example.invalid"}`)
	emailProof, err := VerifySecurityInput(identity, VerificationInput{Method: VerificationMethodPassword, Scope: VerificationScopeAccountBind, Context: emailContext, Password: "fixture-password"})
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := ConsumeOperationProof(emailProof.ProofToken, identity, VerificationOperation{Scope: VerificationScopeAccountBind, Context: emailContext})
	if err != nil {
		t.Fatal(err)
	}
	oldSMTPServer, oldSMTPAccount, oldSMTPFrom := common.SMTPServer, common.SMTPAccount, common.SMTPFrom
	common.SMTPServer, common.SMTPAccount, common.SMTPFrom = "", "", ""
	t.Cleanup(func() {
		common.SMTPServer, common.SMTPAccount, common.SMTPFrom = oldSMTPServer, oldSMTPAccount, oldSMTPFrom
	})
	if _, err := StartEmailBinding(identity, authorization, "fixture@example.invalid"); !errors.Is(err, ErrEmailBindingDelivery) {
		t.Fatalf("email delivery failure was not explicit: %v", err)
	}
	var saved model.User
	if err := db.First(&saved, user.Id).Error; err != nil || saved.Email != "" {
		t.Fatal("failed delivery bound email")
	}
	rotated, _, err := RefreshLoginSession(bundle.RefreshToken, bundle.Session.SID, "127.0.0.1", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if rotated.RefreshToken == bundle.RefreshToken {
		t.Fatal("refresh token not rotated")
	}
	if _, _, err := RefreshLoginSession(bundle.RefreshToken, "wrong-session", "127.0.0.1", "fixture"); err == nil {
		t.Fatal("wrong session accepted")
	}
	if err := RevokeByRefreshToken(rotated.RefreshToken, rotated.Session.SID, "logout"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidateLoginSession(identity); err == nil {
		t.Fatal("revoked session accepted")
	}

	factor := model.TwoFA{UserId: user.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}
	if err := db.Create(&factor).Error; err != nil {
		t.Fatal(err)
	}
	challenge, err := StartLoginVerification(&user, "password")
	if err != nil || challenge == nil {
		t.Fatalf("2FA challenge missing: %v", err)
	}
	if _, err := RequireLoginVerification(challenge.FlowToken, VerificationMethodPassword); err == nil {
		t.Fatal("password bypassed enrolled 2FA")
	}
	if _, err := VerifyLoginCode(challenge.FlowToken, "bad", "127.0.0.1", "fixture"); err == nil {
		t.Fatal("invalid 2FA accepted")
	}
	code, err := totp.GenerateCode(factor.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLoginCode(challenge.FlowToken, code, "127.0.0.1", "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLoginCode(challenge.FlowToken, code, "127.0.0.1", "fixture"); err == nil {
		t.Fatal("login flow replay accepted")
	}
	var workers sync.WaitGroup
	errors := make(chan error, 4)
	for i := 0; i < 4; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			// Exercise concurrent session persistence, not a password/2FA bypass:
			// production calls this only after completing primary authentication.
			_, err := CreateLoginSession(user.Id, "fixture-authenticated", "127.0.0.1", "fixture")
			errors <- err
		}()
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
}
