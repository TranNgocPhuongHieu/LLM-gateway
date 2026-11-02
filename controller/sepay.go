package controller

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type sepayCheckoutRequest struct {
	Flow      string `json:"flow"`
	AmountVND int64  `json:"amount_vnd" binding:"required"`
	RequestID string `json:"request_id" binding:"required"`
}

type sepayOrder struct {
	ID                 string `json:"id"`
	OrderID            string `json:"order_id"`
	OrderStatus        string `json:"order_status"`
	OrderCurrency      string `json:"order_currency"`
	OrderAmount        string `json:"order_amount"`
	OrderInvoiceNumber string `json:"order_invoice_number"`
}
type sepayTransaction struct {
	ID                  string `json:"id"`
	TransactionID       string `json:"transaction_id"`
	TransactionType     string `json:"transaction_type"`
	TransactionStatus   string `json:"transaction_status"`
	TransactionAmount   string `json:"transaction_amount"`
	TransactionCurrency string `json:"transaction_currency"`
}
type sepayIPN struct {
	Timestamp        int64            `json:"timestamp"`
	NotificationType string           `json:"notification_type"`
	Order            sepayOrder       `json:"order"`
	Transaction      sepayTransaction `json:"transaction"`
}

type sepayBankWebhook struct {
	ID             int64  `json:"id"`
	Code           string `json:"code"`
	Content        string `json:"content"`
	TransferType   string `json:"transferType"`
	TransferAmount int64  `json:"transferAmount"`
	Gateway        string `json:"gateway"`
	AccountNumber  string `json:"accountNumber"`
	ReferenceCode  string `json:"referenceCode"`
}

func sepayCheckoutURL(environment string) string {
	if environment == "sandbox" {
		return "https://pay-sandbox.sepay.vn/v1/checkout/init"
	}
	return "https://pay.sepay.vn/v1/checkout/init"
}

func sepayCheckoutSignature(fields map[string]string, secret string) string {
	ordered := []string{"order_amount", "merchant", "currency", "operation", "order_description", "order_invoice_number", "customer_id", "payment_method", "success_url", "error_url", "cancel_url"}
	parts := make([]string, 0, len(ordered))
	for _, key := range ordered {
		if value, ok := fields[key]; ok {
			parts = append(parts, key+"="+value)
		}
	}
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(strings.Join(parts, ",")))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func GetSepayPublicConfig(c *gin.Context) {
	cfg, err := model.GetSepayConfig()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	configured := cfg.MerchantID != "" && cfg.SecretKey != "" && len(cfg.IPNSecret) >= 16
	reason := ""
	if !configured {
		reason = "not_configured"
	} else if !cfg.Enabled {
		reason = "disabled"
	} else if !isPaymentComplianceConfirmed() {
		reason = "confirmation_required"
	} else if cfg.Environment != "production" && !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		reason = "sandbox_unavailable"
	}
	common.ApiSuccess(c, gin.H{"enabled": reason == "", "bank_enabled": model.SepayBankConfigured(cfg) && reason == "", "unavailable_reason": reason, "min_amount_vnd": 1000, "max_amount_vnd": 1000000})
}

func GetSepayAdminConfig(c *gin.Context) {
	cfg, err := model.GetSepayConfig()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"enabled": cfg.Enabled, "environment": cfg.Environment, "merchant_id": cfg.MerchantID, "secret_key_configured": cfg.SecretKey != "", "ipn_secret_configured": len(cfg.IPNSecret) >= 16, "payment_compliance_confirmed": isPaymentComplianceConfirmed(), "ipn_url": strings.TrimRight(service.GetCallbackAddress(), "/") + "/api/payment/sepay/ipn", "bank_code": cfg.BankCode, "bank_account": cfg.BankAccount, "bank_holder": cfg.BankHolder, "webhook_secret_configured": len(cfg.WebhookSecret) >= 16, "webhook_url": strings.TrimRight(service.GetCallbackAddress(), "/") + "/api/payment/sepay/webhook"})
}

func UpdateSepayAdminConfig(c *gin.Context) {
	var req model.SepayConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "invalid SePay configuration")
		return
	}
	if req.Enabled && !isPaymentComplianceConfirmed() {
		common.ApiErrorMsg(c, "confirm payment compliance before enabling SePay")
		return
	}
	if err := model.SaveSepayConfig(req); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"saved": true})
}

func RequestSepayPay(c *gin.Context) {
	var req sepayCheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "amount_vnd and request_id are required")
		return
	}
	if len(req.RequestID) > 64 || strings.TrimSpace(req.RequestID) == "" {
		common.ApiErrorMsg(c, "invalid request_id")
		return
	}
	cfg, err := model.GetSepayConfig()
	if err != nil || !cfg.Enabled || cfg.MerchantID == "" || cfg.SecretKey == "" || len(cfg.IPNSecret) < 16 {
		common.ApiErrorMsg(c, "SePay is not configured")
		return
	}
	if !isPaymentComplianceConfirmed() {
		common.ApiErrorMsg(c, "payment compliance confirmation is required")
		return
	}
	if cfg.Environment == "sandbox" && !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		common.ApiErrorMsg(c, "Sandbox requires an isolated test deployment")
		return
	}
	if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeCustom {
		common.ApiErrorMsg(c, "SePay requires the configured credit display currency")
		return
	}
	callback := strings.TrimRight(paymentReturnPath(""), "/")
	callbackURL, callbackError := url.Parse(callback)
	if callbackError != nil || callbackURL.Scheme != "https" || callbackURL.Host == "" || callbackURL.RawQuery != "" || callbackURL.Fragment != "" || callbackURL.User != nil {
		common.ApiErrorMsg(c, "a public HTTPS callback domain is required")
		return
	}
	rate := operation_setting.GetUsdToCurrencyRate(operation_setting.USDExchangeRate)
	bankFlow := model.SepayBankConfigured(cfg)
	if req.Flow != "" && req.Flow != "bank" {
		common.ApiErrorMsg(c, "invalid payment flow")
		return
	}
	if req.Flow == "bank" && !bankFlow {
		common.ApiErrorMsg(c, "bank QR connection is not configured")
		return
	}
	var payment *model.SepayPayment
	if bankFlow {
		payment, err = model.CreateSepayBankPayment(c.GetInt("id"), req.RequestID, req.AmountVND, rate, cfg)
	} else {
		payment, err = model.CreateSepayPayment(c.GetInt("id"), req.RequestID, req.AmountVND, rate, cfg)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if bankFlow {
		common.ApiSuccess(c, sepayBankOrderDTO(payment))
		return
	}
	fields := map[string]string{"order_amount": strconv.FormatInt(payment.AmountVND, 10), "merchant": cfg.MerchantID, "currency": "VND", "operation": "PURCHASE", "order_description": "AI cheap wallet credit", "order_invoice_number": payment.OrderInvoiceNumber, "payment_method": "BANK_TRANSFER", "success_url": callback + "/wallet?sepay=success&trade_no=" + payment.OrderInvoiceNumber, "error_url": callback + "/wallet?sepay=error&trade_no=" + payment.OrderInvoiceNumber, "cancel_url": callback + "/wallet?sepay=cancelled&trade_no=" + payment.OrderInvoiceNumber}
	fields["signature"] = sepayCheckoutSignature(fields, cfg.SecretKey)
	orderedFields := make([]gin.H, 0, len(fields))
	for _, name := range []string{"order_amount", "merchant", "currency", "operation", "order_description", "order_invoice_number", "customer_id", "payment_method", "success_url", "error_url", "cancel_url", "signature"} {
		if value, ok := fields[name]; ok {
			orderedFields = append(orderedFields, gin.H{"name": name, "value": value})
		}
	}
	common.ApiSuccess(c, gin.H{"checkout_url": sepayCheckoutURL(cfg.Environment), "fields": orderedFields, "trade_no": payment.OrderInvoiceNumber, "amount_vnd": payment.AmountVND, "credits": payment.Credits, "status": payment.VerificationStatus})
}

func GetSepayOrder(c *gin.Context) {
	p, err := model.GetSepayPayment(c.Param("trade_no"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if p.UserID != c.GetInt("id") {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err := model.ExpireSepayPaymentIfNeeded(c.Param("trade_no")); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiError(c, err)
		return
	}
	if p.VerificationStatus == model.SepayVerificationPending {
		p, err = model.GetSepayPayment(c.Param("trade_no"))
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if p.Flow == "bank" {
		common.ApiSuccess(c, sepayBankOrderDTO(p))
		return
	}
	common.ApiSuccess(c, gin.H{"trade_no": p.OrderInvoiceNumber, "amount_vnd": p.AmountVND, "credits": p.Credits, "status": p.VerificationStatus, "verified_at": p.VerifiedAt})
}

func CancelSepayOrder(c *gin.Context) {
	if err := model.CancelSepayPayment(c.Param("trade_no"), c.GetInt("id")); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"trade_no": c.Param("trade_no"), "status": model.SepayVerificationVoided})
}

func sepayBankOrderDTO(p *model.SepayPayment) gin.H {
	qr := "https://vietqr.app/img?acc=" + url.QueryEscape(p.BankAccount) + "&bank=" + url.QueryEscape(p.BankCode) + "&amount=" + strconv.FormatInt(p.AmountVND, 10) + "&des=" + url.QueryEscape(p.OrderInvoiceNumber) + "&template=compact2&showinfo=true&holder=" + url.QueryEscape(p.BankHolder)
	return gin.H{"flow": "bank", "qr_url": qr, "trade_no": p.OrderInvoiceNumber, "amount_vnd": p.AmountVND, "credits": p.Credits, "status": p.VerificationStatus, "created_at": p.CreatedAt, "expires_at": p.CreatedAt.Add(model.SepayOrderTTL), "bank_code": p.BankCode, "bank_account": p.BankAccount, "bank_holder": p.BankHolder, "transfer_content": p.OrderInvoiceNumber}
}

func SepayIPN(c *gin.Context) {
	cfg, err := model.GetSepayConfig()
	if err != nil || cfg.IPNSecret == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	secret := c.GetHeader("X-Secret-Key")
	if secret == "" || !hmac.Equal([]byte(secret), []byte(cfg.IPNSecret)) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "application/json required"})
		return
	}
	var event sepayIPN
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if event.NotificationType == "TRANSACTION_VOID" {
		p, e := model.GetSepayPayment(event.Order.OrderInvoiceNumber)
		if e != nil || p.MerchantID != cfg.MerchantID || p.MerchantEnvironment != cfg.Environment {
			c.JSON(http.StatusConflict, gin.H{"error": "unknown payment"})
			return
		}
		if e = model.MarkSepayReview(p.OrderInvoiceNumber, "provider reported transaction void; manual review required", true); e != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	if event.NotificationType != "ORDER_PAID" || event.Order.OrderStatus != "CAPTURED" || event.Transaction.TransactionStatus != "APPROVED" || event.Transaction.TransactionType != "PAYMENT" || event.Order.OrderCurrency != "VND" || event.Transaction.TransactionCurrency != "VND" {
		c.JSON(http.StatusConflict, gin.H{"error": "payment not verified"})
		return
	}
	transactionID := event.Transaction.TransactionID
	if transactionID == "" {
		transactionID = event.Transaction.ID
	}
	if event.Order.OrderID == "" || transactionID == "" || event.Order.OrderInvoiceNumber == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "missing provider identifiers"})
		return
	}
	payment, err := model.GetSepayPayment(event.Order.OrderInvoiceNumber)
	if err != nil || payment.MerchantID != cfg.MerchantID || payment.MerchantEnvironment != cfg.Environment || !service.SepayAmountMatches(event.Order.OrderAmount, payment.AmountVND) || !service.SepayAmountMatches(event.Transaction.TransactionAmount, payment.AmountVND) {
		c.JSON(http.StatusConflict, gin.H{"error": "payment mismatch"})
		return
	}
	if payment.VerificationStatus == model.SepayVerificationPaid && payment.OrderID != nil && *payment.OrderID == event.Order.OrderID {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	now := time.Now()
	updates := map[string]any{"ipn_received_at": now, "provider_transaction": transactionID}
	if payment.ProviderTransaction != nil && *payment.ProviderTransaction != transactionID {
		c.JSON(http.StatusConflict, gin.H{"error": "transaction mismatch"})
		return
	}
	result := model.DB.Model(payment).Where("provider_transaction IS NULL OR provider_transaction = ?", transactionID).Updates(updates)
	if result.Error != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	if result.RowsAffected != 1 {
		c.JSON(http.StatusConflict, gin.H{"error": "transaction mismatch"})
		return
	}
	orderID, verifiedTransaction, err := service.VerifySepayPayment(c.Request.Context(), cfg, payment, event.Order.OrderID, transactionID)
	if err == nil {
		_, err = model.CompleteSepayPayment(payment.OrderInvoiceNumber, orderID, verifiedTransaction, c.ClientIP())
	}
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, model.ErrSepayConflict) {
			status = http.StatusConflict
			if markErr := model.MarkSepayReview(payment.OrderInvoiceNumber, "provider evidence conflicts with local order", false); markErr != nil {
				c.Status(http.StatusServiceUnavailable)
				return
			}
		}
		c.JSON(status, gin.H{"error": "payment could not be verified"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// SepayBankWebhook receives SePay's bank-balance webhook for direct VietQR
// orders. It never trusts a browser or a redirect to credit the wallet.
func SepayBankWebhook(c *gin.Context) {
	cfg, err := model.GetSepayConfig()
	if err != nil || len(cfg.WebhookSecret) < 16 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "unauthorized"})
		return
	}
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"success": false})
		return
	}
	body, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false})
		return
	}
	auth := c.GetHeader("Authorization")
	if signature := c.GetHeader("X-SePay-Signature"); signature != "" {
		timestamp, parseErr := strconv.ParseInt(c.GetHeader("X-SePay-Timestamp"), 10, 64)
		if parseErr != nil || timestamp <= 0 || absInt64(time.Now().Unix()-timestamp) > 300 || !strings.HasPrefix(signature, "sha256=") {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "invalid webhook signature"})
			return
		}
		mac := hmac.New(sha256.New, []byte(cfg.WebhookSecret))
		_, _ = mac.Write([]byte(strconv.FormatInt(timestamp, 10) + "."))
		_, _ = mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(expected), []byte(signature)) {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "invalid webhook signature"})
			return
		}
	} else if !strings.HasPrefix(auth, "Apikey ") || !hmac.Equal([]byte(strings.TrimPrefix(auth, "Apikey ")), []byte(cfg.WebhookSecret)) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "unauthorized"})
		return
	}
	var event sepayBankWebhook
	if err := json.Unmarshal(body, &event); err != nil || event.TransferType != "in" || event.TransferAmount <= 0 || event.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid incoming transaction"})
		return
	}
	code := strings.ToUpper(strings.TrimSpace(event.Code))
	matches := regexp.MustCompile(`\bNAP[A-Z0-9]{28}\b`).FindAllString(strings.ToUpper(event.Content), -1)
	if len(matches) > 1 || (len(matches) == 1 && code != "" && matches[0] != code) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "ambiguous payment reference"})
		return
	}
	if code == "" && len(matches) == 1 {
		code = matches[0]
	}
	if !regexp.MustCompile(`^NAP[A-Z0-9]{28}$`).MatchString(code) {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	payment, err := model.GetSepayPayment(code)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// An unrelated bank deposit must never be matched by amount alone.
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false})
		return
	}
	if payment.Flow != "bank" || payment.MerchantEnvironment != "production" || payment.AmountVND != event.TransferAmount || payment.BankAccount == "" || payment.BankAccount != event.AccountNumber || !strings.EqualFold(payment.BankCode, event.Gateway) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "payment details mismatch"})
		return
	}
	if _, err = model.CompleteSepayBankPayment(payment.OrderInvoiceNumber, "bank:"+strconv.FormatInt(event.ID, 10), c.ClientIP()); err != nil {
		if errors.Is(err, model.ErrSepayConflict) {
			c.JSON(http.StatusConflict, gin.H{"success": false})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func SepayReconcile(c *gin.Context) {
	if c.GetInt("role") < common.RoleRootUser {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	p, err := model.GetSepayPayment(c.Param("trade_no"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	err = reconcileSepay(c.Request.Context(), p)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"verified": true})
}

func reconcileSepay(ctx context.Context, p *model.SepayPayment) error {
	if p.VerificationStatus == model.SepayVerificationVoided {
		return model.ErrSepayConflict
	}
	cfg, err := model.GetSepayConfig()
	if err != nil {
		return err
	}
	authenticatedTransaction := ""
	if p.IPNReceivedAt != nil && p.ProviderTransaction != nil {
		authenticatedTransaction = *p.ProviderTransaction
	}
	orderID, transactionID, err := service.VerifySepayPayment(ctx, cfg, p, "", authenticatedTransaction)
	if err != nil {
		return err
	}
	_, err = model.CompleteSepayPayment(p.OrderInvoiceNumber, orderID, transactionID, "")
	return err
}

func StartSepayReconciler(ctx context.Context) {
	// Only the designated master polls; callbacks remain safe on every node.
	if os.Getenv("NODE_TYPE") == "slave" {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var payments []model.SepayPayment
			if err := model.DB.Where("verification_status = ? AND next_reconcile_at <= ? AND reconcile_attempts < 5", model.SepayVerificationPending, time.Now()).Order("next_reconcile_at ASC").Limit(20).Find(&payments).Error; err != nil {
				common.SysError("SePay reconcile query failed")
				continue
			}
			for _, p := range payments {
				if ctx.Err() != nil {
					return
				}
				if p.Flow == "bank" && time.Since(p.CreatedAt) >= model.SepayOrderTTL {
					if err := model.ExpireSepayPaymentIfNeeded(p.OrderInvoiceNumber); err != nil {
						common.SysError("SePay QR expiry update failed")
					}
					continue
				}
				delays := []time.Duration{2 * time.Minute, 5 * time.Minute, 15 * time.Minute, 60 * time.Minute, 60 * time.Minute}
				next := time.Now().Add(delays[p.ReconcileAttempts])
				claimed := model.DB.Model(&model.SepayPayment{}).Where("id = ? AND reconcile_attempts = ? AND verification_status = ?", p.ID, p.ReconcileAttempts, model.SepayVerificationPending).Updates(map[string]any{"reconcile_attempts": p.ReconcileAttempts + 1, "next_reconcile_at": next})
				if claimed.Error != nil || claimed.RowsAffected != 1 {
					continue
				}
				checkCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
				err := reconcileSepay(checkCtx, &p)
				cancel()
				if err != nil {
					updates := map[string]any{"verification_reason": err.Error()}
					if errors.Is(err, model.ErrSepayConflict) || p.ReconcileAttempts >= 4 {
						updates["verification_status"] = model.SepayVerificationReview
					}
					if err := model.DB.Model(&model.SepayPayment{}).Where("id = ? AND verification_status = ?", p.ID, model.SepayVerificationPending).Updates(updates).Error; err != nil {
						common.SysError("SePay reconcile state update failed")
					}
				}
			}
		}
	}
}
