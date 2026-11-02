package controller

import (
	"context"
	"errors"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	"github.com/TranNgocPhuongHieu/LLM-gateway/dto"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
	"github.com/TranNgocPhuongHieu/LLM-gateway/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRelayRetryCandidates(t *testing.T) (*gin.Context, *relaycommon.RelayInfo, *service.RetryParam) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	oldDB, oldCache, oldRetries := model.DB, common.MemoryCacheEnabled, common.RetryTimes
	model.DB, common.MemoryCacheEnabled, common.RetryTimes = db, true, 5
	t.Cleanup(func() {
		model.DB, common.MemoryCacheEnabled, common.RetryTimes = oldDB, oldCache, oldRetries
		_ = conn.Close()
	})
	const modelName = "gpt-6.1-sol"
	for _, id := range []int{8, 9, 11, 12, 10} {
		priority := int64(1)
		if id == 10 {
			priority = 0
		}
		ch := &model.Channel{Id: id, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled,
			Key: "test-only-key", Priority: &priority, Models: modelName, Group: "default"}
		if err := db.Create(ch).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Cache construction uses the group list from abilities, just as production does.
	if err := db.Create(&model.Ability{Group: "default", Model: modelName, ChannelId: 8, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	model.InitChannelCache()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	info := &relaycommon.RelayInfo{TokenGroup: "default", OriginModelName: modelName, ChannelMeta: &relaycommon.ChannelMeta{}}
	param := &service.RetryParam{Ctx: c, TokenGroup: "default", ModelName: modelName, RequestPath: c.Request.URL.Path, Retry: common.GetPointer(0)}
	return c, info, param
}

func TestRelayRetryCandidatesExhaustPrimariesAndPreserveLastError(t *testing.T) {
	c, info, param := setupRelayRetryCandidates(t)
	upstreamError := types.NewOpenAIError(errors.New("provider unavailable"), types.ErrorCodeBadResponseStatusCode, 429)
	for attempt := 0; attempt < 5; attempt++ {
		param.SetRetry(attempt)
		ch, err := getChannel(c, info, param)
		if err != nil || ch == nil {
			t.Fatalf("attempt %d: channel=%v err=%v", attempt, ch, err)
		}
		if slices.Contains(param.AttemptedChannelIDs, ch.Id) {
			t.Fatalf("channel repeated: %d", ch.Id)
		}
		if (attempt < 4 && ch.Id == 10) || (attempt == 4 && ch.Id != 10) {
			t.Fatalf("wrong priority on attempt %d: channel=%d", attempt, ch.Id)
		}
		param.AttemptedChannelIDs = append(param.AttemptedChannelIDs, ch.Id)
		info.LastError = upstreamError
		if !shouldRetry(c, upstreamError, common.RetryTimes-attempt) {
			t.Fatalf("429 unexpectedly stopped retrying at attempt %d", attempt)
		}
	}
	param.SetRetry(5)
	ch, err := getChannel(c, info, param)
	if ch != nil || err != upstreamError {
		t.Fatalf("lost final provider error: channel=%v err=%v", ch, err)
	}
	if shouldRetry(c, upstreamError, 0) {
		t.Fatal("retry budget was exceeded")
	}
}

func TestRelayRetryCandidatesKeepStopConditions(t *testing.T) {
	c, _, _ := setupRelayRetryCandidates(t)
	for _, code := range []int{400, 408, 504, 524} {
		err := types.NewOpenAIError(errors.New("not retryable"), types.ErrorCodeBadResponseStatusCode, code)
		if shouldRetry(c, err, 5) {
			t.Fatalf("status %d became retryable", code)
		}
	}
	for _, code := range []int{429, 502, 503} {
		err := types.NewOpenAIError(errors.New("retryable"), types.ErrorCodeBadResponseStatusCode, code)
		if !shouldRetry(c, err, 5) {
			t.Fatalf("status %d stopped being retryable", code)
		}
	}
	if shouldRetry(c, nil, 5) {
		t.Fatal("successful response was retried")
	}
	service.GetChannelConstraints(c).AddPin(dto.ChannelPin{ChannelId: 8, Source: dto.PinSourceToken, RetryMode: dto.PinRetrySingleAttempt})
	err := types.NewOpenAIError(errors.New("retryable"), types.ErrorCodeBadResponseStatusCode, 429)
	if shouldRetry(c, err, 5) {
		t.Fatal("single-attempt pin was bypassed")
	}
}

func TestProcessChannelErrorWritesOnlyTerminalFailure(t *testing.T) {
	c, relayInfo, _ := setupRelayRetryCandidates(t)
	if err := model.DB.AutoMigrate(&model.Log{}); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, setting TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	oldLogDB := model.LOG_DB
	oldRedis := common.RedisEnabled
	model.LOG_DB = model.DB
	common.RedisEnabled = false
	t.Cleanup(func() { model.LOG_DB = oldLogDB; common.RedisEnabled = oldRedis })
	oldErrorLogging := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = oldErrorLogging })

	c.Set("id", 1)
	c.Set("username", "retry-test-user")
	c.Set("token_id", 7)
	c.Set("token_name", "test-token")
	c.Set("original_model", "gpt-6.1-sol")
	c.Set("group", "default")
	c.Set(common.RequestIdKey, "retry-final-only")
	c.Set("use_channel", []string{"8", "11"})
	c.Set("channel_id", 11)

	firstError := types.NewOpenAIError(errors.New("first channel unavailable"), types.ErrorCodeBadResponseStatusCode, 503)
	processChannelError(c, *types.NewChannelError(8, 1, "first", false, "", false), firstError, relayInfo, false)
	var count int64
	if err := model.LOG_DB.Model(&model.Log{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("intermediate channel failure was persisted: %d rows", count)
	}

	finalError := types.NewOpenAIError(errors.New("last channel unavailable"), types.ErrorCodeBadResponseStatusCode, 503)
	processChannelError(c, *types.NewChannelError(11, 1, "last", false, "", false), finalError, relayInfo, true)
	var logs []*model.Log
	if err := model.LOG_DB.Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].ChannelId != 11 || logs[0].RequestId != "retry-final-only" {
		t.Fatalf("terminal failure log mismatch: %+v", logs)
	}
	if !strings.Contains(logs[0].Other, "request_final") || !strings.Contains(logs[0].Other, "use_channel") || !strings.Contains(logs[0].Other, "8") || !strings.Contains(logs[0].Other, "11") {
		t.Fatalf("terminal failure lost final/retry metadata: %s", logs[0].Other)
	}
}

func TestXPikiOperationalErrorsRetryWithoutBypassingSafety(t *testing.T) {
	for _, code := range []int{408, 500, 502, 503, 504, 524} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			c, _, _ := setupRelayRetryCandidates(t)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://api.xpiki.com")
			err := types.NewOpenAIError(errors.New("provider failed before output"), types.ErrorCodeBadResponseStatusCode, code)
			if !shouldRetry(c, err, 5) {
				t.Fatalf("XPiki operational status %d did not reach fallback", code)
			}
			if shouldRetry(c, err, 0) {
				t.Fatal("XPiki override bypassed retry budget")
			}
			partial := types.NewOpenAIError(errors.New("partial output"), types.ErrorCodeBadResponseStatusCode, code, types.ErrOptionWithSkipRetry())
			if shouldRetry(c, partial, 5) {
				t.Fatal("XPiki override replayed partial output")
			}
			ctx, cancel := context.WithCancel(c.Request.Context())
			c.Request = c.Request.WithContext(ctx)
			cancel()
			if shouldRetry(c, err, 5) {
				t.Fatal("XPiki override retried after client cancellation")
			}
		})
	}
	c, _, _ := setupRelayRetryCandidates(t)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://api.xpiki.com")
	invalid := types.NewOpenAIError(errors.New("invalid input"), types.ErrorCodeInvalidRequest, 400)
	if shouldRetry(c, invalid, 5) {
		t.Fatal("invalid client input was retried")
	}
	service.GetChannelConstraints(c).AddPin(dto.ChannelPin{ChannelId: 8, Source: dto.PinSourceToken, RetryMode: dto.PinRetrySingleAttempt})
	if shouldRetry(c, types.NewOpenAIError(errors.New("gateway timeout"), types.ErrorCodeBadResponseStatusCode, 504), 5) {
		t.Fatal("single-attempt pin was overridden")
	}
}

func Test9routerTimeoutsRetryToXPikiWithoutBypassingSafety(t *testing.T) {
	for _, code := range []int{408, 504, 524} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			c, info, param := setupRelayRetryCandidates(t)
			routerURL := "http://9router:20128"
			xpikiURL := "https://api.xpiki.com"
			if err := model.DB.Model(&model.Channel{}).Where("id = ?", 10).Updates(map[string]any{"priority": 2, "base_url": routerURL}).Error; err != nil {
				t.Fatal(err)
			}
			if err := model.DB.Model(&model.Channel{}).Where("id IN ?", []int{8, 9, 11, 12}).Update("base_url", xpikiURL).Error; err != nil {
				t.Fatal(err)
			}
			model.InitChannelCache()
			first, err := getChannel(c, info, param)
			if err != nil || first.Id != 10 {
				t.Fatalf("first attempt was not priority-2 9router: %v %v", first, err)
			}
			param.AttemptedChannelIDs = append(param.AttemptedChannelIDs, first.Id)
			timeout := types.NewOpenAIError(errors.New("9router timeout before output"), types.ErrorCodeBadResponseStatusCode, code)
			if !shouldRetry(c, timeout, 5) {
				t.Fatal("9router timeout did not retry")
			}
			param.SetRetry(1)
			info.LastError = timeout
			next, err := getChannel(c, info, param)
			if err != nil || next == nil || next.Id == 10 || !slices.Contains([]int{8, 9, 11, 12}, next.Id) {
				t.Fatalf("9router did not fall back to XPiki: %v %v", next, err)
			}
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, routerURL)
			if shouldRetry(c, timeout, 0) {
				t.Fatal("retry budget bypassed")
			}
			partial := types.NewOpenAIError(errors.New("timeout after output"), types.ErrorCodeBadResponseStatusCode, code, types.ErrOptionWithSkipRetry())
			if shouldRetry(c, partial, 5) {
				t.Fatal("partial answer replayed")
			}
			service.GetChannelConstraints(c).AddPin(dto.ChannelPin{ChannelId: 10, Source: dto.PinSourceToken, RetryMode: dto.PinRetrySingleAttempt})
			if shouldRetry(c, timeout, 5) {
				t.Fatal("single-attempt pin bypassed")
			}
			other, _, _ := setupRelayRetryCandidates(t)
			common.SetContextKey(other, constant.ContextKeyChannelBaseUrl, routerURL)
			ctx, cancel := context.WithCancel(other.Request.Context())
			other.Request = other.Request.WithContext(ctx)
			cancel()
			if shouldRetry(other, timeout, 5) {
				t.Fatal("client cancellation retried")
			}
		})
	}
}

func Test9routerTimeoutRetryScope(t *testing.T) {
	for _, upstream := range []string{"http://9router:20128/v1", "https://9router.trnhieu.dev/v1", "https://9router.trnhieu.dev.attacker.test", "https://example.test", "https://9router@other.test", "invalid"} {
		c, _, _ := setupRelayRetryCandidates(t)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream)
		want := upstream == "http://9router:20128/v1" || upstream == "https://9router.trnhieu.dev/v1"
		for _, code := range []int{408, 504, 524} {
			err := types.NewOpenAIError(errors.New("gateway timeout"), types.ErrorCodeBadResponseStatusCode, code)
			if shouldRetry(c, err, 5) != want {
				t.Fatalf("unexpected retry scope: %s/%d", upstream, code)
			}
		}
		invalid := types.NewOpenAIError(errors.New("invalid request"), types.ErrorCodeInvalidRequest, 400)
		if shouldRetry(c, invalid, 5) {
			t.Fatal("400 input error became retryable")
		}
	}
}

func TestRelayRetryCandidatesRespectConfiguredBudget(t *testing.T) {
	for budget := 0; budget <= 5; budget++ {
		t.Run(strconv.Itoa(budget), func(t *testing.T) {
			c, info, param := setupRelayRetryCandidates(t)
			common.RetryTimes = budget
			upstreamError := types.NewOpenAIError(errors.New("provider unavailable"), types.ErrorCodeBadResponseStatusCode, 429)
			attempts := 0
			for ; param.GetRetry() <= common.RetryTimes; param.IncreaseRetry() {
				ch, err := getChannel(c, info, param)
				if err != nil {
					if err != upstreamError {
						t.Fatalf("unexpected terminal error: %v", err)
					}
					break
				}
				attempts++
				param.AttemptedChannelIDs = append(param.AttemptedChannelIDs, ch.Id)
				info.LastError = upstreamError
				if !shouldRetry(c, upstreamError, common.RetryTimes-param.GetRetry()) {
					break
				}
			}
			if attempts != min(budget+1, 5) {
				t.Fatalf("budget %d allowed %d attempts", budget, attempts)
			}
			if slices.Contains(param.AttemptedChannelIDs, 10) != (budget >= 4) {
				t.Fatalf("incorrect fallback for budget %d: %v", budget, param.AttemptedChannelIDs)
			}
		})
	}
}

func TestRelayRetryCandidatesAutoGroupsRetainLegacySelection(t *testing.T) {
	c, _, param := setupRelayRetryCandidates(t)
	param.TokenGroup = "auto"
	param.SetRetry(1)
	param.AttemptedChannelIDs = []int{8}
	common.SetContextKey(c, constant.ContextKeyTokenAutoGroups, []string{"default"})
	ch, group, err := service.CacheGetRandomSatisfiedChannel(param)
	if err != nil || ch == nil || ch.Id != 10 || group != "default" {
		t.Fatalf("auto-group policy changed: channel=%v group=%s err=%v", ch, group, err)
	}
	if param.GetRetry() != 1 {
		t.Fatalf("auto-group retry counter changed: %d", param.GetRetry())
	}
}

func TestRelayRetryFallbackTo9RouterAfterOneFailure(t *testing.T) {
	c, info, param := setupRelayRetryCandidates(t)
	xpikiURL := "https://api.xpiki.com"
	routerURL := "https://9router.trnhieu.dev"
	for _, id := range []int{8, 9, 11, 12} {
		model.DB.Model(&model.Channel{}).Where("id = ?", id).Update("base_url", xpikiURL)
	}
	model.DB.Model(&model.Channel{}).Where("id = ?", 10).Update("base_url", routerURL)
	model.InitChannelCache()

	// Attempt 0: XPiki channel
	param.SetRetry(0)
	ch0, err := getChannel(c, info, param)
	if err != nil || ch0 == nil || ch0.Id == 10 {
		t.Fatalf("attempt 0 expected xpiki channel, got %v", ch0)
	}
	param.AttemptedChannelIDs = append(param.AttemptedChannelIDs, ch0.Id)

	// Attempt 1 (after 1 failure): with RELAY_MAX_HOST_ATTEMPTS=1, must immediately select 9router (Channel 10)
	param.SetRetry(1)
	ch1, err := getChannel(c, info, param)
	if err != nil || ch1 == nil || ch1.Id != 10 {
		t.Fatalf("attempt 1 expected immediate fallback to 9router (channel 10), got %v err=%v", ch1, err)
	}
}

func TestRelayRetryFallbackTo9RouterAfterTwoFailures(t *testing.T) {
	t.Setenv("RELAY_MAX_HOST_ATTEMPTS", "2")
	c, info, param := setupRelayRetryCandidates(t)
	xpikiURL := "https://api.xpiki.com"
	routerURL := "https://9router.trnhieu.dev"
	for _, id := range []int{8, 9, 11, 12} {
		model.DB.Model(&model.Channel{}).Where("id = ?", id).Update("base_url", xpikiURL)
	}
	model.DB.Model(&model.Channel{}).Where("id = ?", 10).Update("base_url", routerURL)
	model.InitChannelCache()

	// Attempt 0: XPiki channel
	param.SetRetry(0)
	ch0, err := getChannel(c, info, param)
	if err != nil || ch0 == nil || ch0.Id == 10 {
		t.Fatalf("attempt 0 expected xpiki channel, got %v", ch0)
	}
	param.AttemptedChannelIDs = append(param.AttemptedChannelIDs, ch0.Id)

	// Attempt 1: Another XPiki channel (fall ve xpiki them 1 lan nua)
	param.SetRetry(1)
	ch1, err := getChannel(c, info, param)
	if err != nil || ch1 == nil || ch1.Id == 10 || ch1.Id == ch0.Id {
		t.Fatalf("attempt 1 expected distinct xpiki channel, got %v", ch1)
	}
	param.AttemptedChannelIDs = append(param.AttemptedChannelIDs, ch1.Id)

	// Attempt 2: Both XPiki channels failed -> must immediately select 9router (Channel 10)
	param.SetRetry(2)
	ch2, err := getChannel(c, info, param)
	if err != nil || ch2 == nil || ch2.Id != 10 {
		t.Fatalf("attempt 2 expected immediate fallback to 9router (channel 10), got %v err=%v", ch2, err)
	}
}
