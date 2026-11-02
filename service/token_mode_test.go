package service

import (
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRawTextTokenUsageMatchesRetailTokenSemantics(t *testing.T) {
	if got := rawTextTokenUsage(textQuotaSummary{PromptTokens: 100, CompletionTokens: 20, CacheTokens: 30}); got != 120 {
		t.Fatalf("openai-style usage=%d, want 120", got)
	}
	if got := rawTextTokenUsage(textQuotaSummary{IsClaudeUsageSemantic: true, PromptTokens: 100, CompletionTokens: 20, CacheTokens: 30, CacheCreationTokens: 7}); got != 157 {
		t.Fatalf("anthropic usage=%d, want 157", got)
	}
}

func TestTokenModeSettlementDoesNotTouchUserWallet(t *testing.T) {
	oldDB, oldRedis, oldBatch := model.DB, common.RedisEnabled, common.BatchUpdateEnabled
	t.Cleanup(func() { model.DB, common.RedisEnabled, common.BatchUpdateEnabled = oldDB, oldRedis, oldBatch })
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	db, err := gorm.Open(sqlite.Open("file:token-mode-settlement?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	model.DB = db
	if err := db.AutoMigrate(&model.User{}, &model.Token{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.User{Id: 1, Username: "token-mode", Quota: 900}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Token{Id: 2, UserId: 1, Key: "token-mode-key", RemainQuota: 1000, QuotaMode: "token"}).Error; err != nil {
		t.Fatal(err)
	}
	info := &relaycommon.RelayInfo{UserId: 1, TokenId: 2, TokenKey: "token-mode-key", TokenQuotaMode: "token"}
	session, apiErr := NewBillingSession(nil, info, 0)
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	info.TokenUsage = new(int)
	*info.TokenUsage = 150
	if err := session.Settle(999); err != nil {
		t.Fatal(err)
	}
	var user model.User
	var token model.Token
	if err := db.First(&user, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&token, 2).Error; err != nil {
		t.Fatal(err)
	}
	if user.Quota != 900 {
		t.Fatalf("wallet quota=%d, want 900", user.Quota)
	}
	if token.RemainQuota != 850 || token.UsedQuota != 150 {
		t.Fatalf("token quota=(%d,%d), want (850,150)", token.RemainQuota, token.UsedQuota)
	}
}
