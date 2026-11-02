package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/gin-gonic/gin"
)

func TestSecureSessionOriginGuard(t *testing.T) {
	oldSecure, oldURLs := common.SessionCookieSecure, common.SessionCookieTrustedURLs
	t.Cleanup(func() { common.SessionCookieSecure, common.SessionCookieTrustedURLs = oldSecure, oldURLs })
	common.SessionCookieSecure = true
	common.SessionCookieTrustedURLs = []string{"https://llm.trnhieu.dev"}
	r := gin.New()
	r.POST("/refresh", SessionCookieOriginGuard(), func(c *gin.Context) { c.Status(204) })
	for _, tc := range []struct {
		name    string
		origin  []string
		referer string
		host    string
		want    int
	}{
		{"trusted", []string{"https://llm.trnhieu.dev"}, "", "localhost", 204},
		{"normalized", []string{"https://LLM.trnhieu.dev:443"}, "", "localhost", 204},
		{"referer", nil, "https://llm.trnhieu.dev/console", "localhost", 204},
		{"missing", nil, "", "llm.trnhieu.dev", 403},
		{"null", []string{"null"}, "", "llm.trnhieu.dev", 403},
		{"http", []string{"http://llm.trnhieu.dev"}, "", "llm.trnhieu.dev", 403},
		{"port", []string{"https://llm.trnhieu.dev:444"}, "", "llm.trnhieu.dev", 403},
		{"multiple", []string{"https://llm.trnhieu.dev", "https://evil.invalid"}, "", "llm.trnhieu.dev", 403},
		{"comma", []string{"https://llm.trnhieu.dev,https://evil.invalid"}, "", "llm.trnhieu.dev", 403},
		{"forged_host", []string{"https://evil.invalid"}, "", "evil.invalid", 403},
		{"untrusted_referer", nil, "https://evil.invalid/", "llm.trnhieu.dev", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "https://"+tc.host+"/refresh", nil)
			req.TLS = &tls.ConnectionState{}
			req.Header["Origin"] = tc.origin
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}
			req.Header.Set("X-Forwarded-Host", "llm.trnhieu.dev")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("got %d want %d", w.Code, tc.want)
			}
		})
	}
}

func TestExplicitTrustedProxyPeers(t *testing.T) {
	r := gin.New()
	peers, defaults, err := common.ResolveTrustedProxies("172.30.0.1")
	if err != nil || defaults {
		t.Fatalf("proxy config: %v", err)
	}
	if err := common.ConfigureTrustedProxies(r, peers); err != nil {
		t.Fatal(err)
	}
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
	for _, tc := range []struct{ peer, want string }{
		{"172.30.0.1:1234", "203.0.113.9"},
		{"172.30.0.2:1234", "172.30.0.2"},
		{"198.51.100.1:1234", "198.51.100.1"},
	} {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = tc.peer
		req.Header.Set("X-Forwarded-For", "203.0.113.9")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Body.String() != tc.want {
			t.Fatalf("peer %s: got %s", tc.peer, w.Body.String())
		}
	}
}
