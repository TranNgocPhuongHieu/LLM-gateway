package model

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SepayPayment stores the immutable monetary and quota snapshot used to settle
// one checkout. Secrets and raw provider payloads are intentionally excluded.
type SepayPayment struct {
	Flow                 string     `json:"flow,omitempty" gorm:"type:varchar(16);default:'gateway'"`
	BankCode             string     `json:"bank_code,omitempty" gorm:"type:varchar(32)"`
	BankAccount          string     `json:"bank_account,omitempty" gorm:"type:varchar(64)"`
	BankHolder           string     `json:"bank_holder,omitempty" gorm:"type:varchar(128)"`
	ID                   int64      `json:"id" gorm:"primaryKey"`
	TopUpID              int        `json:"top_up_id" gorm:"uniqueIndex;not null"`
	UserID               int        `json:"-" gorm:"uniqueIndex:sepay_user_request;not null"`
	RequestID            string     `json:"-" gorm:"type:varchar(64);uniqueIndex:sepay_user_request;not null"`
	MerchantEnvironment  string     `json:"merchant_environment" gorm:"type:varchar(16);not null;uniqueIndex:sepay_order;uniqueIndex:sepay_transaction"`
	MerchantID           string     `json:"-" gorm:"type:varchar(128);not null;uniqueIndex:sepay_order;uniqueIndex:sepay_transaction"`
	OrderID              *string    `json:"order_id,omitempty" gorm:"type:varchar(128);uniqueIndex:sepay_order"`
	OrderInvoiceNumber   string     `json:"order_invoice_number" gorm:"type:varchar(255);uniqueIndex;not null"`
	ProviderTransaction  *string    `json:"transaction_id,omitempty" gorm:"type:varchar(128);uniqueIndex:sepay_transaction"`
	AmountVND            int64      `json:"amount_vnd" gorm:"not null"`
	Credits              int64      `json:"credits" gorm:"not null"`
	QuotaSnapshot        int        `json:"-" gorm:"not null"`
	QuotaPerUnitSnapshot float64    `json:"-" gorm:"not null"`
	ExchangeRateSnapshot float64    `json:"-" gorm:"not null"`
	Currency             string     `json:"currency" gorm:"type:char(3);not null"`
	VerificationStatus   string     `json:"verification_status" gorm:"type:varchar(32);not null;index"`
	VerificationReason   string     `json:"verification_reason,omitempty" gorm:"type:varchar(255)"`
	IPNReceivedAt        *time.Time `json:"ipn_received_at,omitempty" gorm:"column:ipn_received_at"`
	VerifiedAt           *time.Time `json:"verified_at,omitempty"`
	NextReconcileAt      *time.Time `json:"-"`
	ReconcileAttempts    int        `json:"-" gorm:"not null;default:0"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

const (
	SepayVerificationPending = "pending"
	SepayVerificationPaid    = "paid"
	SepayVerificationReview  = "review"
	SepayVerificationVoided  = "voided"
)

const SepayConfigOption = "SepayConfigSecret"
const SepayOrderTTL = 15 * time.Minute

var ErrSepayConflict = errors.New("payment details conflict; administrator review required")

type SepayConfig struct {
	BankCode      string `json:"bank_code"`
	BankAccount   string `json:"bank_account"`
	BankHolder    string `json:"bank_holder"`
	WebhookSecret string `json:"webhook_secret"`
	Enabled       bool   `json:"enabled"`
	Environment   string `json:"environment"`
	MerchantID    string `json:"merchant_id"`
	SecretKey     string `json:"secret_key"`
	IPNSecret     string `json:"ipn_secret"`
}

// Credentials are encrypted with the existing deployment crypto secret. The
// option is excluded from generic settings APIs, including for administrators.
func sepayCipher() (cipher.AEAD, error) {
	if len(common.CryptoSecret) < 16 {
		return nil, errors.New("deployment crypto secret is not configured")
	}
	key := sha256.Sum256([]byte(common.CryptoSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func decodeSepayConfig(raw string) (SepayConfig, error) {
	config := SepayConfig{Environment: "production"}
	if raw == "" {
		return config, nil
	}
	aead, err := sepayCipher()
	if err != nil {
		return config, err
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(data) < aead.NonceSize() {
		return config, errors.New("invalid encrypted payment configuration")
	}
	plain, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte(SepayConfigOption))
	if err != nil {
		return config, errors.New("cannot decrypt payment configuration")
	}
	err = json.Unmarshal(plain, &config)
	return config, err
}

func GetSepayConfig() (SepayConfig, error) {
	var row Option
	err := DB.Where("key = ?", SepayConfigOption).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return SepayConfig{Environment: "production"}, nil
	}
	if err != nil {
		return SepayConfig{}, err
	}
	return decodeSepayConfig(row.Value)
}

func SaveSepayConfig(next SepayConfig) error {
	if len(next.BankCode) > 32 || len(next.BankAccount) > 64 || len(next.BankHolder) > 128 || len(next.WebhookSecret) > 4096 {
		return errors.New("invalid bank configuration length")
	}
	if next.BankCode != "" && !regexp.MustCompile(`^[A-Za-z0-9]+$`).MatchString(next.BankCode) {
		return errors.New("invalid bank code")
	}
	if next.BankAccount != "" && !regexp.MustCompile(`^[A-Za-z0-9]+$`).MatchString(next.BankAccount) {
		return errors.New("invalid bank account")
	}
	if next.WebhookSecret != "" && len(next.WebhookSecret) < 16 {
		return errors.New("bank webhook secret must contain at least 16 characters")
	}
	if len(next.MerchantID) > 128 || len(next.SecretKey) > 4096 || len(next.IPNSecret) > 4096 {
		return errors.New("invalid payment configuration length")
	}
	if next.Environment == "sandbox" && !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return errors.New("Sandbox must use an isolated SQLite test deployment, never the production wallet")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Option{Key: SepayConfigOption}).Error; err != nil {
			return err
		}
		var row Option
		if err := lockForUpdate(tx).Where("key = ?", SepayConfigOption).First(&row).Error; err != nil {
			return err
		}
		old, err := decodeSepayConfig(row.Value)
		if err != nil {
			return err
		}
		if next.SecretKey == "" {
			next.SecretKey = old.SecretKey
		}
		if next.IPNSecret == "" {
			next.IPNSecret = old.IPNSecret
		}
		if next.WebhookSecret == "" {
			next.WebhookSecret = old.WebhookSecret
		}
		if old.BankAccount != next.BankAccount || old.BankCode != next.BankCode || old.BankHolder != next.BankHolder || old.WebhookSecret != next.WebhookSecret {
			var count int64
			if err := tx.Model(&SepayPayment{}).Where("flow = ? AND verification_status <> ?", "bank", SepayVerificationPaid).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("resolve outstanding bank QR orders before changing bank connection")
			}
		}
		if next.Environment != "production" && next.Environment != "sandbox" {
			return errors.New("invalid SePay environment")
		}
		if next.Enabled && (next.SecretKey == "" || next.MerchantID == "" || len(next.IPNSecret) < 16) {
			return errors.New("merchant and secrets are required (IPN secret: at least 16 characters)")
		}
		if old.MerchantID != next.MerchantID || old.Environment != next.Environment || old.SecretKey != next.SecretKey || old.IPNSecret != next.IPNSecret {
			var count int64
			if err := tx.Model(&SepayPayment{}).Where("verification_status <> ?", SepayVerificationPaid).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("resolve outstanding SePay orders before changing credentials")
			}
		}
		aead, err := sepayCipher()
		if err != nil {
			return err
		}
		plain, err := json.Marshal(next)
		if err != nil {
			return err
		}
		nonce := make([]byte, aead.NonceSize())
		if _, err = rand.Read(nonce); err != nil {
			return err
		}
		row.Value = base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, plain, []byte(SepayConfigOption)))
		return tx.Save(&row).Error
	})
}

func SepayQuota(amountVND int64, quotaPerUnit, exchangeRate float64) (int, error) {
	if amountVND < 10_000 || amountVND > 1_000_000 || amountVND%1000 != 0 {
		return 0, errors.New("amount must be 10,000–1,000,000 VND in steps of 1,000")
	}
	return sepayQuotaForVND(amountVND, quotaPerUnit, exchangeRate)
}

func sepayQuotaForVND(amountVND int64, quotaPerUnit, exchangeRate float64) (int, error) {
	if quotaPerUnit <= 0 || exchangeRate <= 0 || math.IsNaN(quotaPerUnit) || math.IsNaN(exchangeRate) || math.IsInf(quotaPerUnit, 0) || math.IsInf(exchangeRate, 0) {
		return 0, errors.New("invalid currency conversion")
	}
	quota, err := common.WalletQuotaFromDecimalStrict(decimal.NewFromInt(amountVND).Mul(decimal.NewFromFloat(quotaPerUnit)).Div(decimal.NewFromFloat(exchangeRate)))
	if err == nil && quota <= 0 {
		err = ErrInvalidTopUpQuota
	}
	return quota, err
}

// SepayBonusPercent returns the promotional bonus for the fixed VND tiers.
// The bonus is applied to credited quota, while the bank transfer amount stays
// equal to the amount shown in the QR request.
func SepayBonusPercent(amountVND int64) int64 {
	switch {
	case amountVND >= 1_000_000:
		return 30
	case amountVND >= 500_000:
		return 20
	case amountVND >= 200_000:
		return 15
	case amountVND >= 100_000:
		return 10
	default:
		return 0
	}
}

func sepayCreditedVND(amountVND int64) int64 {
	bonus := SepayBonusPercent(amountVND)
	return amountVND + amountVND*bonus/100
}

func CreateSepayPayment(userID int, requestID string, amount int64, rate float64, cfg SepayConfig) (*SepayPayment, error) {
	return createSepayPayment(userID, requestID, amount, rate, cfg, "gateway")
}

func SepayBankConfigured(cfg SepayConfig) bool {
	return cfg.Environment == "production" && cfg.BankCode != "" && cfg.BankAccount != "" && cfg.BankHolder != "" && len(cfg.WebhookSecret) >= 16
}

func CreateSepayBankPayment(userID int, requestID string, amount int64, rate float64, cfg SepayConfig) (*SepayPayment, error) {
	if !SepayBankConfigured(cfg) {
		return nil, errors.New("bank QR connection is not configured")
	}
	return createSepayPayment(userID, requestID, amount, rate, cfg, "bank")
}

func createSepayPayment(userID int, requestID string, amount int64, rate float64, cfg SepayConfig, flow string) (*SepayPayment, error) {
	quota, err := sepayQuotaForVND(sepayCreditedVND(amount), common.QuotaPerUnit, rate)
	if err != nil {
		return nil, err
	}
	if bonus := operation_setting.GetActiveTopupBonus(amount, time.Now()); bonus > 0 {
		quota, err = common.WalletQuotaFromDecimalStrict(decimal.NewFromInt(int64(quota)).Mul(decimal.NewFromFloat(1 + bonus)))
		if err != nil {
			return nil, err
		}
	}
	var result SepayPayment
	err = DB.Transaction(func(tx *gorm.DB) error {
		// Serialize checkout creation with config changes and duplicate requests.
		var option Option
		if err := lockForUpdate(tx).Where("key = ?", SepayConfigOption).First(&option).Error; err != nil {
			return err
		}
		current, err := decodeSepayConfig(option.Value)
		if err != nil {
			return err
		}
		if !current.Enabled || current != cfg {
			return errors.New("payment configuration changed; retry checkout")
		}
		err = tx.Where("user_id = ? AND request_id = ?", userID, requestID).First(&result).Error
		if err == nil {
			if result.AmountVND != amount || result.Flow != flow {
				return ErrSepayConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var user User
		if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
			return err
		}
		max, err := topUpQuotaMaxCurrent(quota)
		if err != nil {
			return err
		}
		if user.Quota > max {
			return ErrTopUpQuotaLimitExceeded
		}
		invoice := "SP" + common.GetRandomString(28)
		if flow == "bank" {
			invoice = "NAP" + strings.ToUpper(common.GetRandomString(28))
		}
		creditedVND := sepayCreditedVND(amount)
		order := TopUp{UserId: userID, Amount: creditedVND / 1000, Money: float64(amount), TradeNo: invoice, PaymentMethod: "sepay_bank_transfer", PaymentProvider: PaymentProviderSepay, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		next := time.Now().Add(SepayOrderTTL)
		result = SepayPayment{TopUpID: order.Id, UserID: userID, RequestID: requestID, MerchantEnvironment: cfg.Environment, MerchantID: cfg.MerchantID, OrderInvoiceNumber: invoice, AmountVND: amount, Credits: creditedVND / 1000, QuotaSnapshot: quota, QuotaPerUnitSnapshot: common.QuotaPerUnit, ExchangeRateSnapshot: rate, Currency: "VND", VerificationStatus: SepayVerificationPending, NextReconcileAt: &next}
		result.Flow = flow
		if flow == "bank" {
			result.BankCode = cfg.BankCode
			result.BankAccount = cfg.BankAccount
			result.BankHolder = cfg.BankHolder
			result.NextReconcileAt = nil
		}
		return tx.Create(&result).Error
	})
	return &result, err
}

func GetSepayPayment(invoice string) (*SepayPayment, error) {
	var p SepayPayment
	err := DB.Where("order_invoice_number = ?", invoice).First(&p).Error
	return &p, err
}

// ExpireSepayPaymentIfNeeded closes an unpaid order after the QR lifetime.
func ExpireSepayPaymentIfNeeded(invoice string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var payment SepayPayment
		if err := tx.Where("order_invoice_number = ?", invoice).First(&payment).Error; err != nil {
			return err
		}
		if payment.VerificationStatus != SepayVerificationPending || time.Since(payment.CreatedAt) < SepayOrderTTL {
			return nil
		}
		result := tx.Model(&payment).Where("id = ? AND verification_status = ?", payment.ID, SepayVerificationPending).Updates(map[string]any{
			"verification_status": SepayVerificationVoided,
			"verification_reason": "QR payment expired after 15 minutes",
			"next_reconcile_at":   nil,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return tx.Model(&TopUp{}).Where("id = ? AND status = ?", payment.TopUpID, common.TopUpStatusPending).Update("status", common.TopUpStatusExpired).Error
	})
}

// ExpireStaleSepayPayments closes only unpaid bank QR orders older than the
// documented TTL. Provider-backed orders and paid/review records are untouched.
func ExpireStaleSepayPayments() error {
	cutoff := time.Now().Add(-SepayOrderTTL)
	var payments []SepayPayment
	if err := DB.Where("flow = ? AND verification_status = ? AND created_at < ?", "bank", SepayVerificationPending, cutoff).Find(&payments).Error; err != nil {
		return err
	}
	for _, payment := range payments {
		if err := ExpireSepayPaymentIfNeeded(payment.OrderInvoiceNumber); err != nil {
			return err
		}
	}
	return nil
}

// CancelSepayPayment lets the owner close an unpaid bank QR before its TTL.
// It is deliberately idempotent for already-voided orders and never touches a
// paid or review order, so a late bank callback cannot be mistaken for a user
// cancellation.
func CancelSepayPayment(invoice string, userID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var payment SepayPayment
		if err := lockForUpdate(tx).Where("order_invoice_number = ? AND user_id = ?", invoice, userID).First(&payment).Error; err != nil {
			return err
		}
		if payment.VerificationStatus == SepayVerificationVoided {
			return nil
		}
		if payment.VerificationStatus != SepayVerificationPending {
			return ErrTopUpStatusInvalid
		}
		if err := tx.Model(&payment).Updates(map[string]any{
			"verification_status": SepayVerificationVoided,
			"verification_reason": "cancelled by user",
			"next_reconcile_at":   nil,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&TopUp{}).Where("id = ? AND status = ?", payment.TopUpID, common.TopUpStatusPending).
			Update("status", common.TopUpStatusExpired).Error
	})
}

func MarkSepayReview(invoice, reason string, voided bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var topup TopUp
		if err := lockForUpdate(tx).Where("trade_no = ? AND payment_provider = ?", invoice, PaymentProviderSepay).First(&topup).Error; err != nil {
			return err
		}
		var p SepayPayment
		if err := lockForUpdate(tx).Where("top_up_id = ?", topup.Id).First(&p).Error; err != nil {
			return err
		}
		status := SepayVerificationReview
		if voided {
			status = SepayVerificationVoided
		}
		return tx.Model(&p).Updates(map[string]any{"verification_status": status, "verification_reason": reason, "ipn_received_at": time.Now()}).Error
	})
}

func CompleteSepayPayment(invoice, orderID, transactionID, ip string) (bool, error) {
	return completeSepayPayment(invoice, orderID, transactionID, ip, "gateway")
}

func CompleteSepayBankPayment(invoice, transactionID, ip string) (bool, error) {
	return completeSepayPayment(invoice, "", transactionID, ip, "bank")
}

func completeSepayPayment(invoice, orderID, transactionID, ip, flow string) (bool, error) {
	var payment SepayPayment
	already := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var topup TopUp
		if err := lockForUpdate(tx).Where("trade_no = ?", invoice).First(&topup).Error; err != nil {
			return err
		}
		if topup.PaymentProvider != PaymentProviderSepay {
			return ErrPaymentMethodMismatch
		}
		if err := lockForUpdate(tx).Where("top_up_id = ?", topup.Id).First(&payment).Error; err != nil {
			return err
		}
		if payment.Flow != flow {
			return ErrSepayConflict
		}
		if payment.OrderID != nil && *payment.OrderID != orderID {
			return ErrSepayConflict
		}
		if payment.ProviderTransaction != nil && *payment.ProviderTransaction != transactionID {
			return ErrSepayConflict
		}
		if payment.VerificationStatus == SepayVerificationVoided {
			return ErrSepayConflict
		}
		if topup.Status == common.TopUpStatusSuccess {
			already = true
			return nil
		}
		if topup.Status != common.TopUpStatusPending || (orderID == "" && flow != "bank") || transactionID == "" {
			return ErrTopUpStatusInvalid
		}
		if err := creditTopUpQuota(tx, topup.UserId, payment.QuotaSnapshot, nil); err != nil {
			return err
		}
		now := time.Now()
		if orderID != "" {
			payment.OrderID = &orderID
		}
		payment.ProviderTransaction = &transactionID
		payment.VerifiedAt = &now
		payment.VerificationStatus = SepayVerificationPaid
		payment.VerificationReason = ""
		payment.NextReconcileAt = nil
		if err := tx.Save(&payment).Error; err != nil {
			return err
		}
		return tx.Model(&topup).Updates(map[string]any{"status": common.TopUpStatusSuccess, "complete_time": now.Unix()}).Error
	})
	if err == nil && !already {
		syncCreditUserQuotaCache(payment.UserID, payment.QuotaSnapshot, "sepay topup")
		RecordTopupLog(payment.UserID, fmt.Sprintf("SePay: %d VND / %d credits / %s", payment.AmountVND, payment.Credits, invoice), ip, "sepay_bank_transfer", PaymentProviderSepay)
	}
	return already, err
}

// EnrichSepayTopups is one bounded query per history page, never one per row.
func EnrichSepayTopups(topups []*TopUp) error {
	ids := []int{}
	for _, t := range topups {
		if t.PaymentProvider == PaymentProviderSepay {
			ids = append(ids, t.Id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var payments []SepayPayment
	if err := DB.Where("top_up_id IN ?", ids).Find(&payments).Error; err != nil {
		return err
	}
	byID := make(map[int]*SepayPayment, len(payments))
	for i := range payments {
		byID[payments[i].TopUpID] = &payments[i]
	}
	for _, t := range topups {
		t.Sepay = byID[t.Id]
	}
	return nil
}
