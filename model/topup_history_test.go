package model

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestHistoricalTopUpProvidersRemainReadable(t *testing.T) {
	for _, provider := range []string{
		PaymentProviderEpay, PaymentProviderStripe, PaymentProviderBalance,
		PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake,
	} {
		t.Run(provider, func(t *testing.T) {
			original := TopUp{Id: 1, UserId: 2, Amount: 123, Money: 4.5,
				TradeNo: "fixture-order", PaymentMethod: provider, PaymentProvider: provider,
				CreateTime: 100, CompleteTime: 200, Status: "success"}
			data, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			var restored TopUp
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatal(err)
			}
			if restored != original {
				t.Fatalf("historical %s order changed during serialization", provider)
			}
		})
	}
}

func TestRetainedTopUpSettlementPaths(t *testing.T) {
	for _, tc := range []struct{ name, provider, mode string }{
		{"epay", PaymentProviderEpay, "epay"},
		{"stripe", PaymentProviderStripe, "stripe"},
		{"manual-epay", PaymentProviderEpay, "manual"},
		{"manual-stripe", PaymentProviderStripe, "manual"},
		{"manual-creem", PaymentProviderCreem, "manual"},
		{"manual-waffo", PaymentProviderWaffo, "manual"},
		{"manual-waffo-pancake", PaymentProviderWaffoPancake, "manual"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			connection, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			connection.SetMaxOpenConns(1)
			oldDB, oldLogs, oldRedis := DB, LOG_DB, common.RedisEnabled
			oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB, common.RedisEnabled = db, db, false
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
			t.Cleanup(func() {
				DB, LOG_DB, common.RedisEnabled = oldDB, oldLogs, oldRedis
				common.SetDatabaseTypes(oldMainType, oldLogType)
				_ = connection.Close()
			})
			if err := db.AutoMigrate(&User{}, &TopUp{}, &Log{}); err != nil {
				t.Fatal(err)
			}
			user := User{Id: 1, Username: "cleanup-fixture", Quota: 10}
			if err := db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			order := TopUp{UserId: user.Id, Amount: 2, Money: 3,
				TradeNo: "fixture-order", PaymentMethod: "fixture-method",
				PaymentProvider: tc.provider, Status: common.TopUpStatusPending}
			if err := db.Create(&order).Error; err != nil {
				t.Fatal(err)
			}
			switch tc.mode {
			case "epay":
				_, err = RechargeEpay(order.TradeNo, order.PaymentMethod, "")
			case "stripe":
				err = Recharge(order.TradeNo, "fixture-customer", "")
			default:
				err = ManualCompleteTopUp(order.TradeNo, "")
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := db.First(&user, user.Id).Error; err != nil {
				t.Fatal(err)
			}
			units := order.Amount
			if tc.provider == PaymentProviderStripe {
				units = int64(order.Money)
			}
			if expected := 10 + int(float64(units)*common.QuotaPerUnit); user.Quota != expected {
				t.Fatalf("quota = %d, want %d", user.Quota, expected)
			}
			if err := db.First(&order, order.Id).Error; err != nil {
				t.Fatal(err)
			}
			if order.Status != common.TopUpStatusSuccess || order.CompleteTime == 0 || order.PaymentProvider != tc.provider {
				t.Fatal("settlement changed historical order metadata")
			}
			if tc.mode == "epay" {
				alreadyDone, err := RechargeEpay(order.TradeNo, order.PaymentMethod, "")
				if err != nil || !alreadyDone {
					t.Fatal("duplicate callback was not recognized")
				}
				var again User
				if err := db.First(&again, user.Id).Error; err != nil || again.Quota != user.Quota {
					t.Fatal("duplicate callback changed quota")
				}
			}
		})
	}
}
