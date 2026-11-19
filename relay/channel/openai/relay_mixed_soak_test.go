package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	relayconstant "github.com/TranNgocPhuongHieu/LLM-gateway/relay/constant"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
	"github.com/gin-gonic/gin"
)

type soakBody struct {
	ctx  context.Context
	mode int
	read bool
}

func (b *soakBody) Read(p []byte) (int, error) {
	if b.read {
		return 0, io.EOF
	}
	b.read = true
	switch b.mode {
	case 1: // upstream truncation after one frame
		copy(p, []byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"))
		return len("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"), io.ErrUnexpectedEOF
	case 2: // client/upstream cancellation
		<-b.ctx.Done()
		return 0, b.ctx.Err()
	case 3: // delayed token while watchdog/stream timeout is active
		select {
		case <-time.After(1200 * time.Millisecond):
			return 0, io.ErrUnexpectedEOF
		case <-b.ctx.Done():
			return 0, b.ctx.Err()
		}
	default:
		body := "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n"
		copy(p, []byte(body))
		return len(body), nil
	}
}
func (b *soakBody) Close() error { return nil }

// TestMixedSSESoak is opt-in because it intentionally runs for five minutes.
// It uses only in-process mock responses and never contacts a provider.
func TestMixedSSESoak(t *testing.T) {
	if testing.Short() || strings.TrimSpace(os.Getenv("NEWAPI_MIXED_SOAK")) != "1" {
		t.Skip("set NEWAPI_MIXED_SOAK=1 to run the five-minute isolated soak")
	}
	const (
		workers  = 100
		duration = 5 * time.Minute
	)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 1
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	start := time.Now()
	startG := runtime.NumGoroutine()
	var startMem runtime.MemStats
	runtime.ReadMemStats(&startMem)
	var total, ok, failed atomic.Int64
	var wg sync.WaitGroup
	soakCtx, stopSoak := context.WithTimeout(context.Background(), duration)
	defer stopSoak()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for {
				select {
				case <-soakCtx.Done():
					return
				default:
				}
				ctx, cancel := context.WithCancel(context.Background())
				mode := int(total.Load() % 5)
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
				info := &relaycommon.RelayInfo{IsStream: true, RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "soak-model"}}
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: &soakBody{ctx: ctx, mode: mode}}
				if mode == 2 {
					go func() { time.Sleep(10 * time.Millisecond); cancel() }()
				}
				_, err := OaiStreamHandler(c, info, resp)
				cancel()
				total.Add(1)
				successMode := mode == 0 || mode == 4
				if successMode && err == nil && strings.Contains(w.Body.String(), "[DONE]") {
					ok.Add(1)
				} else if !successMode && err != nil {
					failed.Add(1)
				} else {
					failed.Add(1)
				}
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)
	runtime.GC()
	var endMem runtime.MemStats
	runtime.ReadMemStats(&endMem)
	t.Logf("mixed_soak requests=%d success=%d expected_failures=%d duration=%s throughput=%.1f req/s goroutines=%d->%d heap=%d->%d", total.Load(), ok.Load(), failed.Load(), elapsed, float64(total.Load())/elapsed.Seconds(), startG, runtime.NumGoroutine(), startMem.HeapAlloc, endMem.HeapAlloc)
	if total.Load() == 0 || ok.Load() == 0 || failed.Load() == 0 {
		t.Fatalf("mixed soak did not exercise both success and failure paths")
	}
}
