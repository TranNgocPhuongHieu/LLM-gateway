package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"gorm.io/gorm"
)

// setupAdminSummaryTestDB reuses the top-up fixture database and adds the
// user, claim and log tables that the admin summary reads.
func setupAdminSummaryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupAdminTopUpListDB(t)
	if err := db.AutoMigrate(&User{}, &ExternalIdentityClaim{}, &Log{}); err != nil {
		t.Fatal(err)
	}
	oldQuota := common.QuotaForNewUser
	common.QuotaForNewUser = 1000
	t.Cleanup(func() { common.QuotaForNewUser = oldQuota })
	return db
}

func seedAdminSummaryFixtures(t *testing.T, db *gorm.DB) {
	t.Helper()
	// Users: u1 Google signup in range, u2 plain signup in range,
	// u3 Google signup out of range, u4 soft-deleted signup in range.
	deletedAt := gorm.DeletedAt{Time: time.Unix(4000, 0), Valid: true}
	users := []User{
		{Id: 1, Username: "u1", AffCode: "aff-1", CreatedAt: 2000},
		{Id: 2, Username: "u2", AffCode: "aff-2", CreatedAt: 3000},
		{Id: 3, Username: "u3", AffCode: "aff-3", CreatedAt: 9000},
		{Id: 4, Username: "u4", AffCode: "aff-4", CreatedAt: 4000, DeletedAt: deletedAt},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	claims := []ExternalIdentityClaim{
		{Provider: ExternalIdentityProviderGoogle, Subject: "sub-1", UserId: 1},
		{Provider: ExternalIdentityProviderGoogle, Subject: "sub-3", UserId: 3},
		{Provider: ExternalIdentityProviderTelegram, Subject: "tg-2", UserId: 2},
	}
	if err := db.Create(&claims).Error; err != nil {
		t.Fatal(err)
	}

	retry := func(final bool) string {
		encoded, err := json.Marshal(map[string]any{"request_final": final})
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	logs := []Log{
		// Signup grants: two in range, one out of range, one non-grant system log.
		{Id: 1, UserId: 1, Type: LogTypeSystem, CreatedAt: 2000, Content: "新用户注册赠送 $0.002000"},
		{Id: 2, UserId: 2, Type: LogTypeSystem, CreatedAt: 3000, Content: "新用户注册赠送 $0.002000"},
		{Id: 3, UserId: 3, Type: LogTypeSystem, CreatedAt: 9000, Content: "新用户注册赠送 $0.002000"},
		{Id: 4, UserId: 1, Type: LogTypeSystem, CreatedAt: 2100, Content: "管理员调整额度"},
		// Consume logs: users 1 and 2 active in range; user 3 only out of range.
		{Id: 5, UserId: 1, Type: LogTypeConsume, CreatedAt: 2000, ModelName: "gpt-4o", Quota: 100},
		{Id: 6, UserId: 1, Type: LogTypeConsume, CreatedAt: 2500, ModelName: "gpt-4o", Quota: 200},
		{Id: 7, UserId: 2, Type: LogTypeConsume, CreatedAt: 3000, ModelName: "gpt-4o", Quota: 300},
		{Id: 8, UserId: 3, Type: LogTypeConsume, CreatedAt: 9000, ModelName: "gpt-4o", Quota: 400},
		// Error logs: one final and one intermediate retry in range, one final out of range.
		{Id: 9, UserId: 1, Type: LogTypeError, CreatedAt: 2100, Content: "status_code=429, rate limit", Other: retry(true)},
		{Id: 10, UserId: 1, Type: LogTypeError, CreatedAt: 2200, Content: "status_code=503, dial tcp failed", Other: retry(false)},
		{Id: 11, UserId: 2, Type: LogTypeError, CreatedAt: 3100, Content: "status_code=500, boom", Other: retry(true)},
		{Id: 12, UserId: 2, Type: LogTypeError, CreatedAt: 9500, Content: "status_code=500, boom", Other: retry(true)},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
}

func seedAdminSummaryTopUps(t *testing.T, db *gorm.DB) {
	t.Helper()
	topups := []TopUp{
		// SePay success, completed in range (2 SePay rows + 1 Binance + 1 epay expected).
		{Id: 1, UserId: 1, Amount: 1, Money: 500000, TradeNo: "SEPAY-1", PaymentMethod: "sepay_bank_transfer",
			PaymentProvider: PaymentProviderSepay, Status: common.TopUpStatusSuccess, CreateTime: 1900, CompleteTime: 2000},
		// SePay success without complete_time: falls back to create_time (in range).
		{Id: 2, UserId: 2, Amount: 1, Money: 300000, TradeNo: "SEPAY-2", PaymentMethod: "sepay_bank_transfer",
			PaymentProvider: PaymentProviderSepay, Status: common.TopUpStatusSuccess, CreateTime: 1500},
		// SePay success created in range but completed after the range: excluded.
		{Id: 3, UserId: 1, Amount: 1, Money: 999999, TradeNo: "SEPAY-LATE", PaymentMethod: "sepay_bank_transfer",
			PaymentProvider: PaymentProviderSepay, Status: common.TopUpStatusSuccess, CreateTime: 2000, CompleteTime: 9000},
		// Binance success completed in range.
		{Id: 4, UserId: 2, Amount: 1, Money: 260000, TradeNo: "USDT-1", PaymentMethod: "binance_usdt_BEP20",
			PaymentProvider: PaymentProviderBinance, Status: common.TopUpStatusSuccess, CreateTime: 2900, CompleteTime: 3000},
		// Epay success in range: no durable quota, empty currency.
		{Id: 5, UserId: 1, Amount: 1, Money: 7.5, TradeNo: "EPAY-1", PaymentMethod: "alipay",
			PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusSuccess, CreateTime: 3900, CompleteTime: 4000},
		// Pending SePay order flagged for review, created out of range: counted in pending only.
		{Id: 6, UserId: 1, Amount: 1, Money: 100000, TradeNo: "SEPAY-REVIEW", PaymentMethod: "sepay_bank_transfer",
			PaymentProvider: PaymentProviderSepay, Status: common.TopUpStatusPending, CreateTime: 9999},
		// Pending Binance order in review, in range: counted in pending.
		{Id: 7, UserId: 2, Amount: 1, Money: 260000, TradeNo: "USDT-REVIEW", PaymentMethod: "binance_usdt_TRC20",
			PaymentProvider: PaymentProviderBinance, Status: common.TopUpStatusPending, CreateTime: 2600},
		// Pending Epay order with no review state: not counted anywhere.
		{Id: 8, UserId: 1, Amount: 1, Money: 1, TradeNo: "EPAY-PENDING", PaymentMethod: "alipay",
			PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending, CreateTime: 2700},
		// Successful Binance order whose review flag was cleared by settlement: not pending.
		{Id: 9, UserId: 2, Amount: 1, Money: 260000, TradeNo: "USDT-DONE", PaymentMethod: "binance_usdt_BEP20",
			PaymentProvider: PaymentProviderBinance, Status: common.TopUpStatusSuccess, CreateTime: 2800, CompleteTime: 2810},
	}
	if err := db.Create(&topups).Error; err != nil {
		t.Fatal(err)
	}
	sepay := []SepayPayment{
		{TopUpID: 1, UserID: 1, RequestID: "r1", MerchantEnvironment: "test", MerchantID: "m", OrderInvoiceNumber: "INV-1",
			AmountVND: 500000, Credits: 550, QuotaSnapshot: 1000, QuotaPerUnitSnapshot: 1, ExchangeRateSnapshot: 1,
			Currency: "VND", VerificationStatus: SepayVerificationPaid},
		{TopUpID: 2, UserID: 2, RequestID: "r2", MerchantEnvironment: "test", MerchantID: "m", OrderInvoiceNumber: "INV-2",
			AmountVND: 300000, Credits: 300, QuotaSnapshot: 600, QuotaPerUnitSnapshot: 1, ExchangeRateSnapshot: 1,
			Currency: "VND", VerificationStatus: SepayVerificationPaid},
		{TopUpID: 3, UserID: 1, RequestID: "r3", MerchantEnvironment: "test", MerchantID: "m", OrderInvoiceNumber: "INV-3",
			AmountVND: 999999, Credits: 999, QuotaSnapshot: 9999, QuotaPerUnitSnapshot: 1, ExchangeRateSnapshot: 1,
			Currency: "VND", VerificationStatus: SepayVerificationPaid},
		{TopUpID: 6, UserID: 1, RequestID: "r6", MerchantEnvironment: "test", MerchantID: "m", OrderInvoiceNumber: "INV-6",
			AmountVND: 100000, Credits: 100, QuotaSnapshot: 100, QuotaPerUnitSnapshot: 1, ExchangeRateSnapshot: 1,
			Currency: "VND", VerificationStatus: SepayVerificationReview},
	}
	if err := db.Create(&sepay).Error; err != nil {
		t.Fatal(err)
	}
	binance := []BinanceTopUp{
		{TopUpID: 4, UserID: 2, RequestID: "b4", AccountScope: "scope", TradeNo: "USDT-1", Network: "BEP20", Coin: "USDT",
			// Created before the quote fix: quota_snapshot is far too small, but
			// amount_vnd (25 USDT x 26,000) and credits (+10% bonus) are right.
			DepositAddress: "addr", AmountUnits: 25000000, AmountUSDT: "25.000000", AmountVND: 650000, Credits: 715, QuotaSnapshot: 634,
			ExchangeRateSnapshot: "26000", FeePercentSnapshot: "0", BonusSnapshot: "0", Status: BinanceOrderPaid,
			ExpiresAt: time.Unix(5000, 0), LateReconcileUntil: time.Unix(5000, 0)},
		{TopUpID: 7, UserID: 2, RequestID: "b7", AccountScope: "scope", TradeNo: "USDT-REVIEW", Network: "TRC20", Coin: "USDT",
			DepositAddress: "addr", AmountUnits: 10000000, AmountUSDT: "10.000000", Credits: 1000, QuotaSnapshot: 1000,
			ExchangeRateSnapshot: "26000", FeePercentSnapshot: "0", BonusSnapshot: "0", Status: BinanceOrderReview,
			ExpiresAt: time.Unix(5000, 0), LateReconcileUntil: time.Unix(5000, 0)},
		{TopUpID: 9, UserID: 2, RequestID: "b9", AccountScope: "scope", TradeNo: "USDT-DONE", Network: "BEP20", Coin: "USDT",
			DepositAddress: "addr", AmountUnits: 1000000, AmountUSDT: "1.000000", AmountVND: 26000, Credits: 26, QuotaSnapshot: 100,
			ExchangeRateSnapshot: "26000", FeePercentSnapshot: "0", BonusSnapshot: "0", Status: BinanceOrderPaid,
			ExpiresAt: time.Unix(5000, 0), LateReconcileUntil: time.Unix(5000, 0)},
	}
	if err := db.Create(&binance).Error; err != nil {
		t.Fatal(err)
	}
}

func TestGetAdminSummaryAggregatesRangeAndMethods(t *testing.T) {
	db := setupAdminSummaryTestDB(t)
	seedAdminSummaryFixtures(t, db)
	seedAdminSummaryTopUps(t, db)

	summary, err := GetAdminSummary(1000, 5000, false)
	if err != nil {
		t.Fatal(err)
	}

	// Completed in range: TopUp 1, 2, 4, 5 and 9 (TopUp 9 completes at 2810).
	// TopUp 3 (completed 9000) and pending rows are excluded.
	if summary.Topup.Count != 5 {
		t.Fatalf("topup count = %d, want 5", summary.Topup.Count)
	}
	// SePay quota 1000 + 600, Binance quota 634 + 100; Epay has no durable quota.
	if summary.Topup.CreditedQuota != 2334 {
		t.Fatalf("credited quota = %d, want 2334", summary.Topup.CreditedQuota)
	}
	// Paid money comes from the orders, not quota_snapshot: SePay 500,000 +
	// 300,000 VND, Binance 650,000 + 26,000 VND. Bonus: SePay 550,000 - 500,000,
	// Binance 715,000 + 26,000 - 676,000.
	if summary.Topup.PaidVND != 1476000 || summary.Topup.BonusVND != 115000 {
		t.Fatalf("paid/bonus VND = %d/%d, want 1476000/115000", summary.Topup.PaidVND, summary.Topup.BonusVND)
	}
	wantByMethod := []AdminTopUpMethodSummary{
		{Method: "binance", Currency: "USDT", Amount: 26.0, Count: 2, PaidVND: 676000, BonusVND: 65000},
		{Method: "epay", Currency: "", Amount: 7.5, Count: 1},
		{Method: "sepay", Currency: "VND", Amount: 800000, Count: 2, PaidVND: 800000, BonusVND: 50000},
	}
	if !reflect.DeepEqual(summary.Topup.ByMethod, wantByMethod) {
		t.Fatalf("by_method = %+v, want %+v", summary.Topup.ByMethod, wantByMethod)
	}
	// Review badge scope: TopUp 6 (SePay review, out of range) and TopUp 7 (Binance review).
	if summary.Topup.PendingCount != 2 {
		t.Fatalf("pending count = %d, want 2", summary.Topup.PendingCount)
	}
}

func TestGetAdminSummaryEmptyRangeReturnsEmptyByMethod(t *testing.T) {
	db := setupAdminSummaryTestDB(t)
	seedAdminSummaryTopUps(t, db)

	summary, err := GetAdminSummary(100, 200, false)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Topup.ByMethod == nil || len(summary.Topup.ByMethod) != 0 {
		t.Fatalf("by_method must be a non-nil empty slice, got %#v", summary.Topup.ByMethod)
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"by_method":[]`) {
		t.Fatalf("by_method must encode as [], got %s", encoded)
	}
	if summary.Topup.PendingCount != 2 {
		t.Fatalf("pending count must ignore the range, got %d", summary.Topup.PendingCount)
	}
}

func TestGetAdminSummaryUsersAndRequests(t *testing.T) {
	db := setupAdminSummaryTestDB(t)
	seedAdminSummaryFixtures(t, db)

	summary, err := GetAdminSummary(1000, 5000, false)
	if err != nil {
		t.Fatal(err)
	}
	wantUsers := AdminUserSummary{
		NewUsers:           3, // u1, u2 and soft-deleted u4; u3 is out of range
		NewGoogleUsers:     1, // only u1 (u2 has a Telegram claim)
		SignupQuotaGranted: 2000,
		ActiveUsers:        2, // users 1 and 2; user 3 consumed only out of range
	}
	if summary.Users != wantUsers {
		t.Fatalf("users = %+v, want %+v", summary.Users, wantUsers)
	}
	// Final consume logs: 3 in range (ids 5-7). Final errors: ids 9 and 11.
	// Intermediate retry (id 10) and out-of-range rows are excluded.
	wantRequests := AdminRequestSummary{Total: 5, Errors: 2}
	if summary.Requests != wantRequests {
		t.Fatalf("requests = %+v, want %+v", summary.Requests, wantRequests)
	}
	// Consume quota in range: ids 5-7.
	if summary.Usage.ConsumedQuota != 600 {
		t.Fatalf("consumed quota = %d, want 600", summary.Usage.ConsumedQuota)
	}

	// No admin accounts yet: excluding internal accounts changes nothing.
	same, err := GetAdminSummary(1000, 5000, true)
	if err != nil {
		t.Fatal(err)
	}
	if same.Users != summary.Users || same.Requests != summary.Requests || same.Usage != summary.Usage {
		t.Fatalf("exclude_internal without admins = %+v, want %+v", same, summary)
	}
}

func TestGetAdminSummaryExcludeInternalDropsAdminUsage(t *testing.T) {
	db := setupAdminSummaryTestDB(t)
	seedAdminSummaryFixtures(t, db)
	// u1 becomes root. Soft-deleted u4 becomes an admin: it is still internal.
	if err := db.Unscoped().Model(&User{}).Where("id = ?", 1).Update("role", common.RoleRootUser).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Unscoped().Model(&User{}).Where("id = ?", 4).Update("role", common.RoleAdminUser).Error; err != nil {
		t.Fatal(err)
	}
	ids, err := internalUserIDs()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []int{1, 4}) {
		t.Fatalf("internal ids = %v, want [1 4]", ids)
	}

	summary, err := GetAdminSummary(1000, 5000, true)
	if err != nil {
		t.Fatal(err)
	}
	// Only u2 remains: consume id 7 and final error id 11.
	if summary.Users.ActiveUsers != 1 {
		t.Fatalf("active users = %d, want 1", summary.Users.ActiveUsers)
	}
	if want := (AdminRequestSummary{Total: 2, Errors: 1}); summary.Requests != want {
		t.Fatalf("requests = %+v, want %+v", summary.Requests, want)
	}
	if summary.Usage.ConsumedQuota != 300 {
		t.Fatalf("consumed quota = %d, want 300", summary.Usage.ConsumedQuota)
	}
	// Signups stay platform-wide.
	if summary.Users.NewUsers != 3 {
		t.Fatalf("new users = %d, want 3", summary.Users.NewUsers)
	}

	errorsAll, err := GetErrorSummary(0, 1000, 5000, false)
	if err != nil {
		t.Fatal(err)
	}
	errorsCustomers, err := GetErrorSummary(0, 1000, 5000, true)
	if err != nil {
		t.Fatal(err)
	}
	if errorsAll.Total != 2 || errorsCustomers.Total != 1 {
		t.Fatalf("error totals all/customers = %d/%d, want 2/1", errorsAll.Total, errorsCustomers.Total)
	}
	// A specific user id ignores exclude_internal.
	own, err := GetErrorSummary(1, 1000, 5000, true)
	if err != nil {
		t.Fatal(err)
	}
	if own.Total != 1 {
		t.Fatalf("user 1 error total = %d, want 1", own.Total)
	}

	// The admin log list follows the same filter.
	countByInternal := func(excludeInternal bool) (internal, customer int) {
		t.Helper()
		logs, _, err := GetAllLogs(LogTypeUnknown, 1000, 5000, "", "", "", 0, 100, 0, "", "", "", excludeInternal)
		if err != nil {
			t.Fatal(err)
		}
		for _, log := range logs {
			if log.UserId == 1 || log.UserId == 4 {
				internal++
			} else {
				customer++
			}
		}
		return internal, customer
	}
	allInternal, allCustomer := countByInternal(false)
	keptInternal, keptCustomer := countByInternal(true)
	if allInternal == 0 || keptInternal != 0 || keptCustomer == 0 || keptCustomer != allCustomer {
		t.Fatalf("log rows internal/customer: all %d/%d, excluded %d/%d", allInternal, allCustomer, keptInternal, keptCustomer)
	}
}

func TestGetErrorSummarySystemWideAndSelfUnchanged(t *testing.T) {
	db := setupSelfUsageTestDB(t)
	retry := func(final bool) string {
		encoded, err := json.Marshal(map[string]any{"request_final": final})
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	rows := []Log{
		{UserId: 1, Type: LogTypeError, CreatedAt: 1000, ModelName: "gpt-a", Content: "status_code=429, rate limit reached", Other: retry(true)},
		{UserId: 2, Type: LogTypeError, CreatedAt: 1500, ModelName: "gpt-b", Content: "status_code=403, token quota is not enough", Other: retry(true)},
		{UserId: 2, Type: LogTypeError, CreatedAt: 1600, ModelName: "gpt-b", Content: "status_code=403, token quota is not enough", Other: retry(false)},
		{UserId: 3, Type: LogTypeError, CreatedAt: 9000, ModelName: "gpt-c", Content: "status_code=500, boom", Other: retry(true)},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	all, err := GetErrorSummary(0, 500, 5000, false)
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 2 || all.Truncated || len(all.Categories) != 2 {
		t.Fatalf("system-wide summary = %+v, want 2 final rows across users", all)
	}

	self, err := GetUserErrorSummary(2, 500, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if self.Total != 1 || len(self.Categories) != 1 || self.Categories[0].Category != "key_quota_exhausted" {
		t.Fatalf("self summary for user 2 = %+v, want only its final row", self)
	}

	// The self path must not widen when called with user id 0.
	zero, err := GetUserErrorSummary(0, 500, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if zero.Total != 0 || len(zero.Categories) != 0 {
		t.Fatalf("self summary for user 0 = %+v, want empty", zero)
	}

	filtered, err := GetErrorSummary(1, 500, 5000, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(filtered, mustUserErrorSummary(t, 1, 500, 5000)) {
		t.Fatalf("GetErrorSummary(1) must equal GetUserErrorSummary(1)")
	}
}

func mustUserErrorSummary(t *testing.T, userId int, start, end int64) *ErrorSummary {
	t.Helper()
	summary, err := GetUserErrorSummary(userId, start, end)
	if err != nil {
		t.Fatal(err)
	}
	return summary
}

func TestGetAllQuotaDatesExcludeInternal(t *testing.T) {
	db := setupAdminSummaryTestDB(t)
	if err := db.AutoMigrate(&QuotaData{}); err != nil {
		t.Fatal(err)
	}
	users := []User{
		{Id: 1, Username: "root", AffCode: "aff-1", Role: common.RoleRootUser},
		{Id: 2, Username: "customer", AffCode: "aff-2", Role: common.RoleCommonUser},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	rows := []QuotaData{
		{UserID: 1, Username: "root", ModelName: "m", UseGroup: "default", CreatedAt: 3600, Count: 1, Quota: 100},
		{UserID: 2, Username: "customer", ModelName: "m", UseGroup: "default", CreatedAt: 3600, Count: 2, Quota: 30},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	all, err := GetAllQuotaDates(0, 7200, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Quota != 130 || all[0].Count != 3 {
		t.Fatalf("all rows = %+v, want one bucket with quota 130", all)
	}
	customers, err := GetAllQuotaDates(0, 7200, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(customers) != 1 || customers[0].Quota != 30 || customers[0].Count != 2 {
		t.Fatalf("customer rows = %+v, want one bucket with quota 30", customers)
	}

	flowAll, err := GetFlowQuotaData(0, 7200, "", 0, common.RoleRootUser, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(flowAll) != 2 {
		t.Fatalf("root flow rows = %+v, want 2", flowAll)
	}
	flowCustomers, err := GetFlowQuotaData(0, 7200, "", 0, common.RoleRootUser, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(flowCustomers) != 1 || flowCustomers[0].UserID != 2 {
		t.Fatalf("root flow customer rows = %+v, want only user 2", flowCustomers)
	}
	// The self view ignores exclude_internal.
	self, err := GetFlowQuotaData(0, 7200, "", 1, common.RoleCommonUser, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(self) != 1 || self[0].Quota != 100 {
		t.Fatalf("self flow rows = %+v, want the root user's own row", self)
	}
}
