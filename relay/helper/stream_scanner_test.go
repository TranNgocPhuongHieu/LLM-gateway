package helper

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
	"github.com/gin-gonic/gin"
)

// A stream that remains active with chunks more frequently than the idle
// timeout must not be terminated by a stale timer event. This regresses the
// old concurrent Ticker.Reset/receive pattern.
func TestStreamScannerActivityResetsIdleTimeout(t *testing.T) {
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 1
	t.Cleanup(func() { constant.StreamingTimeout = old })

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	info := &relaycommon.RelayInfo{IsStream: true, RelayFormat: types.RelayFormatOpenAI}

	reader, writer := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: reader, Header: make(http.Header)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			_, _ = writer.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n"))
			time.Sleep(100 * time.Millisecond)
		}
		_, _ = writer.Write([]byte("data: [DONE]\n\n"))
		_ = writer.Close()
	}()

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		if !strings.Contains(data, `"content":"x"`) {
			sr.Error(io.ErrUnexpectedEOF)
		}
	})
	<-done
	if info.StreamStatus.EndReason == relaycommon.StreamEndReasonTimeout {
		t.Fatalf("active stream was terminated by idle timeout: %s", info.StreamStatus.Summary())
	}
	if info.StreamStatus.TerminalEvidence != "done" {
		t.Fatalf("expected terminal SSE evidence, got %q (%s)", info.StreamStatus.TerminalEvidence, info.StreamStatus.Summary())
	}
	if recorder.Code != 200 {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
}

// Opt-in end-to-end timing check for a genuinely long SSE response. It is
// intentionally excluded from normal CI because it takes three minutes.
func TestLongSSEActivityDoesNotTimeout(t *testing.T) {
	if os.Getenv("NEWAPI_LONG_SSE") != "1" {
		t.Skip("set NEWAPI_LONG_SSE=1 to run the 180-second isolated stream")
	}
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 2
	t.Cleanup(func() { constant.StreamingTimeout = old })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	info := &relaycommon.RelayInfo{IsStream: true, RelayFormat: types.RelayFormatOpenAI}
	reader, writer := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: reader, Header: make(http.Header)}
	const chunks = 180
	started := time.Now()
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		for i := 0; i < chunks; i++ {
			_, _ = io.WriteString(writer, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
			time.Sleep(time.Second)
		}
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
		_ = writer.Close()
	}()
	var received int
	var last time.Time
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		if !strings.Contains(data, `"content":"x"`) {
			sr.Stop(io.ErrUnexpectedEOF)
			return
		}
		received++
		last = time.Now()
	})
	<-writeDone
	if received != chunks {
		t.Fatalf("received %d/%d chunks after %s", received, chunks, time.Since(started))
	}
	if last.Sub(started) < 150*time.Second {
		t.Fatalf("stream completed too early: %s", last.Sub(started))
	}
	if info.StreamStatus.EndReason != relaycommon.StreamEndReasonDone {
		t.Fatalf("expected done, got %s", info.StreamStatus.Summary())
	}
}
