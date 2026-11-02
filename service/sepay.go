package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/shopspring/decimal"
)

type SepayTransaction struct {
	ID            string `json:"id"`
	TransactionID string `json:"transaction_id"`
	Type          string `json:"transaction_type"`
	Status        string `json:"transaction_status"`
	Amount        string `json:"transaction_amount"`
	Currency      string `json:"transaction_currency"`
}

type SepayOrder struct {
	OrderID      string             `json:"order_id"`
	Invoice      string             `json:"order_invoice_number"`
	Status       string             `json:"order_status"`
	Amount       string             `json:"order_amount"`
	Currency     string             `json:"order_currency"`
	Transactions []SepayTransaction `json:"transactions"`
}

var sepayHTTP = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}

func SepayAmountMatches(raw string, expected int64) bool {
	if len(raw) > 32 || !regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`).MatchString(raw) {
		return false
	}
	value, err := decimal.NewFromString(raw)
	return err == nil && value.Equal(decimal.NewFromInt(expected))
}

func sepayAPI(ctx context.Context, cfg model.SepayConfig, path string, target any) error {
	base := "https://pgapi.sepay.vn"
	if cfg.Environment == "sandbox" {
		base = "https://pgapi-sandbox.sepay.vn"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(cfg.MerchantID, cfg.SecretKey)
	req.Header.Set("Accept", "application/json")
	res, err := sepayHTTP.Do(req)
	if err != nil {
		return errors.New("SePay API connection failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("SePay API returned HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(target)
}

// VerifySepayPayment uses merchant-scoped API credentials, not the redirect or
// IPN payload, as the final evidence of payment. No write is sent to SePay.
func VerifySepayPayment(ctx context.Context, cfg model.SepayConfig, p *model.SepayPayment, orderID, authenticatedIPNTransaction string) (string, string, error) {
	if p.MerchantID != cfg.MerchantID || p.MerchantEnvironment != cfg.Environment {
		return "", "", model.ErrSepayConflict
	}
	if orderID == "" && p.OrderID != nil {
		orderID = *p.OrderID
	}
	if orderID == "" {
		for page := 1; page <= 3; page++ {
			var list struct {
				Data []SepayOrder `json:"data"`
				Meta struct {
					HasMore bool `json:"has_more"`
				} `json:"meta"`
			}
			path := fmt.Sprintf("/v1/order?per_page=100&page=%d&q=%s", page, url.QueryEscape(p.OrderInvoiceNumber))
			if err := sepayAPI(ctx, cfg, path, &list); err != nil {
				return "", "", err
			}
			for _, candidate := range list.Data {
				if candidate.Invoice == p.OrderInvoiceNumber {
					if orderID != "" && orderID != candidate.OrderID {
						return "", "", model.ErrSepayConflict
					}
					orderID = candidate.OrderID
				}
			}
			if !list.Meta.HasMore {
				break
			}
		}
		if orderID == "" {
			return "", "", errors.New("payment is still pending")
		}
	}
	var detail struct {
		Data SepayOrder `json:"data"`
	}
	// SePay's detail endpoint is keyed by the merchant invoice number. The
	// order_id from IPN is still checked against the response below, but must
	// not be used as the URL identifier (it returns 404 for valid orders).
	if err := sepayAPI(ctx, cfg, "/v1/order/detail/"+url.PathEscape(p.OrderInvoiceNumber), &detail); err != nil {
		return "", "", err
	}
	d := detail.Data
	if d.Invoice != p.OrderInvoiceNumber || d.OrderID != orderID || d.Currency != "VND" || !SepayAmountMatches(d.Amount, p.AmountVND) {
		return "", "", model.ErrSepayConflict
	}
	if d.Status != "CAPTURED" {
		return "", "", errors.New("payment is still pending")
	}
	// Live bank-transfer orders can be CAPTURED without a transaction list.
	// Only an authenticated, amount/status-validated IPN may supply the payment
	// transaction in this case; a background poll alone cannot manufacture it.
	if len(d.Transactions) == 0 && strings.TrimSpace(authenticatedIPNTransaction) != "" {
		return orderID, authenticatedIPNTransaction, nil
	}
	for _, t := range d.Transactions {
		if t.Type == "PAYMENT" && t.Status == "APPROVED" && t.Currency == "VND" && SepayAmountMatches(t.Amount, p.AmountVND) {
			id := t.TransactionID
			if id == "" {
				id = t.ID
			}
			if strings.TrimSpace(id) == "" {
				continue
			}
			return orderID, id, nil
		}
	}
	return "", "", errors.New("no approved payment found")
}
