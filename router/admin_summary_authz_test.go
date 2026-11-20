package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestAdminSummaryRoutesRequireRoot checks both admin summary routes: anonymous
// callers get 401, common and admin roles get 403, and the root role passes
// the middleware.
func TestAdminSummaryRoutesRequireRoot(t *testing.T) {
	setupRouterTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	for _, path := range []string{
		"/api/data/admin_summary?start_timestamp=1000&end_timestamp=5000",
		"/api/log/error_summary?start_timestamp=1000&end_timestamp=5000",
	} {
		t.Run(path, func(t *testing.T) {
			do := func(token string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				if token != "" {
					req.Header.Set("Authorization", "Bearer "+token)
				}
				rec := httptest.NewRecorder()
				engine.ServeHTTP(rec, req)
				return rec
			}
			if rec := do(""); rec.Code != http.StatusUnauthorized {
				t.Errorf("anonymous: status %d, want 401 (body: %s)", rec.Code, rec.Body.String())
			}
			if rec := do("pat_user_101"); rec.Code != http.StatusForbidden {
				t.Errorf("common user: status %d, want 403 (body: %s)", rec.Code, rec.Body.String())
			}
			if rec := do("pat_admin_102"); rec.Code != http.StatusForbidden {
				t.Errorf("admin user: status %d, want 403 (body: %s)", rec.Code, rec.Body.String())
			}
			if rec := do("pat_root_103"); rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
				t.Errorf("root user: status %d, want the handler to run (body: %s)", rec.Code, rec.Body.String())
			}
		})
	}
}
