package model

import (
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
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

func TestConfirmSepayBankPayment(t *testing.T) {
	sepayTestDB(t, "")
	cfg, _ := sepayFixture(t, "gateway-original")
	cfg.BankCode, cfg.BankAccount, cfg.BankHolder, cfg.WebhookSecret = "TPBANK", "123", "TEST", "test-bank-webhook-secret"
	if err := SaveSepayConfig(cfg); err != nil {
		t.Fatal(err)
	}
	p, err := CreateSepayBankPayment(1, "manual-bank", 500000, 26000, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"bank:123", "0", "0123", "-1"} {
		if _, err := ConfirmSepayBankPayment(p.OrderInvoiceNumber, id, "received payment", ""); err == nil {
			t.Fatalf("accepted invalid ID %q", id)
		}
	}
	if _, err := ConfirmSepayBankPayment(p.OrderInvoiceNumber, "123", strings.Repeat("x", 256), ""); err == nil {
		t.Fatal("accepted oversized reason")
	}
	if err := DB.Model(p).Update("created_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	if err := ExpireSepayPaymentIfNeeded(p.OrderInvoiceNumber); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		already, err := ConfirmSepayBankPayment(p.OrderInvoiceNumber, "123", "received payment", "")
		if err != nil || already != (i > 0) {
			t.Fatalf("confirmation %d: %v %v", i, already, err)
		}
	}
	if _, err := CompleteSepayBankPayment(p.OrderInvoiceNumber, "bank:123", ""); err != nil {
		t.Fatal(err)
	}
	other, err := CreateSepayBankPayment(1, "manual-other", 500000, 26000, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ConfirmSepayBankPayment(other.OrderInvoiceNumber, "123", "received payment", ""); err == nil {
		t.Fatal("reused transaction credited another order")
	}
	if _, err := ConfirmSepayBankPayment(p.OrderInvoiceNumber, "124", "received payment", ""); err == nil {
		t.Fatal("accepted conflicting identity")
	}
	if err := CancelSepayPayment(other.OrderInvoiceNumber, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfirmSepayBankPayment(other.OrderInvoiceNumber, "125", "received payment", ""); err == nil {
		t.Fatal("accepted cancelled order")
	}
	var user User
	if err := DB.First(&user, 1).Error; err != nil || user.Quota != 7+p.QuotaSnapshot {
		t.Fatalf("quota: %d, %v", user.Quota, err)
	}
}

func TestConfirmSepayBankPaymentWithoutEventID(t *testing.T) {
	sepayTestDB(t, "")
	cfg, _ := sepayFixture(t, "gateway-original")
	cfg.BankCode, cfg.BankAccount, cfg.BankHolder, cfg.WebhookSecret = "TPBANK", "123", "TEST", "test-bank-webhook-secret"
	if err := SaveSepayConfig(cfg); err != nil {
		t.Fatal(err)
	}
	var before User
	if err := DB.First(&before, 1).Error; err != nil {
		t.Fatal(err)
	}
	p, err := CreateSepayBankPayment(1, "manual-no-id", 500000, 26000, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		already, err := ConfirmSepayBankPayment(p.OrderInvoiceNumber, "", "", "")
		if err != nil || already != (i > 0) {
			t.Fatalf("confirmation %d: %v %v", i, already, err)
		}
	}
	stored, err := GetSepayPayment(p.OrderInvoiceNumber)
	if err != nil || stored.ProviderTransaction == nil || *stored.ProviderTransaction != "manual:"+p.OrderInvoiceNumber || stored.VerificationReason != "Manually confirmed by administrator" {
		t.Fatalf("stored manual confirmation: %+v %v", stored, err)
	}
	// The delayed bank webhook must not credit again, and its event must stay bound to this order.
	already, err := CompleteSepayBankPayment(p.OrderInvoiceNumber, "bank:555", "")
	if err != nil || !already {
		t.Fatalf("late webhook: %v %v", already, err)
	}
	if already, err := CompleteSepayBankPayment(p.OrderInvoiceNumber, "bank:555", ""); err != nil || !already {
		t.Fatalf("webhook retry: %v %v", already, err)
	}
	if _, err := CompleteSepayBankPayment(p.OrderInvoiceNumber, "bank:556", ""); err == nil {
		t.Fatal("accepted a second bank event for the same order")
	}
	other, err := CreateSepayBankPayment(1, "manual-no-id-other", 500000, 26000, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompleteSepayBankPayment(other.OrderInvoiceNumber, "bank:555", ""); err == nil {
		t.Fatal("reused bank event credited another order")
	}
	var after User
	if err := DB.First(&after, 1).Error; err != nil || after.Quota != before.Quota+p.QuotaSnapshot {
		t.Fatalf("quota: %d -> %d, %v", before.Quota, after.Quota, err)
	}
}

func TestSepayPromotionTiers(t *testing.T) {
	for _, tc := range []struct {
		amount int64
		bonus  int64
		want   int64
	}{
		{10_000, 0, 10_000},
		{99_000, 0, 99_000},
		{150_000, 50, 157_500},
		{100_000, 50, 105_000},
		{200_000, 75, 215_000},
		{213_000, 75, 228_975},
		{500_000, 100, 550_000},
		{1_000_000, 150, 1_150_000},
	} {
		if got := SepayBonusPermille(tc.amount); got != tc.bonus {
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
