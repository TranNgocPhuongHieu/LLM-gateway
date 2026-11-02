package channel

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func TestUpstreamRequestIDDoesNotLeakAcrossRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/first" {
			w.Header().Set("X-Request-Id", "xpiki-attempt-id")
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	for _, path := range []string{"/first", "/fallback"} {
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))
		req, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := DoRequest(c, req, info)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		want := ""
		if path == "/first" {
			want = "xpiki-attempt-id"
		}
		if got := c.GetString(common.UpstreamRequestIdKey); got != want {
			t.Fatalf("%s inherited wrong provider ID: %q != %q", path, got, want)
		}
	}
}
