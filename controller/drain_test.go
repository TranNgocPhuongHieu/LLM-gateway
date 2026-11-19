package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestDrainRequiresTokenAndRejectsNewRequests(t *testing.T) {
	oldReady := ServingReady.Load()
	t.Cleanup(func() {
		ServingReady.Store(oldReady)
		admission.Lock()
		admission.until = time.Time{}
		admission.Unlock()
	})
	ServingReady.Store(true)
	t.Setenv("DRAIN_TOKEN", "test-drain-token")
	r := gin.New()
	r.Use(DrainMiddleware())
	r.POST("/internal/drain", Drain)
	r.POST("/v1/chat/completions", func(c *gin.Context) { c.Status(http.StatusOK) })
	unauth := httptest.NewRecorder()
	r.ServeHTTP(unauth, httptest.NewRequest(http.MethodPost, "/internal/drain", nil))
	if unauth.Code != http.StatusNotFound {
		t.Fatalf("unauthenticated drain status=%d", unauth.Code)
	}
	auth := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/drain", nil)
	req.Header.Set("X-Drain-Token", "test-drain-token")
	r.ServeHTTP(auth, req)
	if auth.Code != http.StatusAccepted || !isDraining() || !ServingReady.Load() {
		t.Fatalf("drain did not enter draining state: code=%d body=%s", auth.Code, auth.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	blocked := httptest.NewRecorder()
	r.ServeHTTP(blocked, req)
	if blocked.Code != http.StatusServiceUnavailable {
		t.Fatalf("new request status=%d", blocked.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/internal/drain?action=cancel", nil)
	req.Header.Set("X-Drain-Token", "test-drain-token")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if isDraining() {
		t.Fatal("cancel did not reopen admission")
	}
}

func TestConcurrentAdmissionAndDrain(t *testing.T) {
	t.Setenv("DRAIN_TOKEN", "race-test")
	t.Cleanup(func() { admission.Lock(); admission.until = time.Time{}; admission.Unlock() })
	release := make(chan struct{})
	r := gin.New()
	r.Use(DrainMiddleware())
	r.POST("/internal/drain", Drain)
	r.GET("/work", func(c *gin.Context) { <-release; c.Status(200) })
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/work", nil)) }()
	}
	req := httptest.NewRequest("POST", "/internal/drain", nil)
	req.Header.Set("X-Drain-Token", "race-test")
	r.ServeHTTP(httptest.NewRecorder(), req)
	admission.Lock()
	snapshot := admission.active
	admission.Unlock()
	// After begin returns, no later admission can increase the active count.
	for i := 0; i < 100; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/work", nil))
		if w.Code != 503 {
			t.Fatalf("admitted after drain: %d", w.Code)
		}
	}
	admission.Lock()
	if admission.active != snapshot {
		t.Errorf("late admission: %d -> %d", snapshot, admission.active)
	}
	admission.Unlock()
	close(release)
	wg.Wait()
	WaitAdmittedRequests()
}

func TestDrainWaitsForSSEAndAllowsWebhook(t *testing.T) {
	t.Setenv("DRAIN_TOKEN", "local-test")
	t.Cleanup(func() { admission.Lock(); admission.until = time.Time{}; admission.Unlock() })
	started, release := make(chan struct{}), make(chan struct{})
	r := gin.New()
	r.Use(DrainMiddleware())
	r.POST("/internal/drain", Drain)
	r.GET("/stream", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		fmt.Fprint(c.Writer, "data: token\n\n")
		c.Writer.Flush()
		close(started)
		<-release
		fmt.Fprint(c.Writer, "data: [DONE]\n\n")
	})
	r.POST("/api/payment/sepay/webhook", func(c *gin.Context) { c.Status(200) })
	s := httptest.NewServer(r)
	defer s.Close()
	done := make(chan string, 1)
	go func() {
		resp, err := http.Get(s.URL + "/stream")
		if err != nil {
			done <- err.Error()
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		done <- string(body)
	}()
	<-started
	call := func(action string) string {
		t.Helper()
		req, _ := http.NewRequest("POST", s.URL+"/internal/drain?action="+action, nil)
		req.Header.Set("X-Drain-Token", "local-test")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if body := call("begin"); !strings.Contains(body, `"active_requests":1`) {
		t.Fatalf("admitted stream omitted: %s", body)
	}
	resp, err := http.Post(s.URL+"/api/payment/sepay/webhook", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("webhook blocked: %d", resp.StatusCode)
	}
	close(release)
	if body := <-done; !strings.Contains(body, "[DONE]") {
		t.Fatal(body)
	}
	WaitAdmittedRequests()
	if body := call("status"); !strings.Contains(body, `"active_requests":0`) {
		t.Fatal(body)
	}
	call("cancel")
	admission.Lock()
	admission.until = time.Now().Add(-time.Second)
	admission.Unlock()
	if isDraining() {
		t.Fatal("expired drain lease stayed active")
	}
}
