package service

import (
	"context"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBinancePollerMockMatchesAndSettlesExactlyOnce(t *testing.T) {
	oldDB, oldLog, oldType, oldRedis := model.DB, model.LOG_DB, common.MainDatabaseType(), common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled = oldDB, oldLog, oldRedis
		common.SetDatabaseTypes(oldType, oldType)
		_ = conn.Close()
	})
	if err := db.AutoMigrate(&model.Option{}, &model.User{}, &model.TopUp{}, &model.BinanceTopUp{}, &model.BinanceDeposit{}, &model.BinancePollCheckpoint{}, &model.BinanceSettlementAudit{}, &model.Log{}); err != nil {
		t.Fatal(err)
	}
	if err := model.SaveBinanceConfig(model.BinanceConfig{Enabled: true, RealPollingEnabled: true, AutoSettlementEnabled: true, AccountScope: "main", MinUSDT: "10", MaxUSDT: "100", FeePercent: "0", BonusPercent: "10", USDTToVND: "26000", Networks: map[string]bool{BinanceNetworkBEP20: true, BinanceNetworkTRC20: true}, Addresses: map[string]string{BinanceNetworkBEP20: "0xgood", BinanceNetworkTRC20: "t-good"}, PollIntervalSeconds: 60, LateReconcileSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	if err := model.MarkBinanceAddressesVerified(); err != nil {
		t.Fatal(err)
	}
	user := model.User{Username: "worker-test", Password: "password", AffCode: "worker-aff-1"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	topup := model.TopUp{UserId: user.Id, Amount: 260, Money: 260000, TradeNo: "BN-worker", PaymentMethod: "binance_usdt_BEP20", PaymentProvider: model.PaymentProviderBinance, Status: common.TopUpStatusPending, CreateTime: now.Unix()}
	if err := db.Create(&topup).Error; err != nil {
		t.Fatal(err)
	}
	order := model.BinanceTopUp{TopUpID: topup.Id, UserID: user.Id, RequestID: "worker-1", AccountScope: "main", TradeNo: topup.TradeNo, Network: model.BinanceNetworkBEP20, Coin: model.BinanceCoinUSDT, DepositAddress: "0xgood", AmountUnits: 10000000, AmountUSDT: "10.000000", RequestedAmountUnits: 10000000, AmountVND: 260000, Credits: 260, QuotaSnapshot: 1000, ExchangeRateSnapshot: "26000", FeePercentSnapshot: "0", BonusSnapshot: "0", Status: model.BinanceOrderPending, CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), LateReconcileUntil: now.Add(time.Hour)}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	user2 := model.User{Username: "worker-test-trx", Password: "password", AffCode: "worker-aff-2"}
	if err := db.Create(&user2).Error; err != nil {
		t.Fatal(err)
	}
	topup2 := model.TopUp{UserId: user2.Id, Amount: 520, Money: 520000, TradeNo: "BN-worker-trx", PaymentMethod: "binance_usdt_TRC20", PaymentProvider: model.PaymentProviderBinance, Status: common.TopUpStatusPending, CreateTime: now.Unix()}
	if err := db.Create(&topup2).Error; err != nil {
		t.Fatal(err)
	}
	order2 := model.BinanceTopUp{TopUpID: topup2.Id, UserID: user2.Id, RequestID: "worker-trx", AccountScope: "main", TradeNo: topup2.TradeNo, Network: model.BinanceNetworkTRC20, Coin: model.BinanceCoinUSDT, DepositAddress: "t-good", AmountUnits: 20000000, AmountUSDT: "20.000000", RequestedAmountUnits: 20000000, AmountVND: 520000, Credits: 520, QuotaSnapshot: 2000, ExchangeRateSnapshot: "26000", FeePercentSnapshot: "0", BonusSnapshot: "10", Status: model.BinanceOrderPending, CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), LateReconcileUntil: now.Add(time.Hour)}
	if err := db.Create(&order2).Error; err != nil {
		t.Fatal(err)
	}
	mock := &MockBinanceWalletClient{Restrictions: BinanceAccountRestrictions{EnableReading: true, IPRestrict: true}, Networks: []BinanceNetworkConfig{{Coin: BinanceCoinUSDT, Network: "BSC", DepositEnable: true}, {Coin: BinanceCoinUSDT, Network: "TRX", DepositEnable: true}}, Addresses: map[string]BinanceDepositAddress{"BSC": {Coin: BinanceCoinUSDT, Network: "BSC", Address: "0xgood"}, "TRX": {Coin: BinanceCoinUSDT, Network: "TRX", Address: "t-good"}}, Deposits: []BinanceDepositRecord{{ProviderDepositID: "dep-worker", Coin: BinanceCoinUSDT, Network: "BSC", Address: "0xgood", AmountUnits: 10000000, Status: 1, InsertTime: now, ConfirmTimes: "20"}, {ProviderDepositID: "dep-worker-trx", Coin: BinanceCoinUSDT, Network: "TRX", Address: "t-good", AmountUnits: 20000000, Status: 1, InsertTime: now, ConfirmTimes: "20"}}}
	poller := &BinancePoller{Client: mock, AccountScope: "main", Owner: "test-owner", Now: func() time.Time { return now }}
	if err := poller.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if already, err := model.SettleBinanceDeposit("main", "dep-worker", "BN-worker"); err != nil || !already {
		t.Fatalf("settled deposit was not idempotent: already=%v err=%v", already, err)
	}
	var user2After model.User
	if err := db.First(&user2After, user2.Id).Error; err != nil {
		t.Fatal(err)
	}
	if user2After.Quota != 2000 {
		t.Fatalf("unexpected TRX quota: %d", user2After.Quota)
	}
	// The overlap window replays the same provider row; it must not downgrade
	// the durable settled state or make a later worker eligible to credit again.
	if err := poller.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var replayed model.BinanceDeposit
	if err := db.Where("provider_deposit_id = ?", "dep-worker").First(&replayed).Error; err != nil {
		t.Fatal(err)
	}
	if replayed.MatchStatus != model.BinanceDepositSettled {
		t.Fatalf("duplicate poll downgraded settled deposit: %s", replayed.MatchStatus)
	}
	var userAfter model.User
	if err := db.First(&userAfter, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if userAfter.Quota != 1000 {
		t.Fatalf("unexpected quota: %d", userAfter.Quota)
	}
}
