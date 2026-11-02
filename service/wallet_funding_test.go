package service

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestRetiredSubscriptionsUseWalletFunding(t *testing.T) {
	originalDB := model.DB
	originalRedis, originalBatch := common.RedisEnabled, common.BatchUpdateEnabled
	t.Cleanup(func() {
		model.DB = originalDB
		common.RedisEnabled = originalRedis
		common.BatchUpdateEnabled = originalBatch
	})
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	db, err := gorm.Open(sqlite.Open("file:wallet-feature-cut?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	model.DB = db
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	for i, preference := range []string{"wallet_only", "subscription_only", "wallet_first", "subscription_first"} {
		t.Run(preference, func(t *testing.T) {
			id := 910000 + i
			user := model.User{Id: id, Username: fmt.Sprintf("wallet-cut-%d", i), AffCode: fmt.Sprintf("cut-%d", i), Quota: 1000}
			if err := db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			info := &relaycommon.RelayInfo{UserId: id, IsPlayground: true}
			info.UserSetting.BillingPreference = preference
			ctx, _ := gin.CreateTestContext(nil)
			session, apiErr := NewBillingSession(ctx, info, 100)
			if apiErr != nil {
				t.Fatal(apiErr)
			}
			if session.funding.Source() != BillingSourceWallet {
				t.Fatal("non-wallet funding selected")
			}
			if err := session.Settle(140); err != nil {
				t.Fatal(err)
			}
			if err := session.Settle(140); err != nil {
				t.Fatal(err)
			}
			var balance int
			if err := db.Model(&model.User{}).Where("id = ?", id).Select("quota").Scan(&balance).Error; err != nil {
				t.Fatal(err)
			}
			if balance != 860 {
				t.Fatalf("wallet quota=%d, want 860 after idempotent settlement", balance)
			}
			failed, apiErr := NewBillingSession(ctx, info, 100)
			if apiErr != nil {
				t.Fatal(apiErr)
			}
			failed.Refund(ctx)
			failed.Refund(ctx)
			deadline := time.After(2 * time.Second)
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
			// Wait for the one async refund, then test the no-fallback path.
			for {
				if err := db.Model(&model.User{}).Where("id = ?", id).Select("quota").Scan(&balance).Error; err != nil {
					t.Fatal(err)
				}
				if balance == 860 {
					break
				}
				select {
				case <-deadline:
					t.Fatalf("wallet refund quota=%d, want 860", balance)
				case <-tick.C:
				}
			}
			if err := db.Model(&model.User{}).Where("id = ?", id).Update("quota", 50).Error; err != nil {
				t.Fatal(err)
			}
			if session, apiErr := NewBillingSession(ctx, info, 100); apiErr == nil || session != nil {
				t.Fatal("insufficient wallet must fail without subscription fallback")
			}
		})
	}
}
