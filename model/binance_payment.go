package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	PaymentProviderBinance = "binance"
	BinanceCoinUSDT        = "USDT"
	BinanceNetworkBEP20    = "BEP20"
	BinanceNetworkTRC20    = "TRC20"
	BinanceDepositSuccess  = 1

	BinanceOrderPending = "pending"
	BinanceOrderPaid    = "paid"
	BinanceOrderPartial = "partial"
	BinanceOrderReview  = "review"
	BinanceOrderExpired = "expired"
	BinanceOrderVoided  = "voided"

	BinanceDepositUnmatched = "unmatched"
	BinanceDepositMatched   = "matched"
	BinanceDepositSettled   = "settled"
	BinanceDepositReview    = "review"
)

const BinanceConfigOption = "BinanceUSDTConfig"

const (
	BinanceBSCAddressEnv   = "BINANCE_BSC_ADDRESS"
	BinanceTRXAddressEnv   = "BINANCE_TRX_ADDRESS"
	BinanceAccountScopeEnv = "BINANCE_ACCOUNT_SCOPE"
)

type BinanceConfig struct {
	Enabled               bool               `json:"enabled"`
	CheckoutEnabled       bool               `json:"checkout_enabled"`
	RealPollingEnabled    bool               `json:"real_polling_enabled"`
	AutoSettlementEnabled bool               `json:"auto_settlement_enabled"`
	AddressesVerified     bool               `json:"addresses_verified"`
	AccountScope          string             `json:"account_scope"`
	MinUSDT               string             `json:"min_usdt"`
	MaxUSDT               string             `json:"max_usdt"`
	FeePercent            string             `json:"fee_percent"`
	BonusPercent          string             `json:"bonus_percent"`
	BonusTiers            []BinanceBonusTier `json:"bonus_tiers,omitempty"`
	USDTToVND             string             `json:"usdt_to_vnd"`
	Networks              map[string]bool    `json:"networks"`
	Addresses             map[string]string  `json:"addresses"`
	PollIntervalSeconds   int                `json:"poll_interval_seconds"`
	LateReconcileSeconds  int                `json:"late_reconcile_seconds"`
	ConfirmationMinimum   int                `json:"confirmation_minimum"`
}

type BinanceBonusTier struct {
	MinUSDT string `json:"min_usdt"`
	MaxUSDT string `json:"max_usdt,omitempty"`
	Percent string `json:"bonus_percent"`
}

func DefaultBinanceConfig() BinanceConfig {
	return BinanceConfig{AccountScope: "main", MinUSDT: "10", MaxUSDT: "100", FeePercent: "0", BonusPercent: "0", BonusTiers: []BinanceBonusTier{{MinUSDT: "10", MaxUSDT: "20", Percent: "0"}, {MinUSDT: "20", MaxUSDT: "50", Percent: "10"}, {MinUSDT: "50", MaxUSDT: "100", Percent: "15"}, {MinUSDT: "100", Percent: "20"}}, USDTToVND: "26000", Networks: map[string]bool{BinanceNetworkBEP20: true, BinanceNetworkTRC20: true}, Addresses: map[string]string{}, PollIntervalSeconds: 60, LateReconcileSeconds: 172800, ConfirmationMinimum: 0}
}

func (cfg BinanceConfig) BonusFor(amount decimal.Decimal) (decimal.Decimal, error) {
	if len(cfg.BonusTiers) == 0 {
		v, err := decimal.NewFromString(cfg.BonusPercent)
		return v, err
	}
	for _, tier := range cfg.BonusTiers {
		min, err := decimal.NewFromString(tier.MinUSDT)
		if err != nil {
			return decimal.Zero, errors.New("invalid Binance bonus tier")
		}
		if amount.LessThan(min) {
			continue
		}
		if tier.MaxUSDT != "" {
			max, err := decimal.NewFromString(tier.MaxUSDT)
			if err != nil {
				return decimal.Zero, errors.New("invalid Binance bonus tier")
			}
			if amount.GreaterThanOrEqual(max) {
				continue
			}
		}
		v, err := decimal.NewFromString(tier.Percent)
		if err != nil || v.IsNegative() {
			return decimal.Zero, errors.New("invalid Binance bonus tier")
		}
		return v, nil
	}
	return decimal.Zero, nil
}

// BootstrapBinanceConfigFromEnv creates the non-secret application
// configuration once, when the database has no BinanceUSDTConfig option.
// Environment values are intentionally limited to wallet addresses and the
// account scope; API credentials are never read or persisted here. The
// resulting configuration is fail-closed until an operator verifies the
// addresses and explicitly enables the feature flags.
func BootstrapBinanceConfigFromEnv() (created bool, missing []string, err error) {
	if DB == nil {
		return false, nil, errors.New("database is not initialized")
	}
	var option Option
	err = DB.Where("key = ?", BinanceConfigOption).First(&option).Error
	if err == nil {
		return false, nil, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil, err
	}

	cfg := DefaultBinanceConfig()
	cfg.Enabled = false
	cfg.CheckoutEnabled = false
	cfg.RealPollingEnabled = false
	cfg.AutoSettlementEnabled = false
	cfg.AddressesVerified = false
	if scope := strings.TrimSpace(os.Getenv(BinanceAccountScopeEnv)); scope != "" {
		cfg.AccountScope = scope
	}
	cfg.Addresses = map[string]string{
		BinanceNetworkBEP20: strings.TrimSpace(os.Getenv(BinanceBSCAddressEnv)),
		BinanceNetworkTRC20: strings.TrimSpace(os.Getenv(BinanceTRXAddressEnv)),
	}
	cfg.Networks = map[string]bool{}
	for network, address := range cfg.Addresses {
		if address == "" {
			missing = append(missing, map[string]string{
				BinanceNetworkBEP20: BinanceBSCAddressEnv,
				BinanceNetworkTRC20: BinanceTRXAddressEnv,
			}[network])
			continue
		}
		cfg.Networks[network] = true
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return false, missing, err
	}
	// Recheck inside the write transaction so concurrent instances never
	// replace an operator-created configuration.
	err = DB.Transaction(func(tx *gorm.DB) error {
		var existing Option
		lookup := tx.Where("key = ?", BinanceConfigOption).First(&existing)
		if lookup.Error == nil {
			created = false
			return nil
		}
		if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
			return lookup.Error
		}
		if err := tx.Create(&Option{Key: BinanceConfigOption, Value: string(raw)}).Error; err != nil {
			// A concurrent creator wins; do not overwrite it.
			var winner Option
			if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(strings.ToLower(err.Error()), "unique") {
				if findErr := tx.Where("key = ?", BinanceConfigOption).First(&winner).Error; findErr == nil {
					created = false
					return nil
				}
			}
			return err
		}
		created = true
		return nil
	})
	return created, missing, err
}

func GetBinanceConfig() (BinanceConfig, error) {
	cfg := DefaultBinanceConfig()
	var option Option
	err := DB.Where("key = ?", BinanceConfigOption).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	var stored BinanceConfig
	if err := json.Unmarshal([]byte(option.Value), &stored); err != nil {
		return BinanceConfig{}, errors.New("invalid Binance configuration")
	}
	cfg = stored
	if cfg.AccountScope == "" {
		cfg.AccountScope = "main"
	}
	if cfg.MinUSDT == "" {
		cfg.MinUSDT = "10"
	}
	if cfg.MaxUSDT == "" {
		cfg.MaxUSDT = "10000"
	}
	if cfg.FeePercent == "" {
		cfg.FeePercent = "0"
	}
	if cfg.BonusPercent == "" {
		cfg.BonusPercent = "0"
	}
	if cfg.USDTToVND == "" {
		cfg.USDTToVND = "26000"
	}
	if cfg.PollIntervalSeconds == 0 {
		cfg.PollIntervalSeconds = 60
	}
	if cfg.LateReconcileSeconds == 0 {
		cfg.LateReconcileSeconds = 172800
	}
	if cfg.Networks == nil {
		cfg.Networks = map[string]bool{}
	}
	if cfg.Addresses == nil {
		cfg.Addresses = map[string]string{}
	}
	return cfg, nil
}

func SaveBinanceConfig(cfg BinanceConfig) error {
	previous, previousErr := GetBinanceConfig()
	if previousErr != nil {
		return previousErr
	}
	// This flag is an attestation produced only by the read-only diagnostic;
	// never accept a client-supplied true value as proof.
	cfg.AddressesVerified = previous.AddressesVerified && cfg.AddressesVerified
	if previous.AccountScope != "" && (previous.Addresses[BinanceNetworkBEP20] != cfg.Addresses[BinanceNetworkBEP20] || previous.Addresses[BinanceNetworkTRC20] != cfg.Addresses[BinanceNetworkTRC20] || previous.Networks[BinanceNetworkBEP20] != cfg.Networks[BinanceNetworkBEP20] || previous.Networks[BinanceNetworkTRC20] != cfg.Networks[BinanceNetworkTRC20]) {
		cfg.AddressesVerified = false
	}
	if cfg.AccountScope == "" || cfg.MinUSDT == "" || cfg.MaxUSDT == "" || cfg.USDTToVND == "" || cfg.PollIntervalSeconds < 10 || cfg.LateReconcileSeconds < 0 {
		return errors.New("invalid Binance configuration")
	}
	if cfg.Networks == nil {
		cfg.Networks = map[string]bool{}
	}
	if cfg.CheckoutEnabled && !cfg.AddressesVerified {
		return errors.New("verify Binance network addresses before enabling checkout")
	}
	if cfg.Addresses == nil {
		cfg.Addresses = map[string]string{}
	}
	for _, network := range []string{BinanceNetworkBEP20, BinanceNetworkTRC20} {
		if cfg.Networks[network] && strings.TrimSpace(cfg.Addresses[network]) == "" {
			return fmt.Errorf("missing Binance %s address", network)
		}
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&Option{Key: BinanceConfigOption, Value: string(raw)}).Error
	})
}

func MarkBinanceAddressesVerified() error {
	cfg, err := GetBinanceConfig()
	if err != nil {
		return err
	}
	cfg.AddressesVerified = true
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&Option{Key: BinanceConfigOption, Value: string(raw)}).Error
	})
}

// BinanceTopUp stores a monetary/quota snapshot for one unique-amount order.
// AmountUnits is USDT scaled by 1e6; it is the matching and uniqueness key.
type BinanceTopUp struct {
	ID                   int64           `json:"id" gorm:"primaryKey"`
	TopUpID              int             `json:"top_up_id" gorm:"uniqueIndex;not null"`
	UserID               int             `json:"user_id" gorm:"index;not null;uniqueIndex:binance_user_request"`
	RequestID            string          `json:"-" gorm:"type:varchar(64);not null;uniqueIndex:binance_user_request"`
	AccountScope         string          `json:"-" gorm:"type:varchar(128);not null;default:'unverified';uniqueIndex:binance_scope_network_amount"`
	TradeNo              string          `json:"trade_no" gorm:"uniqueIndex;type:varchar(255);not null"`
	Network              string          `json:"network" gorm:"type:varchar(16);not null;uniqueIndex:binance_scope_network_amount"`
	Coin                 string          `json:"coin" gorm:"type:varchar(16);not null;default:'USDT'"`
	DepositAddress       string          `json:"deposit_address" gorm:"type:varchar(128);not null"`
	AmountUnits          int64           `json:"amount_units" gorm:"not null;uniqueIndex:binance_scope_network_amount"`
	RequestedAmountUnits int64           `json:"-" gorm:"not null;default:0"`
	RequestedAmountUSDT  string          `json:"requested_amount_usdt" gorm:"type:numeric(30,6);not null;default:0"`
	FeeAmountUSDT        string          `json:"fee_amount_usdt" gorm:"type:numeric(30,6);not null;default:0"`
	PayableAmountUSDT    string          `json:"payable_amount_usdt" gorm:"type:numeric(30,6);not null;default:0"`
	AmountUSDT           string          `json:"amount_usdt" gorm:"type:numeric(30,6);not null"`
	AmountVND            int64           `json:"amount_vnd" gorm:"not null"`
	Credits              int64           `json:"credits" gorm:"not null"`
	QuotaSnapshot        int             `json:"-" gorm:"not null"`
	ExchangeRateSnapshot string          `json:"-" gorm:"type:numeric(30,12);not null"`
	FeePercentSnapshot   string          `json:"-" gorm:"type:numeric(12,6);not null"`
	BonusSnapshot        string          `json:"-" gorm:"type:numeric(12,6);not null"`
	Status               string          `json:"status" gorm:"type:varchar(24);not null;index"`
	ExpiresAt            time.Time       `json:"expires_at" gorm:"not null"`
	LateReconcileUntil   time.Time       `json:"-" gorm:"not null"`
	MatchedDepositID     *int64          `json:"matched_deposit_id,omitempty" gorm:"uniqueIndex"`
	VerificationReason   string          `json:"verification_reason,omitempty" gorm:"type:varchar(255)"`
	NextReconcileAt      *time.Time      `json:"-"`
	ReconcileAttempts    int             `json:"-" gorm:"not null;default:0"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
	PaidAt               *time.Time      `json:"paid_at,omitempty"`
	Deposit              *BinanceDeposit `json:"deposit,omitempty" gorm:"-"`
}

type BinanceDeposit struct {
	ID                int64     `json:"id" gorm:"primaryKey"`
	AccountScope      string    `json:"account_scope" gorm:"type:varchar(128);not null;default:'unverified';uniqueIndex:binance_account_deposit"`
	ProviderDepositID string    `json:"provider_deposit_id" gorm:"type:varchar(255);not null;uniqueIndex:binance_account_deposit"`
	Coin              string    `json:"coin" gorm:"type:varchar(16);not null"`
	Network           string    `json:"network" gorm:"type:varchar(16);not null;index"`
	Address           string    `json:"address" gorm:"type:varchar(128);not null"`
	AmountUnits       int64     `json:"amount_units" gorm:"not null"`
	AmountUSDT        string    `json:"amount_usdt" gorm:"type:numeric(30,6);not null"`
	TxID              string    `json:"tx_id" gorm:"type:varchar(255);index"`
	Status            int       `json:"status" gorm:"not null"`
	ConfirmTimes      string    `json:"confirm_times" gorm:"type:varchar(32)"`
	TravelRuleStatus  string    `json:"travel_rule_status" gorm:"type:varchar(32)"`
	InsertTime        time.Time `json:"insert_time" gorm:"index;not null"`
	DetectedAt        time.Time `json:"detected_at" gorm:"not null"`
	RawPayloadHash    string    `json:"raw_payload_hash" gorm:"type:char(64)"`
	RawPayload        string    `json:"-" gorm:"type:text"`
	MatchStatus       string    `json:"match_status" gorm:"type:varchar(24);not null;index"`
	MatchedOrderID    *int64    `json:"matched_order_id,omitempty"`
	ReviewReason      string    `json:"review_reason,omitempty" gorm:"type:varchar(255)"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type BinancePollCheckpoint struct {
	ID                 int64      `json:"id" gorm:"primaryKey"`
	AccountScope       string     `json:"-" gorm:"type:varchar(128);not null;default:'unverified';uniqueIndex:binance_scope_network"`
	Network            string     `json:"network" gorm:"type:varchar(16);not null;uniqueIndex:binance_scope_network"`
	LastInsertTime     time.Time  `json:"last_insert_time"`
	LastProviderCursor string     `json:"last_provider_cursor" gorm:"type:varchar(255)"`
	OverlapSeconds     int        `json:"overlap_seconds" gorm:"not null;default:1800"`
	LastSuccessAt      *time.Time `json:"last_success_at,omitempty"`
	LastError          string     `json:"last_error,omitempty" gorm:"type:varchar(255)"`
	LeaseOwner         string     `json:"-" gorm:"type:varchar(128)"`
	LeaseUntil         *time.Time `json:"-"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type BinanceSettlementAudit struct {
	ID                int64     `json:"id" gorm:"primaryKey"`
	AccountScope      string    `json:"account_scope" gorm:"type:varchar(128);not null;default:'unverified';uniqueIndex:binance_audit_deposit"`
	ProviderDepositID string    `json:"provider_deposit_id" gorm:"type:varchar(255);not null;uniqueIndex:binance_audit_deposit"`
	DepositID         int64     `json:"deposit_id" gorm:"index;not null"`
	BinanceOrderID    int64     `json:"binance_order_id" gorm:"index;not null"`
	TopUpID           int       `json:"top_up_id" gorm:"index;not null"`
	UserID            int       `json:"user_id" gorm:"index;not null"`
	Quota             int       `json:"quota" gorm:"not null"`
	SettledAt         time.Time `json:"settled_at" gorm:"not null"`
}

var (
	ErrBinanceOrderNotFound = errors.New("binance top-up order not found")
	ErrBinanceDepositUsed   = errors.New("binance deposit was already used")
	ErrBinanceMatchConflict = errors.New("binance deposit/order match is ambiguous or invalid")
)

type BinanceTopUpInput struct {
	UserID               int
	RequestID            string
	Network              string
	DepositAddress       string
	AmountUnits          int64
	RequestedAmountUnits int64
	AmountUSDT           string
	RequestedAmountUSDT  string
	FeeAmountUSDT        string
	AmountVND            int64
	Credits              int64
	QuotaSnapshot        int
	ExchangeRate         string
	FeePercent           string
	BonusPercent         string
	ExpiresAt            time.Time
	LateReconcileUntil   time.Time
	AccountScope         string
}

// CreateBinanceTopUp is the phase-1 order engine. It is not exposed by a
// route yet; phase 2 will call it only after feature flags and configuration
// validation. The request id and network/amount constraints make retries safe.
func CreateBinanceTopUp(input BinanceTopUpInput) (*BinanceTopUp, error) {
	if strings.TrimSpace(input.AccountScope) == "" {
		return nil, errors.New("binance account scope is required")
	}
	if input.RequestedAmountUnits == 0 {
		input.RequestedAmountUnits = input.AmountUnits
	}
	for attempt := 0; attempt < 5; attempt++ {
		result, err := createBinanceTopUpAttempt(input)
		if err == nil {
			return result, nil
		}
		if !isBinanceAmountCollision(err) {
			return nil, err
		}
		input.AmountUnits++
		input.AmountUSDT = decimal.NewFromInt(input.AmountUnits).Div(decimal.NewFromInt(1_000_000)).StringFixed(6)
		if err := recomputeBinanceSnapshot(&input); err != nil {
			return nil, err
		}
	}
	return nil, errors.New("binance amount allocation retry limit exceeded")
}

func recomputeBinanceSnapshot(input *BinanceTopUpInput) error {
	rate, err := decimal.NewFromString(input.ExchangeRate)
	if err != nil || !rate.IsPositive() {
		return errors.New("invalid quote exchange rate")
	}
	bonus, err := decimal.NewFromString(input.BonusPercent)
	if err != nil || bonus.IsNegative() {
		return errors.New("invalid quote bonus")
	}
	// Collision resolution changes only the payable unique amount. Credit and
	// bonus must remain based on the requested base amount, never on the fee or
	// the one-unit uniqueness adjustment.
	baseAmount := decimal.NewFromInt(input.RequestedAmountUnits).Div(decimal.NewFromInt(1_000_000))
	input.AmountVND = baseAmount.Mul(rate).RoundDown(0).IntPart()
	bonusVND := decimal.NewFromInt(input.AmountVND).Mul(bonus).Div(decimal.NewFromInt(100)).RoundDown(0).IntPart()
	input.Credits = (input.AmountVND + bonusVND) / 1000
	if input.Credits <= 0 || input.RequestedAmountUnits <= 0 {
		return errors.New("unique amount is below one credit")
	}
	return nil
}

func isBinanceAmountCollision(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate") || strings.Contains(message, "binance_network_amount")
}

func createBinanceTopUpAttempt(input BinanceTopUpInput) (*BinanceTopUp, error) {
	if input.UserID <= 0 || input.RequestID == "" || input.Network == "" || input.DepositAddress == "" || input.AmountUnits <= 0 || input.AmountUSDT == "" || input.Credits <= 0 || input.QuotaSnapshot <= 0 {
		return nil, errors.New("invalid binance top-up input")
	}
	quotedAmount, err := decimal.NewFromString(input.AmountUSDT)
	if err != nil || !quotedAmount.IsPositive() || quotedAmount.Mul(decimal.NewFromInt(1_000_000)).IntPart() != input.AmountUnits {
		return nil, errors.New("invalid binance amount snapshot")
	}
	var result BinanceTopUp
	err = DB.Transaction(func(tx *gorm.DB) error {
		var existing BinanceTopUp
		if err := tx.Where("user_id = ? AND request_id = ?", input.UserID, input.RequestID).First(&existing).Error; err == nil {
			existingRequested := existing.RequestedAmountUnits
			if existingRequested == 0 {
				existingRequested = existing.AmountUnits
			}
			if existing.AccountScope != input.AccountScope || existing.Network != input.Network || existingRequested != input.RequestedAmountUnits {
				return ErrBinanceMatchConflict
			}
			result = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var user User
		if err := lockForUpdate(tx).First(&user, input.UserID).Error; err != nil {
			return err
		}
		maxCurrent, err := topUpQuotaMaxCurrent(input.QuotaSnapshot)
		if err != nil {
			return err
		}
		if user.Quota > maxCurrent {
			return ErrTopUpQuotaLimitExceeded
		}
		tradeNo := "BN" + common.GetRandomString(28)
		allocatedUnits := input.AmountUnits
		for attempts := 0; attempts < 10000; attempts++ {
			var collision BinanceTopUp
			err := lockForUpdate(tx).Where("account_scope = ? AND network = ? AND amount_units = ?", input.AccountScope, input.Network, allocatedUnits).First(&collision).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				break
			}
			if err != nil {
				return err
			}
			allocatedUnits++
			if attempts == 9999 {
				return errors.New("unable to allocate unique binance amount")
			}
		}
		allocatedUSDT := decimal.NewFromInt(allocatedUnits).Div(decimal.NewFromInt(1_000_000)).StringFixed(6)
		if allocatedUnits != input.AmountUnits {
			input.AmountUnits = allocatedUnits
			input.AmountUSDT = allocatedUSDT
		}
		topup := TopUp{UserId: input.UserID, Amount: input.Credits, Money: float64(input.AmountVND), TradeNo: tradeNo, PaymentMethod: "binance_usdt_" + input.Network, PaymentProvider: PaymentProviderBinance, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
		if err := tx.Create(&topup).Error; err != nil {
			return err
		}
		requestedUSDT := input.RequestedAmountUSDT
		if requestedUSDT == "" {
			requestedUSDT = decimal.NewFromInt(input.RequestedAmountUnits).Div(decimal.NewFromInt(1_000_000)).StringFixed(6)
		}
		feeUSDT := input.FeeAmountUSDT
		if feeUSDT == "" {
			feeUSDT = "0"
		}
		result = BinanceTopUp{TopUpID: topup.Id, UserID: input.UserID, RequestID: input.RequestID, AccountScope: input.AccountScope, TradeNo: tradeNo, Network: input.Network, Coin: BinanceCoinUSDT, DepositAddress: input.DepositAddress, AmountUnits: allocatedUnits, RequestedAmountUnits: input.RequestedAmountUnits, RequestedAmountUSDT: requestedUSDT, FeeAmountUSDT: feeUSDT, PayableAmountUSDT: allocatedUSDT, AmountUSDT: allocatedUSDT, AmountVND: input.AmountVND, Credits: input.Credits, QuotaSnapshot: input.QuotaSnapshot, ExchangeRateSnapshot: input.ExchangeRate, FeePercentSnapshot: input.FeePercent, BonusSnapshot: input.BonusPercent, Status: BinanceOrderPending, ExpiresAt: input.ExpiresAt, LateReconcileUntil: input.LateReconcileUntil, NextReconcileAt: &input.ExpiresAt}
		return tx.Create(&result).Error
	})
	return &result, err
}

// SettleBinanceDeposit is deliberately not called by any route or worker in
// phase 1. It is the guarded transaction used by phase 2 after matching.
func SettleBinanceDeposit(accountScope, providerDepositID, tradeNo string) (alreadySettled bool, err error) {
	if accountScope == "" || providerDepositID == "" || tradeNo == "" {
		return false, ErrBinanceMatchConflict
	}
	var userID, quota int
	err = DB.Transaction(func(tx *gorm.DB) error {
		var deposit BinanceDeposit
		if err := lockForUpdate(tx).Where("account_scope = ? AND provider_deposit_id = ?", accountScope, providerDepositID).First(&deposit).Error; err != nil {
			return ErrBinanceOrderNotFound
		}
		if deposit.MatchStatus == BinanceDepositSettled {
			alreadySettled = true
			return nil
		}
		if deposit.Status != BinanceDepositSuccess || deposit.MatchStatus != BinanceDepositMatched {
			return ErrBinanceMatchConflict
		}

		var order BinanceTopUp
		if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
			return ErrBinanceOrderNotFound
		}
		if order.Status != BinanceOrderPending && order.Status != BinanceOrderReview {
			return ErrBinanceMatchConflict
		}
		if order.MatchedDepositID != nil && *order.MatchedDepositID != deposit.ID {
			return ErrBinanceMatchConflict
		}
		if order.AccountScope != accountScope || deposit.AccountScope != accountScope {
			return ErrBinanceMatchConflict
		}
		if deposit.MatchedOrderID != nil && *deposit.MatchedOrderID != order.ID {
			return ErrBinanceMatchConflict
		}
		if deposit.Coin != order.Coin || deposit.Network != order.Network || deposit.Address != order.DepositAddress || deposit.AmountUnits != order.AmountUnits || deposit.InsertTime.Before(order.CreatedAt) || deposit.InsertTime.After(order.LateReconcileUntil) {
			return ErrBinanceMatchConflict
		}
		var topup TopUp
		if err := lockForUpdate(tx).Where("id = ?", order.TopUpID).First(&topup).Error; err != nil {
			return ErrBinanceOrderNotFound
		}
		if topup.PaymentProvider != PaymentProviderBinance || topup.UserId != order.UserID || topup.Status != common.TopUpStatusPending {
			return ErrBinanceMatchConflict
		}
		if err := creditTopUpQuota(tx, order.UserID, order.QuotaSnapshot, nil); err != nil {
			return err
		}
		now := time.Now()
		if err := tx.Model(&order).Updates(map[string]any{"status": BinanceOrderPaid, "matched_deposit_id": deposit.ID, "paid_at": now, "verification_reason": ""}).Error; err != nil {
			return err
		}
		if err := tx.Model(&deposit).Updates(map[string]any{"match_status": BinanceDepositSettled, "matched_order_id": order.ID, "review_reason": ""}).Error; err != nil {
			return err
		}
		updated := tx.Model(&TopUp{}).Where("id = ? AND payment_provider = ? AND status = ?", order.TopUpID, PaymentProviderBinance, common.TopUpStatusPending).Updates(map[string]any{"status": common.TopUpStatusSuccess, "complete_time": now.Unix()})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrBinanceMatchConflict
		}
		if err := tx.Create(&BinanceSettlementAudit{AccountScope: accountScope, ProviderDepositID: providerDepositID, DepositID: deposit.ID, BinanceOrderID: order.ID, TopUpID: order.TopUpID, UserID: order.UserID, Quota: order.QuotaSnapshot, SettledAt: now}).Error; err != nil {
			return err
		}
		userID, quota = order.UserID, order.QuotaSnapshot
		return nil
	})
	if err == nil && !alreadySettled {
		syncCreditUserQuotaCache(userID, quota, "binance topup")
		RecordTopupLog(userID, fmt.Sprintf("Binance USDT: %d credits / %s", quota, tradeNo), "", "binance_usdt", PaymentProviderBinance)
	}
	return alreadySettled, err
}

func GetBinanceTopUp(tradeNo string) (*BinanceTopUp, error) {
	var order BinanceTopUp
	err := DB.Where("trade_no = ?", tradeNo).First(&order).Error
	if err != nil {
		return &order, err
	}
	if err := ExpireBinanceTopUpIfNeeded(tradeNo); err != nil {
		return nil, err
	}
	// Return the durable state after the expiry check so the UI never keeps
	// showing an order as payable after its deadline.
	err = DB.Where("trade_no = ?", tradeNo).First(&order).Error
	return &order, err
}

// ExpireBinanceTopUpIfNeeded closes an unpaid order after its checkout
// deadline. Deposits arriving after the deadline remain unmatched and go
// through manual review; this function never settles or credits them.
func ExpireBinanceTopUpIfNeeded(tradeNo string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var order BinanceTopUp
		if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
			return err
		}
		if order.Status != BinanceOrderPending || order.ExpiresAt.IsZero() || time.Now().Before(order.ExpiresAt) {
			return nil
		}
		if err := tx.Model(&order).Updates(map[string]any{
			"status":              BinanceOrderExpired,
			"verification_reason": "order expired before payment",
			"updated_at":          time.Now(),
		}).Error; err != nil {
			return err
		}
		return tx.Model(&TopUp{}).
			Where("id = ? AND payment_provider = ? AND status = ?", order.TopUpID, PaymentProviderBinance, common.TopUpStatusPending).
			Updates(map[string]any{"status": common.TopUpStatusExpired, "complete_time": time.Now().Unix()}).Error
	})
}

// ExpireStaleBinanceTopUps is called by the admin listing and polling worker
// so stale orders are closed even when nobody opens the individual order.
func ExpireStaleBinanceTopUps() error {
	var orders []BinanceTopUp
	if err := DB.Where("status = ? AND expires_at <= ?", BinanceOrderPending, time.Now()).Find(&orders).Error; err != nil {
		return err
	}
	for _, order := range orders {
		if err := ExpireBinanceTopUpIfNeeded(order.TradeNo); err != nil {
			return err
		}
	}
	return nil
}

func CancelBinanceTopUp(tradeNo string, userID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var order BinanceTopUp
		if err := lockForUpdate(tx).Where("trade_no = ? AND user_id = ?", tradeNo, userID).First(&order).Error; err != nil {
			return ErrBinanceOrderNotFound
		}
		if order.Status == BinanceOrderPaid {
			return ErrBinanceMatchConflict
		}
		if order.Status == BinanceOrderVoided || order.Status == BinanceOrderExpired {
			return nil
		}
		if order.Status != BinanceOrderPending && order.Status != BinanceOrderReview {
			return ErrBinanceMatchConflict
		}
		now := time.Now()
		if err := tx.Model(&order).Updates(map[string]any{"status": BinanceOrderVoided, "verification_reason": "cancelled by user", "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&TopUp{}).Where("id = ? AND payment_provider = ? AND status = ?", order.TopUpID, PaymentProviderBinance, common.TopUpStatusPending).Updates(map[string]any{"status": common.TopUpStatusExpired}).Error
	})
}

func MarkBinanceReview(tradeNo, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("review reason is required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var order BinanceTopUp
		if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
			return ErrBinanceOrderNotFound
		}
		if order.Status == BinanceOrderPaid {
			return ErrBinanceMatchConflict
		}
		return tx.Model(&order).Updates(map[string]any{"status": BinanceOrderReview, "verification_reason": reason}).Error
	})
}

func EnrichBinanceTopUps(topups []*TopUp) error {
	ids := make([]int, 0)
	for _, topup := range topups {
		if topup != nil && topup.PaymentProvider == PaymentProviderBinance {
			ids = append(ids, topup.Id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var orders []BinanceTopUp
	if err := DB.Where("top_up_id IN ?", ids).Find(&orders).Error; err != nil {
		return err
	}
	byID := make(map[int]*BinanceTopUp, len(orders))
	for i := range orders {
		byID[orders[i].TopUpID] = &orders[i]
	}
	for _, topup := range topups {
		if topup != nil {
			topup.Binance = byID[topup.Id]
		}
	}
	depositIDs := make([]int64, 0, len(orders))
	for i := range orders {
		if orders[i].MatchedDepositID != nil {
			depositIDs = append(depositIDs, *orders[i].MatchedDepositID)
		}
	}
	if len(depositIDs) > 0 {
		var deposits []BinanceDeposit
		if err := DB.Where("id IN ?", depositIDs).Find(&deposits).Error; err != nil {
			return err
		}
		byDeposit := make(map[int64]*BinanceDeposit, len(deposits))
		for i := range deposits {
			byDeposit[deposits[i].ID] = &deposits[i]
		}
		for i := range orders {
			if orders[i].MatchedDepositID != nil {
				orders[i].Deposit = byDeposit[*orders[i].MatchedDepositID]
			}
		}
	}
	return nil
}

func LogBinanceWorkerError(err error) {
	if err != nil {
		common.SysError("Binance worker: " + err.Error())
	}
}
