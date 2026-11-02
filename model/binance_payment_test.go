package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBootstrapBinanceConfigFromEnvIsFailClosedAndNonDestructive(t *testing.T) {
	oldDB, oldLog := DB, LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	DB, LOG_DB = db, db
	t.Cleanup(func() { DB, LOG_DB = oldDB, oldLog })
	if err := db.AutoMigrate(&Option{}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(BinanceBSCAddressEnv, "0xbsc")
	t.Setenv(BinanceTRXAddressEnv, "")
	t.Setenv(BinanceAccountScopeEnv, "scope-test")
	created, missing, err := BootstrapBinanceConfigFromEnv()
	if err != nil || !created || len(missing) != 1 || missing[0] != BinanceTRXAddressEnv {
		t.Fatalf("bootstrap: created=%v missing=%v err=%v", created, missing, err)
	}
	cfg, err := GetBinanceConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addresses[BinanceNetworkBEP20] != "0xbsc" || cfg.Networks[BinanceNetworkTRC20] || cfg.CheckoutEnabled || cfg.RealPollingEnabled || cfg.AutoSettlementEnabled || cfg.AddressesVerified {
		t.Fatalf("unexpected fail-closed config: %+v", cfg)
	}
	if _, _, err := BootstrapBinanceConfigFromEnv(); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&Option{}).Where("key = ?", BinanceConfigOption).Update("value", `{"account_scope":"operator","addresses":{"BEP20":"operator-bsc"}}`).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv(BinanceBSCAddressEnv, "replacement")
	created, _, err = BootstrapBinanceConfigFromEnv()
	if err != nil || created {
		t.Fatalf("existing config overwritten: created=%v err=%v", created, err)
	}
	var option Option
	if err := db.First(&option, "key = ?", BinanceConfigOption).Error; err != nil {
		t.Fatal(err)
	}
	if option.Value != `{"account_scope":"operator","addresses":{"BEP20":"operator-bsc"}}` {
		t.Fatalf("existing option changed: %s", option.Value)
	}
}

func TestBinanceBonusTiersBoundaries(t *testing.T) {
	cfg := DefaultBinanceConfig()
	cases := []struct{ amount, want string }{{"10", "0"}, {"20", "10"}, {"50", "15"}, {"100", "20"}}
	for _, tc := range cases {
		amount, err := decimal.NewFromString(tc.amount)
		if err != nil {
			t.Fatal(err)
		}
		got, err := cfg.BonusFor(amount)
		if err != nil || got.String() != tc.want {
			t.Fatalf("amount %s: got %s err=%v", tc.amount, got, err)
		}
	}
}

func TestBinanceCollisionKeepsRequestedCreditSnapshot(t *testing.T) {
	input := BinanceTopUpInput{
		RequestedAmountUnits: 20_000_000,
		AmountUnits:          20_000_001,
		AmountVND:            520_000,
		Credits:              520,
		QuotaSnapshot:        1_000,
		ExchangeRate:         "26000",
		BonusPercent:         "10",
	}
	if err := recomputeBinanceSnapshot(&input); err != nil {
		t.Fatal(err)
	}
	if input.AmountVND != 520_000 || input.Credits != 572 {
		t.Fatalf("collision changed requested credit snapshot: amount_vnd=%d credits=%d", input.AmountVND, input.Credits)
	}
}

func TestExpireStaleBinanceTopUpClosesOrderAndTopUp(t *testing.T) {
	oldDB, oldLog := DB, LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	DB, LOG_DB = db, db
	t.Cleanup(func() { DB, LOG_DB = oldDB, oldLog })
	if err := db.AutoMigrate(&User{}, &TopUp{}, &BinanceTopUp{}); err != nil {
		t.Fatal(err)
	}
	user := User{Username: "expiry-test", Password: "password"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	topup := TopUp{UserId: user.Id, Amount: 260, Money: 260000, TradeNo: "BN-expiry", PaymentProvider: PaymentProviderBinance, Status: common.TopUpStatusPending}
	if err := db.Create(&topup).Error; err != nil {
		t.Fatal(err)
	}
	order := BinanceTopUp{TopUpID: topup.Id, UserID: user.Id, TradeNo: topup.TradeNo, Status: BinanceOrderPending, ExpiresAt: time.Now().Add(-time.Minute), AmountUSDT: "10.200000", AmountUnits: 10200000, DepositAddress: "0xaddress", Network: BinanceNetworkBEP20, Coin: BinanceCoinUSDT}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	if err := ExpireStaleBinanceTopUps(); err != nil {
		t.Fatal(err)
	}
	var gotOrder BinanceTopUp
	if err := db.First(&gotOrder, order.ID).Error; err != nil {
		t.Fatal(err)
	}
	var gotTopUp TopUp
	if err := db.First(&gotTopUp, topup.Id).Error; err != nil {
		t.Fatal(err)
	}
	if gotOrder.Status != BinanceOrderExpired || gotTopUp.Status != common.TopUpStatusExpired {
		t.Fatalf("stale order was not expired: order=%s topup=%s", gotOrder.Status, gotTopUp.Status)
	}
}

func TestBinanceSettlementIsIdempotent(t *testing.T) {
	oldDB, oldLog, oldType, oldRedis := DB, LOG_DB, common.MainDatabaseType(), common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	DB, LOG_DB = db, db
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLog
		common.RedisEnabled = oldRedis
		common.SetDatabaseTypes(oldType, oldType)
		_ = conn.Close()
	})
	if err := db.AutoMigrate(&User{}, &TopUp{}, &BinanceTopUp{}, &BinanceDeposit{}, &BinanceSettlementAudit{}, &Log{}); err != nil {
		t.Fatal(err)
	}
	user := User{Username: "binance-test", Password: "password", Quota: 0}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	topup := TopUp{UserId: user.Id, Amount: 26, Money: 266500, TradeNo: "BN-test", PaymentMethod: "binance_usdt_BEP20", PaymentProvider: PaymentProviderBinance, Status: common.TopUpStatusPending, CreateTime: now.Unix()}
	if err := db.Create(&topup).Error; err != nil {
		t.Fatal(err)
	}
	order := BinanceTopUp{TopUpID: topup.Id, UserID: user.Id, RequestID: "req-1", AccountScope: "main", TradeNo: topup.TradeNo, Network: BinanceNetworkBEP20, Coin: BinanceCoinUSDT, DepositAddress: "0xaddress", AmountUnits: 10250000, AmountUSDT: "10.250000", AmountVND: 266500, Credits: 266, QuotaSnapshot: 1000, ExchangeRateSnapshot: "26000", FeePercentSnapshot: "2.5", BonusSnapshot: "0", Status: BinanceOrderPending, ExpiresAt: now.Add(time.Hour), LateReconcileUntil: now.Add(24 * time.Hour)}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	second, err := CreateBinanceTopUp(BinanceTopUpInput{UserID: user.Id, RequestID: "req-2", AccountScope: "main", Network: BinanceNetworkBEP20, DepositAddress: "0xaddress", AmountUnits: order.AmountUnits, AmountUSDT: order.AmountUSDT, AmountVND: order.AmountVND, Credits: order.Credits, QuotaSnapshot: order.QuotaSnapshot, ExchangeRate: "26000", FeePercent: "2.5", BonusPercent: "0", ExpiresAt: now.Add(time.Hour), LateReconcileUntil: now.Add(24 * time.Hour)})
	if err != nil || second.AmountUnits != order.AmountUnits+1 {
		t.Fatalf("unique amount allocation failed: %+v err=%v", second, err)
	}
	deposit := BinanceDeposit{AccountScope: "main", ProviderDepositID: "dep-1", Coin: BinanceCoinUSDT, Network: BinanceNetworkBEP20, Address: "0xaddress", AmountUnits: order.AmountUnits, AmountUSDT: order.AmountUSDT, Status: BinanceDepositSuccess, InsertTime: now.Add(time.Minute), DetectedAt: now, MatchStatus: BinanceDepositMatched, MatchedOrderID: &order.ID}
	if err := db.Create(&deposit).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&order).Update("matched_deposit_id", deposit.ID).Error; err != nil {
		t.Fatal(err)
	}
	already, err := SettleBinanceDeposit("main", "dep-1", "BN-test")
	if err != nil || already {
		t.Fatalf("first settlement failed: already=%v err=%v", already, err)
	}
	already, err = SettleBinanceDeposit("main", "dep-1", "BN-test")
	if err != nil || !already {
		t.Fatalf("repeat settlement not idempotent: already=%v err=%v", already, err)
	}
	var got User
	if err := db.First(&got, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if got.Quota != 1000 {
		t.Fatalf("double credit or missing credit: %d", got.Quota)
	}
	var topupAfter TopUp
	if err := db.First(&topupAfter, topup.Id).Error; err != nil {
		t.Fatal(err)
	}
	if topupAfter.Status != common.TopUpStatusSuccess {
		t.Fatalf("topup not completed: %s", topupAfter.Status)
	}
	var audit BinanceSettlementAudit
	if err := db.Where("account_scope = ? AND provider_deposit_id = ?", "main", "dep-1").First(&audit).Error; err != nil {
		t.Fatalf("durable settlement audit missing: %v", err)
	}
}
