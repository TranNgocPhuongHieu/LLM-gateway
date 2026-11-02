package model

import (
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func sepayTestDB(t *testing.T, postgresDSN string) {
	t.Helper()
	var db *gorm.DB
	var err error
	oldDB, oldLog, oldRedis, oldSecret, oldType := DB, LOG_DB, common.RedisEnabled, common.CryptoSecret, common.MainDatabaseType()
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
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if postgresDSN == "" {
		connection.SetMaxOpenConns(1)
	}
	DB, LOG_DB, common.RedisEnabled, common.CryptoSecret = db, db, false, "sepay-test-encryption-key-32-chars"
	t.Cleanup(func() {
		DB, LOG_DB, common.RedisEnabled, common.CryptoSecret = oldDB, oldLog, oldRedis, oldSecret
		common.SetDatabaseTypes(oldType, oldType)
		_ = connection.Close()
	})
	if err = db.AutoMigrate(&User{}, &TopUp{}, &SepayPayment{}, &Option{}, &Log{}); err != nil {
		t.Fatal(err)
	}
}

func TestSepayQuotaConversion(t *testing.T) {
	for _, tc := range []struct {
		vnd  int64
		want int
	}{{10000, 192308}, {100000, 1923077}, {1000000, 19230769}} {
		got, err := SepayQuota(tc.vnd, 500000, 26000)
		if err != nil || got != tc.want {
			t.Fatalf("%d: got %d %v want %d", tc.vnd, got, err, tc.want)
		}
		credits := float64(got) / 500000 * 26
		if math.Abs(credits-float64(tc.vnd)/1000) > 0.00003 {
			t.Fatal("display currency round trip changed")
		}
	}
	for _, vnd := range []int64{-1000, 0, 999, 1500, 1000001} {
		if _, err := SepayQuota(vnd, 500000, 26000); err == nil {
			t.Fatalf("accepted %d", vnd)
		}
	}
	for _, rate := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := SepayQuota(10000, 500000, rate); err == nil {
			t.Fatal("accepted bad conversion")
		}
	}
}

func TestSepayPromotionTiers(t *testing.T) {
	for _, tc := range []struct {
		amount int64
		bonus  int64
		want   int64
	}{
		{10_000, 0, 10_000},
		{150_000, 10, 165_000},
		{100_000, 10, 110_000},
		{200_000, 15, 230_000},
		{500_000, 20, 600_000},
		{1_000_000, 30, 1_300_000},
	} {
		if got := SepayBonusPercent(tc.amount); got != tc.bonus {
			t.Fatalf("bonus for %d: got %d want %d", tc.amount, got, tc.bonus)
		}
		if got := sepayCreditedVND(tc.amount); got != tc.want {
			t.Fatalf("credited amount for %d: got %d want %d", tc.amount, got, tc.want)
		}
	}
}

func sepayFixture(t *testing.T, requestID string) (SepayConfig, *SepayPayment) {
	t.Helper()
	cfg := SepayConfig{Enabled: true, Environment: "production", MerchantID: "test-merchant", SecretKey: "test-checkout-secret", IPNSecret: "test-ipn-secret-123456789"}
	if err := SaveSepayConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&User{Id: 1, Username: "sepay-test-user", Quota: 7}).Error; err != nil {
		t.Fatal(err)
	}
	p, err := CreateSepayPayment(1, requestID, 100000, 26000, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, p
}

func TestSepayEncryptedConfigAndIdempotentSettlement(t *testing.T) {
	sepayTestDB(t, "")
	cfg, p := sepayFixture(t, "request-1")
	var stored Option
	if err := DB.Where("key = ?", SepayConfigOption).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored.Value, cfg.SecretKey) || strings.Contains(stored.Value, cfg.IPNSecret) {
		t.Fatal("stored plaintext credential")
	}
	loaded, err := GetSepayConfig()
	if err != nil || loaded != cfg {
		t.Fatalf("config roundtrip: %v", err)
	}
	again, err := CreateSepayPayment(1, "request-1", 100000, 26000, cfg)
	if err != nil || again.ID != p.ID {
		t.Fatal("duplicate request created another order")
	}
	if _, err := CreateSepayPayment(1, "request-1", 200000, 26000, cfg); !errors.Is(err, ErrSepayConflict) {
		t.Fatal("accepted duplicate request with new amount")
	}
	changed := cfg
	changed.MerchantID = "different"
	if err := SaveSepayConfig(changed); err == nil {
		t.Fatal("changed merchant with outstanding checkout")
	}
	if err := ManualCompleteTopUp(p.OrderInvoiceNumber, ""); err == nil {
		t.Fatal("manual completion bypassed verification")
	}
	already, err := CompleteSepayPayment(p.OrderInvoiceNumber, "provider-order", "provider-transaction", "")
	if err != nil || already {
		t.Fatalf("first settlement %v", err)
	}
	already, err = CompleteSepayPayment(p.OrderInvoiceNumber, "provider-order", "provider-transaction", "")
	if err != nil || !already {
		t.Fatal("duplicate settlement not idempotent")
	}
	var user User
	if err := DB.First(&user, 1).Error; err != nil {
		t.Fatal(err)
	}
	if user.Quota != 7+p.QuotaSnapshot {
		t.Fatalf("incorrect credited balance %d", user.Quota)
	}
	if _, err := CompleteSepayPayment(p.OrderInvoiceNumber, "different-order", "provider-transaction", ""); !errors.Is(err, ErrSepayConflict) {
		t.Fatal("accepted conflicting provider order")
	}
}

func TestSepaySettlementRollbackAndVoid(t *testing.T) {
	sepayTestDB(t, "")
	_, p := sepayFixture(t, "rollback")
	if err := DB.Model(&User{}).Where("id = ?", 1).Update("quota", common.MaxWalletQuota).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := CompleteSepayPayment(p.OrderInvoiceNumber, "order", "transaction", ""); err == nil {
		t.Fatal("wallet overflow accepted")
	}
	var topup TopUp
	if err := DB.First(&topup, p.TopUpID).Error; err != nil {
		t.Fatal(err)
	}
	if topup.Status != common.TopUpStatusPending {
		t.Fatal("failed credit committed success")
	}
	if err := DB.Model(&User{}).Where("id = ?", 1).Update("quota", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := MarkSepayReview(p.OrderInvoiceNumber, "void", true); err != nil {
		t.Fatal(err)
	}
	if _, err := CompleteSepayPayment(p.OrderInvoiceNumber, "order", "transaction", ""); !errors.Is(err, ErrSepayConflict) {
		t.Fatal("voided order was credited")
	}
}

func TestCancelSepayPaymentClosesPendingOrder(t *testing.T) {
	sepayTestDB(t, "")
	_, p := sepayFixture(t, "cancel-request")
	if err := CancelSepayPayment(p.OrderInvoiceNumber, 1); err != nil {
		t.Fatal(err)
	}
	stored, err := GetSepayPayment(p.OrderInvoiceNumber)
	if err != nil || stored.VerificationStatus != SepayVerificationVoided {
		t.Fatalf("payment was not voided: %v", err)
	}
	var topup TopUp
	if err := DB.First(&topup, p.TopUpID).Error; err != nil {
		t.Fatal(err)
	}
	if topup.Status != common.TopUpStatusExpired {
		t.Fatalf("topup status = %s, want expired", topup.Status)
	}
	if err := CancelSepayPayment(p.OrderInvoiceNumber, 1); err != nil {
		t.Fatalf("cancel should be idempotent: %v", err)
	}
	if err := CancelSepayPayment(p.OrderInvoiceNumber, 2); err == nil {
		t.Fatal("different user cancelled the order")
	}
}

func TestSepayPostgresConcurrentSettlement(t *testing.T) {
	dsn := os.Getenv("SEPAY_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires a dedicated disposable test PostgreSQL database")
	}
	sepayTestDB(t, dsn)
	_, p := sepayFixture(t, "postgres-concurrency")
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := CompleteSepayPayment(p.OrderInvoiceNumber, "parallel-order", "parallel-transaction", "")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var user User
	if err := DB.First(&user, 1).Error; err != nil {
		t.Fatal(err)
	}
	if user.Quota != 7+p.QuotaSnapshot {
		t.Fatalf("parallel callbacks credited %d", user.Quota)
	}
}
