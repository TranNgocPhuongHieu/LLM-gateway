package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
)

type sepayTestTransport func(*http.Request) (*http.Response, error)

func (f sepayTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSepayAPIRequiresExactMerchantScopedEvidence(t *testing.T) {
	old := sepayHTTP
	t.Cleanup(func() { sepayHTTP = old })
	cfg := model.SepayConfig{Environment: "production", MerchantID: "fixture", SecretKey: "test-secret"}
	p := &model.SepayPayment{MerchantEnvironment: "production", MerchantID: "fixture", OrderInvoiceNumber: "invoice", AmountVND: 1000}
	for _, tc := range []struct {
		name, status, amount, currency, invoice string
		pass                                    bool
	}{{"paid", "CAPTURED", "1000.00", "VND", "invoice", true}, {"underpaid", "CAPTURED", "999", "VND", "invoice", false}, {"pending", "AUTHENTICATION_NOT_NEEDED", "1000", "VND", "invoice", false}, {"wrong-currency", "CAPTURED", "1000", "USD", "invoice", false}, {"wrong-invoice", "CAPTURED", "1000", "VND", "someone-else", false}} {
		t.Run(tc.name, func(t *testing.T) {
			sepayHTTP = &http.Client{Transport: sepayTestTransport(func(r *http.Request) (*http.Response, error) {
				user, key, ok := r.BasicAuth()
				if !ok || user != cfg.MerchantID || key != cfg.SecretKey || r.URL.Host != "pgapi.sepay.vn" || r.Method != "GET" {
					t.Fatal("unexpected authenticated request")
				}
				if !strings.HasSuffix(r.URL.Path, "/v1/order/detail/invoice") {
					t.Fatalf("detail endpoint must use invoice number, got %s", r.URL.Path)
				}
				body := `{"data":{"order_id":"order","order_invoice_number":"` + tc.invoice + `","order_status":"` + tc.status + `","order_amount":"` + tc.amount + `","order_currency":"` + tc.currency + `","transactions":[{"id":"tx","transaction_type":"PAYMENT","transaction_status":"APPROVED","transaction_amount":"1000","transaction_currency":"VND"}]}}`
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			_, _, err := VerifySepayPayment(context.Background(), cfg, p, "order", "")
			if (err == nil) != tc.pass {
				t.Fatalf("verification result: %v", err)
			}
		})
	}
	for _, raw := range []string{"1000.00", "1000"} {
		if !SepayAmountMatches(raw, 1000) {
			t.Fatal("integer VND rejected")
		}
	}
	for _, raw := range []string{"1000.01", "999", "NaN", "bad"} {
		if SepayAmountMatches(raw, 1000) {
			t.Fatal("invalid VND accepted")
		}
	}
}

func TestSepayBankTransferNeedsAuthenticatedIPNWhenTransactionsAreAbsent(t *testing.T) {
	old := sepayHTTP
	t.Cleanup(func() { sepayHTTP = old })
	cfg := model.SepayConfig{Environment: "production", MerchantID: "fixture", SecretKey: "test-secret"}
	p := &model.SepayPayment{MerchantEnvironment: "production", MerchantID: "fixture", OrderInvoiceNumber: "invoice", AmountVND: 1000}
	for _, tc := range []struct {
		name, status, amount, ipnTransaction string
		pass                                 bool
	}{
		{"authenticated-bank-payment", "CAPTURED", "1000", "bank-transaction", true},
		{"poll-alone", "CAPTURED", "1000", "", false},
		{"unpaid", "AUTHENTICATION_NOT_NEEDED", "1000", "bank-transaction", false},
		{"wrong-amount", "CAPTURED", "999", "bank-transaction", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sepayHTTP = &http.Client{Transport: sepayTestTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/v1/order/detail/invoice" {
					t.Fatal("wrong invoice lookup")
				}
				body := `{"data":{"order_id":"order","order_invoice_number":"invoice","order_status":"` + tc.status + `","order_amount":"` + tc.amount + `","order_currency":"VND","transactions":[]}}`
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			_, _, err := VerifySepayPayment(context.Background(), cfg, p, "order", tc.ipnTransaction)
			if (err == nil) != tc.pass {
				t.Fatalf("verification result: %v", err)
			}
		})
	}
}
