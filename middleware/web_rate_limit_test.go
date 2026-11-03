package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/gin-gonic/gin"
)

func TestWebRateLimitAssetBurstDoesNotConsumePageBudget(t *testing.T) {
	oldEnabled, oldRedis := common.GlobalWebRateLimitEnable, common.RedisEnabled
	oldNum, oldDuration := common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration
	t.Cleanup(func() {
		common.GlobalWebRateLimitEnable, common.RedisEnabled = oldEnabled, oldRedis
		common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration = oldNum, oldDuration
	})
	common.GlobalWebRateLimitEnable, common.RedisEnabled = true, false
	common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration = 1, 180
	r := gin.New()
	r.Use(GlobalWebRateLimit())
	r.NoRoute(func(c *gin.Context) { c.Status(200) })
	request := func(path string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		req.RemoteAddr = "192.0.2.187:12345"
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s: status %d, want %d", path, w.Code, want)
		}
	}
	for i := 0; i < 125; i++ {
		request("/static/js/async/chunk.js", 200)
	}
	request("/login", 200)
	request("/login", 429)
	request("/static/css/app.css", 200)
	request("/favicon.ico", 200)
}
