package controller

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Draining rejects new application requests while existing relay requests are
// allowed to finish before the process receives SIGTERM.
var admission struct {
	sync.Mutex
	until  time.Time
	active int
}

func isDraining() bool {
	admission.Lock()
	defer admission.Unlock()
	return time.Now().Before(admission.until)
}

// Called only after the listener is closed. Unlike net/http.Shutdown this also
// waits for upgraded connections whose handler is still running.
func WaitAdmittedRequests() {
	for {
		admission.Lock()
		active := admission.active
		admission.Unlock()
		if active == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func DrainMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/healthz" || path == "/readyz" || path == "/internal/drain" {
			c.Next()
			return
		}
		admission.Lock()
		// Payment callbacks remain admitted until HTTP shutdown closes the listener.
		webhook := path == "/api/payment/sepay/ipn" || path == "/api/payment/sepay/webhook"
		if time.Now().Before(admission.until) && !webhook {
			admission.Unlock()
			c.Header("Retry-After", "30")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "server is draining"}})
			return
		}
		admission.active++
		admission.Unlock()
		defer func() { admission.Lock(); admission.active--; admission.Unlock() }()
		c.Next()
	}
}

func Drain(c *gin.Context) {
	want := strings.TrimSpace(os.Getenv("DRAIN_TOKEN"))
	got := strings.TrimSpace(c.GetHeader("X-Drain-Token"))
	if want == "" || len(got) != len(want) || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	admission.Lock()
	defer admission.Unlock()
	switch c.Query("action") {
	case "", "begin":
		// A lost deployment controller cannot leave an instance unavailable forever.
		admission.until = time.Now().Add(10 * time.Minute)
	case "status":
	case "cancel":
		admission.until = time.Time{}
	default:
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"draining": time.Now().Before(admission.until), "active_requests": admission.active})
}
