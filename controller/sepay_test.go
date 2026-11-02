package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type sepayFixtureTransport func(*http.Request) (*http.Response, error)

func (f sepayFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSepaySignatureGolden(t *testing.T) {
	fields := map[string]string{"order_amount": "1000", "merchant": "FIXTURE", "currency": "VND", "operation": "PURCHASE", "order_description": "Test", "order_invoice_number": "INV_1", "payment_method": "BANK_TRANSFER"}
	if got := sepayCheckoutSignature(fields, "fixture-secret"); got != "M4kONQ1yVOoUaHA7quGJSrsgVBGXXbnQHjm+aNWJQxc=" {
		t.Fatalf("unexpected signature %s", got)
	}
	fields["ignored"] = "not signed"
	if got := sepayCheckoutSignature(fields, "fixture-secret"); got != "M4kONQ1yVOoUaHA7quGJSrsgVBGXXbnQHjm+aNWJQxc=" {
		t.Fatal("unexpected signed field")
	}
}

func TestSepayBankWebhookAuthenticatedSettlement(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	oldDB, oldLog, oldRedis, oldSecret, oldType := model.DB, model.LOG_DB, common.RedisEnabled, common.CryptoSecret, common.MainDatabaseType()
	model.DB, model.LOG_DB, common.RedisEnabled, common.CryptoSecret = db, db, false, "bank-webhook-fixture-crypto-secret"
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.CryptoSecret = oldDB, oldLog, oldRedis, oldSecret
		common.SetDatabaseTypes(oldType, oldType)
		_ = conn.Close()
	})
	if err := db.AutoMigrate(&model.Option{}, &model.User{}, &model.TopUp{}, &model.SepayPayment{}, &model.Log{}); err != nil {
		t.Fatal(err)
	}
	cfg := model.SepayConfig{Enabled: true, Environment: "production", MerchantID: "fixture", SecretKey: "fixture-merchant", IPNSecret: "fixture-ipn-12345678", BankCode: "TPBANK", BankAccount: "123456789", BankHolder: "FIXTURE USER", WebhookSecret: "fixture-bank-key-123456789"}
	if err := model.SaveSepayConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.User{Id: 1, Username: "bank-fixture", Quota: 7}).Error; err != nil {
		t.Fatal(err)
	}
	p, err := model.CreateSepayBankPayment(1, "bank-request", 1000, 26000, cfg)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/webhook", SepayBankWebhook)
	invoke := func(auth, body, contentType string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/webhook", strings.NewReader(body))
		req.Header.Set("Authorization", auth)
		req.Header.Set("Content-Type", contentType)
		router.ServeHTTP(rec, req)
		return rec.Code
	}
	event := sepayBankWebhook{ID: 123, Content: "payment " + p.OrderInvoiceNumber, TransferType: "in", TransferAmount: 1000, Gateway: "TPBank", AccountNumber: cfg.BankAccount}
	payload, _ := json.Marshal(event)
	for _, tc := range []struct {
		name, auth, body, contentType string
		want                          int
	}{
		{"missing-key", "", string(payload), "application/json", 401},
		{"wrong-key", "Apikey wrong", string(payload), "application/json", 401},
		{"wrong-content-type", "Apikey " + cfg.WebhookSecret, string(payload), "text/plain", 415},
		{"bad-json", "Apikey " + cfg.WebhookSecret, "bad-json", "application/json", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := invoke(tc.auth, tc.body, tc.contentType); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
	for _, kind := range []string{"outgoing", "wrong-amount", "wrong-bank", "wrong-account", "unknown-reference", "ambiguous-reference"} {
		bad := event
		want := 409
		switch kind {
		case "outgoing":
			bad.TransferType = "out"
			want = 400
		case "wrong-amount":
			bad.TransferAmount = 2000
		case "wrong-bank":
			bad.Gateway = "OtherBank"
		case "wrong-account":
			bad.AccountNumber = "987654321"
		case "unknown-reference":
			bad.Content = "unrelated transfer"
			want = 200
		case "ambiguous-reference":
			bad.Content += " NAP" + strings.Repeat("A", 28)
		}
		body, _ := json.Marshal(bad)
		if got := invoke("Apikey "+cfg.WebhookSecret, string(body), "application/json"); got != want {
			t.Fatalf("%s got %d want %d", kind, got, want)
		}
	}
	var user model.User
	if err := db.First(&user, 1).Error; err != nil || user.Quota != 7 {
		t.Fatal("invalid webhook changed wallet")
	}
	for i := 0; i < 3; i++ {
		if got := invoke("Apikey "+cfg.WebhookSecret, string(payload), "application/json"); got != 200 {
			t.Fatalf("valid callback/replay: %d", got)
		}
	}
	if err := db.First(&user, 1).Error; err != nil || user.Quota != 7+p.QuotaSnapshot {
		t.Fatal("callback was not credited exactly once")
	}
	other, err := model.CreateSepayBankPayment(1, "bank-request-2", 1000, 26000, cfg)
	if err != nil {
		t.Fatal(err)
	}
	event.Content = other.OrderInvoiceNumber
	body, _ := json.Marshal(event)
	if got := invoke("Apikey "+cfg.WebhookSecret, string(body), "application/json"); got == 200 {
		t.Fatal("reused provider transaction credited another order")
	}
	if err := db.First(&user, 1).Error; err != nil || user.Quota != 7+p.QuotaSnapshot {
		t.Fatal("reused transaction changed wallet")
	}
}

func TestSepayIPNEndToEndWithoutExternalNetwork(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	conn.SetMaxOpenConns(1)
	oldDB, oldLog, oldRedis, oldSecret, oldType, oldTransport := model.DB, model.LOG_DB, common.RedisEnabled, common.CryptoSecret, common.MainDatabaseType(), http.DefaultTransport
	model.DB, model.LOG_DB, common.RedisEnabled, common.CryptoSecret = db, db, false, "controller-payment-test-secret-32"
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.CryptoSecret = oldDB, oldLog, oldRedis, oldSecret
		http.DefaultTransport = oldTransport
		common.SetDatabaseTypes(oldType, oldType)
		_ = conn.Close()
	})
	if err := db.AutoMigrate(&model.Option{}, &model.User{}, &model.TopUp{}, &model.SepayPayment{}, &model.Log{}); err != nil {
		t.Fatal(err)
	}
	cfg := model.SepayConfig{Enabled: true, Environment: "production", MerchantID: "fixture", SecretKey: "fixture-merchant-key", IPNSecret: "fixture-ipn-key-12345678"}
	if err := model.SaveSepayConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.User{Id: 1, Username: "ipn-user", Quota: 0}).Error; err != nil {
		t.Fatal(err)
	}
	p, err := model.CreateSepayPayment(1, "ipn-request", 1000, 26000, cfg)
	if err != nil {
		t.Fatal(err)
	}
	apiUnavailable := false
	http.DefaultTransport = sepayFixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Host != "pgapi.sepay.vn" {
			t.Fatal("unexpected network request")
		}
		if apiUnavailable {
			return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		if r.URL.Path == "/v1/order" {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[{"order_id":"sepay-order","order_invoice_number":"` + p.OrderInvoiceNumber + `"}],"meta":{"has_more":false}}`))}, nil
		}
		if r.URL.Path != "/v1/order/detail/"+p.OrderInvoiceNumber {
			t.Fatal("wrong detail lookup")
		}
		// Live bank-transfer detail responses can omit transaction records.
		body := `{"data":{"order_id":"sepay-order","order_invoice_number":"` + p.OrderInvoiceNumber + `","order_status":"CAPTURED","order_currency":"VND","order_amount":"1000.00","transactions":[]}}`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	router := gin.New()
	router.POST("/ipn", SepayIPN)
	event := sepayIPN{Timestamp: 1757058220, NotificationType: "ORDER_PAID", Order: sepayOrder{OrderID: "sepay-order", OrderInvoiceNumber: p.OrderInvoiceNumber, OrderStatus: "CAPTURED", OrderCurrency: "VND", OrderAmount: "1000.00"}, Transaction: sepayTransaction{ID: "ipn-uuid", TransactionID: "public-tx", TransactionType: "PAYMENT", TransactionStatus: "APPROVED", TransactionAmount: "1000", TransactionCurrency: "VND"}}
	invoke := func(secret, body string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/ipn", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Secret-Key", secret)
		router.ServeHTTP(rec, req)
		return rec.Code
	}
	payload, _ := json.Marshal(event)
	if got := invoke("wrong", string(payload)); got != 401 {
		t.Fatalf("bad secret: %d", got)
	}
	if got := invoke(cfg.IPNSecret, "bad-json"); got != 400 {
		t.Fatalf("bad json: %d", got)
	}
	bad := event
	bad.Order.OrderAmount = "2000"
	wrongAmount, _ := json.Marshal(bad)
	if got := invoke(cfg.IPNSecret, string(wrongAmount)); got != 409 {
		t.Fatalf("bad amount: %d", got)
	}
	apiUnavailable = true
	if got := invoke(cfg.IPNSecret, string(payload)); got != 503 {
		t.Fatalf("API outage: %d", got)
	}
	pending, err := model.GetSepayPayment(p.OrderInvoiceNumber)
	if err != nil || pending.IPNReceivedAt == nil || pending.ProviderTransaction == nil {
		t.Fatal("authenticated IPN evidence was not retained for reconciliation")
	}
	apiUnavailable = false
	if err := reconcileSepay(context.Background(), pending); err != nil {
		t.Fatalf("automatic reconciliation after API recovery: %v", err)
	}
	for i := 0; i < 3; i++ {
		if got := invoke(cfg.IPNSecret, string(payload)); got != 200 {
			t.Fatalf("IPN attempt %d: %d", i, got)
		}
	}
	var user model.User
	if err := db.First(&user, 1).Error; err != nil {
		t.Fatal(err)
	}
	if user.Quota != p.QuotaSnapshot {
		t.Fatalf("duplicate IPN changed quota: %d", user.Quota)
	}
	configRouter := gin.New()
	configRouter.GET("/config", GetSepayAdminConfig)
	rec := httptest.NewRecorder()
	configRouter.ServeHTTP(rec, httptest.NewRequest("GET", "/config", nil))
	if strings.Contains(rec.Body.String(), cfg.SecretKey) || strings.Contains(rec.Body.String(), cfg.IPNSecret) {
		t.Fatal("credential exposed by config DTO")
	}
}
