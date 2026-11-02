package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCacheMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Cache())
	router.GET("/*path", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	tests := []struct {
		url         string
		expectedHdr string
	}{
		{"/", "no-cache, must-revalidate"},
		{"/logo.png", "no-cache, must-revalidate"},
		{"/favicon.ico", "no-cache, must-revalidate"},
		{"/logo.png?v=740e706f", "max-age=604800"},
		{"/favicon.ico?v=1af31875", "max-age=604800"},
		{"/assets/index.js", "max-age=604800"},
	}

	for _, tt := range tests {
		req, _ := http.NewRequest(http.MethodGet, tt.url, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		got := rec.Header().Get("Cache-Control")
		if got != tt.expectedHdr {
			t.Errorf("URL %s: expected Cache-Control %q, got %q", tt.url, tt.expectedHdr, got)
		}
	}
}
