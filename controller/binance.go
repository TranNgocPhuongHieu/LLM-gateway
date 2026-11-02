package controller

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/TranNgocPhuongHieu/LLM-gateway/service"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type binanceOrderRequest struct {
	Network    string `json:"network" binding:"required"`
	AmountUSDT string `json:"amount_usdt" binding:"required"`
	RequestID  string `json:"request_id" binding:"required"`
}

type binanceQuoteRequest struct {
	AmountUSDT string `json:"amount_usdt" binding:"required"`
}

func BinanceQuote(c *gin.Context) {
	var req binanceQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "invalid Binance quote request")
		return
	}
	cfg, err := model.GetBinanceConfig()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !cfg.Enabled || !cfg.AddressesVerified || (!cfg.CheckoutEnabled && !service.IsBinanceCanaryUser(c.GetInt("id"))) {
		common.ApiErrorMsg(c, "Binance top-up is unavailable")
		return
	}
	base, err := decimal.NewFromString(req.AmountUSDT)
	if err != nil {
		common.ApiErrorMsg(c, "invalid USDT amount")
		return
	}
	bonus, err := cfg.BonusFor(base)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	q, err := service.BuildBinanceQuote(service.QuoteInput{BaseUSDT: req.AmountUSDT, FeePercent: cfg.FeePercent, USDTToVND: cfg.USDTToVND, QuotaPerUnit: decimal.NewFromFloat(common.QuotaPerUnit).String(), BonusPercent: bonus.String(), MinUSDT: cfg.MinUSDT, MaxUSDT: cfg.MaxUSDT})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"base_usdt": q.BaseUSDT, "fee_percent": q.FeePercent, "fee_usdt": q.FeeUSDT, "payable_usdt": q.PayableUSDT, "usdt_to_vnd": q.USDTToVND, "bonus_percent": q.BonusPercent, "amount_vnd": q.AmountVND, "credits": q.Credits, "min_usdt": cfg.MinUSDT, "max_usdt": cfg.MaxUSDT})
}

func BinancePublicConfig(c *gin.Context) {
	cfg, err := model.GetBinanceConfig()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"enabled": cfg.Enabled && cfg.CheckoutEnabled && cfg.AddressesVerified, "min_usdt": cfg.MinUSDT, "max_usdt": cfg.MaxUSDT, "fee_percent": cfg.FeePercent, "networks": cfg.Networks})
}

func CreateBinanceOrder(c *gin.Context) {
	var req binanceOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.RequestID) == "" || len(req.RequestID) > 64 {
		common.ApiErrorMsg(c, "invalid Binance order request")
		return
	}
	cfg, err := model.GetBinanceConfig()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !cfg.Enabled || !cfg.AddressesVerified || !cfg.Networks[req.Network] || (!cfg.CheckoutEnabled && !service.IsBinanceCanaryUser(c.GetInt("id"))) {
		common.ApiErrorMsg(c, "Binance top-up is unavailable")
		return
	}
	address := cfg.Addresses[req.Network]
	if address == "" {
		common.ApiErrorMsg(c, "Binance network address is not configured")
		return
	}
	if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeCustom {
		common.ApiErrorMsg(c, "Binance top-up requires custom credit currency")
		return
	}
	base, err := decimal.NewFromString(req.AmountUSDT)
	if err != nil {
		common.ApiErrorMsg(c, "invalid USDT amount")
		return
	}
	bonus, err := cfg.BonusFor(base)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	q, err := service.BuildBinanceQuote(service.QuoteInput{BaseUSDT: req.AmountUSDT, FeePercent: cfg.FeePercent, USDTToVND: cfg.USDTToVND, QuotaPerUnit: decimal.NewFromFloat(common.QuotaPerUnit).String(), BonusPercent: bonus.String(), MinUSDT: cfg.MinUSDT, MaxUSDT: cfg.MaxUSDT})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	now := time.Now()
	order, err := model.CreateBinanceTopUp(model.BinanceTopUpInput{UserID: c.GetInt("id"), RequestID: req.RequestID, AccountScope: cfg.AccountScope, Network: req.Network, DepositAddress: address, AmountUnits: q.AmountUnits, AmountUSDT: q.PayableUSDT, RequestedAmountUSDT: q.BaseUSDT, FeeAmountUSDT: q.FeeUSDT, AmountVND: q.AmountVND, Credits: q.Credits, QuotaSnapshot: int(q.Quota), ExchangeRate: q.USDTToVND, FeePercent: q.FeePercent, BonusPercent: q.BonusPercent, ExpiresAt: now.Add(15 * time.Minute), LateReconcileUntil: now.Add(time.Duration(cfg.LateReconcileSeconds) * time.Second)})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"trade_no": order.TradeNo, "network": order.Network, "coin": order.Coin, "address": order.DepositAddress, "amount_usdt": order.AmountUSDT, "amount_vnd": order.AmountVND, "credits": order.Credits, "status": order.Status, "expires_at": order.ExpiresAt, "qr_payload": order.DepositAddress + "?coin=USDT&network=" + order.Network + "&amount=" + order.AmountUSDT})
}

func GetBinanceOrder(c *gin.Context) {
	order, err := model.GetBinanceTopUp(c.Param("trade_no"))
	if err != nil || order.UserID != c.GetInt("id") {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	common.ApiSuccess(c, gin.H{"trade_no": order.TradeNo, "network": order.Network, "coin": order.Coin, "address": order.DepositAddress, "amount_usdt": order.AmountUSDT, "amount_vnd": order.AmountVND, "credits": order.Credits, "status": order.Status, "reason": order.VerificationReason, "expires_at": order.ExpiresAt, "paid_at": order.PaidAt})
}

func CancelBinanceOrder(c *gin.Context) {
	if err := model.CancelBinanceTopUp(c.Param("trade_no"), c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"status": model.BinanceOrderVoided})
}

func GetBinanceAdminConfig(c *gin.Context) {
	cfg, err := model.GetBinanceConfig()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, cfg)
}
func UpdateBinanceAdminConfig(c *gin.Context) {
	var cfg model.BinanceConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		common.ApiErrorMsg(c, "invalid Binance configuration")
		return
	}
	if err := model.SaveBinanceConfig(cfg); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordOperationAuditLog(c.GetInt("id"), c.GetInt("role"), "Updated Binance USDT payment configuration", c.ClientIP(), "binance.config.update", map[string]any{"enabled": cfg.Enabled, "checkout_enabled": cfg.CheckoutEnabled, "real_polling_enabled": cfg.RealPollingEnabled, "auto_settlement_enabled": cfg.AutoSettlementEnabled}, nil, nil, c)
	common.ApiSuccess(c, gin.H{"saved": true})
}

func BinanceDiagnostic(c *gin.Context) {
	if !service.DefaultBinanceFeatureFlags.MockMode && !service.DefaultBinanceFeatureFlags.RealPollingEnabled {
		common.ApiErrorMsg(c, "real Binance diagnostic is disabled")
		return
	}
	cfg, err := service.BinanceHTTPClientConfigFromEnv()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	client, err := service.NewBinanceHTTPClient(cfg)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	restrictions, err := client.AccountRestrictions(ctx)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := service.ValidateReadOnlyRestrictions(restrictions); err != nil {
		common.ApiError(c, err)
		return
	}
	networks, err := client.CapitalConfig(ctx)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	appCfg, err := model.GetBinanceConfig()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	checks := make([]gin.H, 0)
	for _, uiNetwork := range []string{model.BinanceNetworkBEP20, model.BinanceNetworkTRC20} {
		if !appCfg.Networks[uiNetwork] {
			continue
		}
		providerNetwork, mapErr := service.BinanceNetworkCode(uiNetwork)
		if mapErr != nil {
			common.ApiError(c, mapErr)
			return
		}
		address, addrErr := client.DepositAddress(ctx, service.BinanceCoinUSDT, providerNetwork)
		if addrErr != nil {
			common.ApiError(c, addrErr)
			return
		}
		var networkConfig service.BinanceNetworkConfig
		for _, candidate := range networks {
			if candidate.Coin == service.BinanceCoinUSDT && candidate.Network == providerNetwork {
				networkConfig = candidate
				break
			}
		}
		if err := service.ValidateBinanceNetworkAddress(networkConfig, address, providerNetwork, appCfg.Addresses[uiNetwork]); err != nil {
			common.ApiError(c, err)
			return
		}
		checks = append(checks, gin.H{"network": uiNetwork, "provider_network": providerNetwork, "address_verified": true})
	}
	if err := model.MarkBinanceAddressesVerified(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"read_only": true, "ip_restricted": restrictions.IPRestrict, "networks": networks, "address_checks": checks})
}

func BinanceAdminReconcile(c *gin.Context) {
	tradeNo := c.Param("trade_no")
	order, err := model.GetBinanceTopUp(tradeNo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if order.Status == model.BinanceOrderPaid {
		common.ApiSuccess(c, gin.H{"status": order.Status, "already_settled": true})
		return
	}
	cfg, err := model.GetBinanceConfig()
	if err != nil || !cfg.RealPollingEnabled {
		common.ApiErrorMsg(c, "real Binance polling is disabled")
		return
	}
	clientCfg, err := service.BinanceHTTPClientConfigFromEnv()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	client, err := service.NewBinanceHTTPClient(clientCfg)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	poller := &service.BinancePoller{Client: client, AccountScope: cfg.AccountScope, Owner: "admin-reconcile"}
	if err := poller.PollOnce(c.Request.Context()); err != nil {
		common.ApiError(c, err)
		return
	}
	updated, _ := model.GetBinanceTopUp(tradeNo)
	model.RecordOperationAuditLog(c.GetInt("id"), c.GetInt("role"), "Reconciled Binance USDT order", c.ClientIP(), "binance.order.reconcile", map[string]any{"trade_no": tradeNo, "status": updated.Status}, nil, nil, c)
	common.ApiSuccess(c, gin.H{"status": updated.Status, "trade_no": tradeNo})
}

func BinanceAdminReview(c *gin.Context) {
	var request struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Reason) == "" {
		common.ApiErrorMsg(c, "review reason is required")
		return
	}
	if err := model.MarkBinanceReview(c.Param("trade_no"), request.Reason); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordOperationAuditLog(c.GetInt("id"), c.GetInt("role"), "Marked Binance USDT order for manual review", c.ClientIP(), "binance.order.review", map[string]any{"trade_no": c.Param("trade_no")}, nil, nil, c)
	common.ApiSuccess(c, gin.H{"status": model.BinanceOrderReview})
}
