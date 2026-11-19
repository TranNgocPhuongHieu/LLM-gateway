package model

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/driver/postgres"
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
	if err != nil || second.AmountUnits != order.AmountUnits+BinanceAmountStepUnits || second.AmountUSDT != "10.260000" {
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

func binanceAmountTestDB(t *testing.T, postgresDSN string) *gorm.DB {
	t.Helper()
	oldDB, oldLog, oldType, oldRedis := DB, LOG_DB, common.MainDatabaseType(), common.RedisEnabled
	var db *gorm.DB
	var err error
	if postgresDSN == "" {
		db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	} else {
		db, err = gorm.Open(postgres.Open(postgresDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, common.DatabaseTypePostgreSQL)
	}
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if postgresDSN == "" {
		conn.SetMaxOpenConns(1)
	}
	DB, LOG_DB = db, db
	common.RedisEnabled = false
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLog
		common.RedisEnabled = oldRedis
		common.SetDatabaseTypes(oldType, oldType)
		_ = conn.Close()
	})
	if postgresDSN != "" {
		if err := db.Migrator().DropTable(&BinanceTopUp{}, &TopUp{}, &User{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AutoMigrate(&User{}, &TopUp{}, &BinanceTopUp{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// installLegacyBinanceAmountIndex recreates the pre-change unique index.
func installLegacyBinanceAmountIndex(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Migrator().DropIndex(&BinanceTopUp{}, "binance_scope_network_amount"); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX binance_scope_network_amount ON binance_top_ups (account_scope, network, amount_units)").Error; err != nil {
		t.Fatal(err)
	}
}

func binanceAmountIndexIsUnique(t *testing.T, db *gorm.DB) bool {
	t.Helper()
	indexes, err := db.Migrator().GetIndexes(&BinanceTopUp{})
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range indexes {
		if index.Name() == "binance_scope_network_amount" {
			unique, _ := index.Unique()
			return unique
		}
	}
	t.Fatal("binance_scope_network_amount index missing")
	return false
}

func insertBinanceAmountOrder(t *testing.T, db *gorm.DB, network string, units int64, status string, lateUntil time.Time) {
	t.Helper()
	n := units + int64(len(network))*1_000_000_000 + lateUntil.UnixNano()%1_000_000
	topup := TopUp{UserId: 1, Amount: 1, Money: 1, TradeNo: fmt.Sprintf("BN-%s-%d-%s", network, n, status), PaymentProvider: PaymentProviderBinance, Status: common.TopUpStatusPending}
	if err := db.Create(&topup).Error; err != nil {
		t.Fatal(err)
	}
	amount := decimal.NewFromInt(units).Div(decimal.NewFromInt(1_000_000)).StringFixed(6)
	order := BinanceTopUp{TopUpID: topup.Id, UserID: 1, RequestID: topup.TradeNo, AccountScope: "main", TradeNo: topup.TradeNo, Network: network, Coin: BinanceCoinUSDT, DepositAddress: "0xaddress", AmountUnits: units, AmountUSDT: amount, AmountVND: 1, Credits: 1, QuotaSnapshot: 1, Status: status, ExpiresAt: time.Now(), LateReconcileUntil: lateUntil}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
}

func TestAllocateBinanceAmountSkipsOnlyLiveOrders(t *testing.T) {
	db := binanceAmountTestDB(t, "")
	now := time.Now()
	open, closed := now.Add(time.Hour), now.Add(-time.Minute)
	insertBinanceAmountOrder(t, db, BinanceNetworkBEP20, 10_200_000, BinanceOrderPending, open)
	insertBinanceAmountOrder(t, db, BinanceNetworkBEP20, 10_210_000, BinanceOrderExpired, open)
	insertBinanceAmountOrder(t, db, BinanceNetworkBEP20, 10_220_000, BinanceOrderVoided, open)
	insertBinanceAmountOrder(t, db, BinanceNetworkBEP20, 10_230_000, BinanceOrderPaid, open)
	insertBinanceAmountOrder(t, db, BinanceNetworkBEP20, 30_000_000, BinanceOrderExpired, closed)
	insertBinanceAmountOrder(t, db, BinanceNetworkTRC20, 50_000_000, BinanceOrderPending, open)

	for _, tc := range []struct {
		network   string
		candidate int64
		want      int64
	}{
		{BinanceNetworkBEP20, 10_200_000, 10_230_000}, // pending, expired and voided hold; paid frees
		{BinanceNetworkBEP20, 30_000_000, 30_000_000}, // late window closed: exact amount reused
		{BinanceNetworkBEP20, 50_000_000, 50_000_000}, // other network does not hold
		{BinanceNetworkTRC20, 50_000_000, 50_010_000},
	} {
		got, err := allocateBinanceAmount(db, "main", tc.network, tc.candidate, now)
		if err != nil || got != tc.want {
			t.Fatalf("%s %d: got %d err=%v want %d", tc.network, tc.candidate, got, err, tc.want)
		}
	}
}

func TestAllocateBinanceAmountReportsBusySlots(t *testing.T) {
	db := binanceAmountTestDB(t, "")
	open := time.Now().Add(time.Hour)
	for i := int64(0); i < binanceAmountMaxSlots; i++ {
		insertBinanceAmountOrder(t, db, BinanceNetworkBEP20, 10_000_000+i*BinanceAmountStepUnits, BinanceOrderPending, open)
	}
	if _, err := allocateBinanceAmount(db, "main", BinanceNetworkBEP20, 10_000_000, time.Now()); err != ErrBinanceAmountBusy {
		t.Fatalf("expected busy error, got %v", err)
	}
}

func TestMigrateBinanceAmountIndexDropsUniqueness(t *testing.T) {
	db := binanceAmountTestDB(t, "")
	installLegacyBinanceAmountIndex(t, db)
	if !binanceAmountIndexIsUnique(t, db) {
		t.Fatal("legacy index fixture is not unique")
	}
	for i := 0; i < 2; i++ { // idempotent
		if err := migrateBinanceAmountIndex(db); err != nil {
			t.Fatal(err)
		}
	}
	if binanceAmountIndexIsUnique(t, db) {
		t.Fatal("amount index is still unique after migration")
	}
	open := time.Now().Add(time.Hour)
	insertBinanceAmountOrder(t, db, BinanceNetworkBEP20, 10_200_000, BinanceOrderExpired, open.Add(-2*time.Hour))
	insertBinanceAmountOrder(t, db, BinanceNetworkBEP20, 10_200_000, BinanceOrderPending, open)
}

// TestBinancePostgresConcurrentAmountAllocation proves the advisory lock keeps
// amounts unique among live orders once the unique index is gone.
func TestBinancePostgresConcurrentAmountAllocation(t *testing.T) {
	dsn := os.Getenv("BINANCE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires a dedicated disposable test PostgreSQL database")
	}
	db := binanceAmountTestDB(t, dsn)
	installLegacyBinanceAmountIndex(t, db)
	if err := migrateBinanceAmountIndex(db); err != nil {
		t.Fatal(err)
	}
	if binanceAmountIndexIsUnique(t, db) {
		t.Fatal("amount index is still unique after migration")
	}
	const workers = 24
	for i := 1; i <= workers; i++ {
		if err := db.Create(&User{Username: fmt.Sprintf("binance-pg-%d", i), Password: "password", AffCode: fmt.Sprintf("bnpg%d", i)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	var wg sync.WaitGroup
	results := make(chan int64, workers)
	errs := make(chan error, workers)
	for i := 1; i <= workers; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			order, err := CreateBinanceTopUp(BinanceTopUpInput{UserID: userID, RequestID: "req", AccountScope: "main", Network: BinanceNetworkBEP20, DepositAddress: "0xaddress", AmountUnits: 10_200_000, AmountUSDT: "10.200000", AmountVND: 265200, Credits: 265, QuotaSnapshot: 1000, ExchangeRate: "26000", FeePercent: "2", BonusPercent: "0", ExpiresAt: now.Add(15 * time.Minute), LateReconcileUntil: now.Add(48 * time.Hour)})
			if err != nil {
				errs <- err
				return
			}
			results <- order.AmountUnits
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for units := range results {
		if seen[units] {
			t.Fatalf("amount %d allocated twice", units)
		}
		if (units-10_200_000)%BinanceAmountStepUnits != 0 {
			t.Fatalf("amount %d is off the 0.01 grid", units)
		}
		seen[units] = true
	}
	if len(seen) != workers || !seen[10_200_000] {
		t.Fatalf("unexpected allocations: %v", seen)
	}
}
