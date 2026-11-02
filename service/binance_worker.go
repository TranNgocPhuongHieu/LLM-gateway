package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BinancePoller struct {
	Client       BinanceWalletClient
	AccountScope string
	Owner        string
	Now          func() time.Time
}

func StartBinancePollingWorker(ctx context.Context) {
	if strings.ToLower(strings.TrimSpace(os.Getenv("BINANCE_REAL_POLLING_ENABLED"))) != "true" {
		return
	}
	cfg, err := model.GetBinanceConfig()
	if err != nil || !cfg.Enabled || !cfg.RealPollingEnabled {
		return
	}
	clientCfg, err := BinanceHTTPClientConfigFromEnv()
	if err != nil {
		return
	}
	client, err := NewBinanceHTTPClient(clientCfg)
	if err != nil {
		return
	}
	poller := &BinancePoller{Client: client, AccountScope: cfg.AccountScope, Owner: fmt.Sprintf("%s:%d", os.Getenv("HOSTNAME"), os.Getpid())}
	interval := time.Duration(maxInt(cfg.PollIntervalSeconds, 10)) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := poller.PollOnce(ctx); err != nil {
				model.LogBinanceWorkerError(err)
			}
		}
	}
}

func (p *BinancePoller) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *BinancePoller) PollOnce(ctx context.Context) error {
	if p.Client == nil || strings.TrimSpace(p.AccountScope) == "" || strings.TrimSpace(p.Owner) == "" {
		return errors.New("Binance poller is not configured")
	}
	cfg, err := model.GetBinanceConfig()
	if err != nil {
		return err
	}
	if !cfg.Enabled || !cfg.RealPollingEnabled || !cfg.AddressesVerified {
		return errors.New("Binance real polling is disabled")
	}
	if err := model.ExpireStaleBinanceTopUps(); err != nil {
		return err
	}
	if restrictions, err := p.Client.AccountRestrictions(ctx); err != nil {
		return err
	} else if err := ValidateReadOnlyRestrictions(restrictions); err != nil {
		return err
	}
	if reader, ok := p.Client.(BinanceNetworkReader); ok {
		networks, err := reader.CapitalConfig(ctx)
		if err != nil {
			return err
		}
		for _, uiNetwork := range []string{BinanceNetworkBEP20, BinanceNetworkTRC20} {
			if !cfg.Networks[uiNetwork] {
				continue
			}
			providerNetwork, err := BinanceNetworkCode(uiNetwork)
			if err != nil {
				return err
			}
			var networkConfig *BinanceNetworkConfig
			for i := range networks {
				if networks[i].Network == providerNetwork && networks[i].Coin == BinanceCoinUSDT {
					networkConfig = &networks[i]
					break
				}
			}
			if networkConfig == nil {
				return fmt.Errorf("Binance %s network is missing", providerNetwork)
			}
			address, err := reader.DepositAddress(ctx, BinanceCoinUSDT, providerNetwork)
			if err != nil {
				return err
			}
			if err := ValidateBinanceNetworkAddress(*networkConfig, address, providerNetwork, cfg.Addresses[uiNetwork]); err != nil {
				return err
			}
		}
	}
	for _, uiNetwork := range []string{BinanceNetworkBEP20, BinanceNetworkTRC20} {
		if !cfg.Networks[uiNetwork] {
			continue
		}
		if err := p.pollNetwork(ctx, cfg, uiNetwork); err != nil {
			return err
		}
	}
	return nil
}

func (p *BinancePoller) pollNetwork(ctx context.Context, cfg model.BinanceConfig, uiNetwork string) error {
	providerNetwork, err := BinanceNetworkCode(uiNetwork)
	if err != nil {
		return err
	}
	now := p.now()
	cp, claimed, err := claimBinanceCheckpoint(p.AccountScope, uiNetwork, p.Owner, now, time.Duration(maxInt(cfg.PollIntervalSeconds*2, 30))*time.Second)
	if err != nil || !claimed {
		return err
	}
	var start time.Time
	if cp.LastInsertTime.IsZero() {
		start = now.Add(-24 * time.Hour)
	} else {
		start = cp.LastInsertTime.Add(-time.Duration(maxInt(cp.OverlapSeconds, 1800)) * time.Second)
	}
	rows, err := p.Client.DepositHistory(ctx, providerNetwork, start, now)
	if err != nil {
		return failCheckpoint(p.AccountScope, uiNetwork, p.Owner, err)
	}
	for _, row := range rows {
		if err := p.ingestAndMatch(ctx, cfg, uiNetwork, providerNetwork, row); err != nil {
			return failCheckpoint(p.AccountScope, uiNetwork, p.Owner, err)
		}
		if row.InsertTime.After(cp.LastInsertTime) {
			cp.LastInsertTime = row.InsertTime
		}
	}
	return finishBinanceCheckpoint(p.AccountScope, uiNetwork, p.Owner, cp.LastInsertTime, now)
}

func (p *BinancePoller) ingestAndMatch(ctx context.Context, cfg model.BinanceConfig, uiNetwork, providerNetwork string, row BinanceDepositRecord) error {
	if row.ProviderDepositID == "" || row.Coin != BinanceCoinUSDT || row.Network != providerNetwork || row.Address == "" || row.AmountUnits <= 0 || row.InsertTime.IsZero() {
		return errors.New("invalid Binance deposit record")
	}
	address := cfg.Addresses[uiNetwork]
	now := p.now()
	matchStatus := model.BinanceDepositUnmatched
	reason := ""
	if address == "" || row.Address != address {
		matchStatus, reason = model.BinanceDepositReview, "deposit address does not match configured address"
	}
	deposit := model.BinanceDeposit{AccountScope: p.AccountScope, ProviderDepositID: row.ProviderDepositID, Coin: model.BinanceCoinUSDT, Network: uiNetwork, Address: row.Address, AmountUnits: row.AmountUnits, AmountUSDT: formatUnits(row.AmountUnits), TxID: row.TxID, Status: row.Status, ConfirmTimes: row.ConfirmTimes, TravelRuleStatus: row.TravelRuleStatus, InsertTime: row.InsertTime, DetectedAt: now, MatchStatus: matchStatus, ReviewReason: reason}
	// Deposit history is polled with an overlap window. Once a deposit has
	// settled, a replay must refresh provider metadata without downgrading its
	// durable settlement state or losing the matched order.
	var existing model.BinanceDeposit
	if err := model.DB.Where("account_scope = ? AND provider_deposit_id = ?", p.AccountScope, row.ProviderDepositID).First(&existing).Error; err == nil && existing.MatchStatus == model.BinanceDepositSettled {
		return model.DB.Model(&existing).Updates(map[string]any{"status": row.Status, "confirm_times": row.ConfirmTimes, "travel_rule_status": row.TravelRuleStatus, "tx_id": row.TxID, "updated_at": now}).Error
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err := model.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "account_scope"}, {Name: "provider_deposit_id"}}, DoUpdates: clause.Assignments(map[string]any{"status": row.Status, "confirm_times": row.ConfirmTimes, "travel_rule_status": row.TravelRuleStatus, "tx_id": row.TxID, "match_status": matchStatus, "review_reason": reason, "updated_at": now})}).Create(&deposit).Error; err != nil {
		return err
	}
	if row.Status != model.BinanceDepositSuccess || cfg.ConfirmationMinimum > 0 && parseConfirmations(row.ConfirmTimes) < cfg.ConfirmationMinimum {
		return nil
	}
	if matchStatus == model.BinanceDepositReview {
		return nil
	}
	if row.TravelRuleStatus != "" && strings.ToUpper(row.TravelRuleStatus) != "COMPLETE" {
		_ = model.DB.Model(&model.BinanceDeposit{}).Where("account_scope = ? AND provider_deposit_id = ?", p.AccountScope, row.ProviderDepositID).Updates(map[string]any{"match_status": model.BinanceDepositReview, "review_reason": "travel rule status is not complete"})
		return nil
	}
	var matches []model.BinanceTopUp
	if err := model.DB.Where("account_scope = ? AND network = ? AND deposit_address = ? AND amount_units = ? AND status IN ? AND created_at <= ? AND late_reconcile_until >= ?", p.AccountScope, uiNetwork, address, row.AmountUnits, []string{model.BinanceOrderPending, model.BinanceOrderReview}, row.InsertTime, row.InsertTime).Find(&matches).Error; err != nil {
		return err
	}
	if len(matches) != 1 {
		if len(matches) == 0 {
			var candidates []model.BinanceTopUp
			if err := model.DB.Where("account_scope = ? AND network = ? AND deposit_address = ? AND status IN ? AND created_at <= ? AND late_reconcile_until >= ?", p.AccountScope, uiNetwork, address, []string{model.BinanceOrderPending, model.BinanceOrderReview}, row.InsertTime, row.InsertTime).Find(&candidates).Error; err != nil {
				return err
			}
			if len(candidates) == 1 && candidates[0].AmountUnits != row.AmountUnits {
				reason := "deposit amount differs from order"
				_ = model.DB.Model(&deposit).Updates(map[string]any{"match_status": model.BinanceDepositReview, "review_reason": reason})
				if row.AmountUnits < candidates[0].AmountUnits {
					_ = model.DB.Model(&candidates[0]).Updates(map[string]any{"status": model.BinanceOrderPartial, "verification_reason": reason})
				}
				return nil
			}
		}
		if len(matches) > 1 {
			_ = model.DB.Model(&deposit).Updates(map[string]any{"match_status": model.BinanceDepositReview, "review_reason": "ambiguous order match"})
		}
		return nil
	}
	order := matches[0]
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var locked model.BinanceDeposit
		query := tx
		if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
			query = tx.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.Where("account_scope = ? AND provider_deposit_id = ?", p.AccountScope, row.ProviderDepositID).First(&locked).Error; err != nil {
			return err
		}
		if locked.MatchStatus == model.BinanceDepositSettled {
			return nil
		}
		if locked.MatchedOrderID != nil && *locked.MatchedOrderID != order.ID {
			return errors.New("deposit already matched to another order")
		}
		return tx.Model(&locked).Updates(map[string]any{"match_status": model.BinanceDepositMatched, "matched_order_id": order.ID}).Error
	}); err != nil {
		return err
	}
	if cfg.AutoSettlementEnabled {
		_, err := model.SettleBinanceDeposit(p.AccountScope, row.ProviderDepositID, order.TradeNo)
		return err
	}
	return nil
}

func formatUnits(units int64) string    { return fmt.Sprintf("%d.%06d", units/1_000_000, units%1_000_000) }
func parseConfirmations(raw string) int { var n int; _, _ = fmt.Sscanf(raw, "%d", &n); return n }
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func claimBinanceCheckpoint(scope, network, owner string, now time.Time, lease time.Duration) (model.BinancePollCheckpoint, bool, error) {
	var cp model.BinancePollCheckpoint
	claimed := false
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.BinancePollCheckpoint{AccountScope: scope, Network: network, OverlapSeconds: 1800}).Error; err != nil {
			return err
		}
		query := tx
		if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
			query = tx.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.Where("account_scope = ? AND network = ?", scope, network).First(&cp).Error; err != nil {
			return err
		}
		if cp.LeaseUntil != nil && cp.LeaseUntil.After(now) && cp.LeaseOwner != owner {
			return nil
		}
		until := now.Add(lease)
		result := tx.Model(&cp).Updates(map[string]any{"lease_owner": owner, "lease_until": until, "last_error": ""})
		if result.Error != nil {
			return result.Error
		}
		claimed = true
		return nil
	})
	return cp, claimed, err
}

func finishBinanceCheckpoint(scope, network, owner string, last time.Time, now time.Time) error {
	return model.DB.Model(&model.BinancePollCheckpoint{}).Where("account_scope = ? AND network = ? AND lease_owner = ?", scope, network, owner).Updates(map[string]any{"last_insert_time": last, "last_success_at": now, "lease_until": nil, "last_error": ""}).Error
}
func failCheckpoint(scope, network, owner string, err error) error {
	_ = model.DB.Model(&model.BinancePollCheckpoint{}).Where("account_scope = ? AND network = ? AND lease_owner = ?", scope, network, owner).Updates(map[string]any{"last_error": err.Error(), "lease_until": nil})
	return err
}
