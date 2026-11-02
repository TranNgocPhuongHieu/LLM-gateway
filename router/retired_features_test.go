package router

import (
	"github.com/gin-gonic/gin"
	"strings"
	"testing"
)

func TestManagementRoutesRetireOptionalFeatures(t *testing.T) {
	engine := gin.New()
	SetApiRouter(engine)
	SetRelayRouter(engine)
	present := map[string]bool{}
	for _, route := range engine.Routes() {
		present[route.Method+" "+route.Path] = true
		for _, prefix := range []string{"/v1/videos", "/v1/video", "/mj/", "/suno/"} {
			if strings.HasPrefix(route.Path, prefix) {
				t.Errorf("retired media route still registered: %s", route.Path)
			}
		}
		for _, prefix := range []string{"/api/subscription", "/api/custom-oauth-provider", "/api/plugin/task", "/api/task", "/api/user/passkey/login"} {
			if strings.HasPrefix(route.Path, prefix) {
				t.Errorf("retired route still registered: %s %s", route.Method, route.Path)
			}
		}
		if strings.HasPrefix(route.Path, "/api/oauth/") && !strings.HasPrefix(route.Path, "/api/oauth/email/") {
			t.Errorf("external OAuth route still registered: %s", route.Path)
		}
	}
	for _, retired := range []string{"GET /api/user/aff", "POST /api/user/aff_transfer", "POST /api/user/topup", "GET /api/user/checkin", "POST /api/user/checkin"} {
		if present[retired] {
			t.Errorf("retired wallet route still registered: %s", retired)
		}
	}
	for _, retained := range []string{"GET /api/user/topup/info", "POST /api/user/pay", "POST /api/user/stripe/pay", "POST /api/stripe/webhook", "PUT /api/user/setting"} {
		if !present[retained] {
			t.Errorf("retained payment/settings route missing: %s", retained)
		}
	}
	for _, retained := range []string{"GET /api/pricing", "GET /api/status", "POST /api/user/login", "GET /api/log/", "GET /api/user/topup/self", "POST /api/verify", "POST /v1/responses", "POST /v1/responses/compact", "POST /v1/chat/completions", "POST /v1/messages", "POST /v1beta/models/*path"} {
		if !present[retained] {
			t.Errorf("retained route missing: %s", retained)
		}
	}
}
