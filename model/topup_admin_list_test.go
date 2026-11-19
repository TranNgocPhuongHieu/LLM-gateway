package model

import (
	"reflect"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Local day used by the fixtures: [adminListDayStart, adminListDayEnd).
const (
	adminListDayStart int64 = 2000000000
	adminListDayEnd         = adminListDayStart + 86400
)

func setupAdminTopUpListDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	connection.SetMaxOpenConns(1)
	oldDB, oldLogs, oldRedis := DB, LOG_DB, common.RedisEnabled
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB, common.RedisEnabled = db, db, false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB, LOG_DB, common.RedisEnabled = oldDB, oldLogs, oldRedis
		common.SetDatabaseTypes(oldMainType, oldLogType)
		_ = connection.Close()
	})
	if err := db.AutoMigrate(&TopUp{}, &SepayPayment{}, &BinanceTopUp{}, &BinanceDeposit{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// seedAdminTopUpListFixtures creates ten top-ups covering every filter branch.
// Ids 1-10; ids 1-3 and 10 are successful, id 2 is the only one on the previous day.
func seedAdminTopUpListFixtures(t *testing.T, db *gorm.DB) {
	t.Helper()
	type fixture struct {
		id       int
		trade    string
		provider string
		status   string
		money    float64
		at       int64
		sepay    string // SePay verification status; empty when no SePay order
		amount   int64  // SePay amount_vnd
		binance  string // Binance order status; empty when no USDT order
		tx       string // matched deposit TxID
	}
	day := adminListDayStart
	fixtures := []fixture{
		{1, "ORDER-SEPAY-OK", PaymentProviderSepay, common.TopUpStatusSuccess, 1, day + 100, "paid", 500000, "", ""},
		{2, "ORDER-SEPAY-OLD", PaymentProviderSepay, common.TopUpStatusSuccess, 1, day - 100, "paid", 300000, "", ""},
		{3, "ORDER-USDT-OK", PaymentProviderBinance, common.TopUpStatusSuccess, 260000, day + 200, "", 0, "paid", "0xDEADBEEF"},
		{4, "ORDER-SEPAY-PEND", PaymentProviderSepay, common.TopUpStatusPending, 1, day + 300, "pending", 100000, "", ""},
		{5, "ORDER-SEPAY-EXP", PaymentProviderSepay, common.TopUpStatusExpired, 1, day + 400, "voided", 200000, "", ""},
		{6, "ORDER-USDT-REVIEW", PaymentProviderBinance, common.TopUpStatusPending, 1, day + 500, "", 0, "review", "0xREVIEW01"},
		{7, "ORDER-USDT-EXP", PaymentProviderBinance, common.TopUpStatusExpired, 1, day + 600, "", 0, "expired", "0xEXPIRED1"},
		{8, "ORDER-SEPAY-REVIEW", PaymentProviderSepay, common.TopUpStatusPending, 1, day + 700, "review", 150000, "", ""},
		{9, "ORDER-USDT-PARTIAL", PaymentProviderBinance, common.TopUpStatusPending, 1, day - 500, "", 0, "partial", "0xPARTIAL1"},
		{10, "ORDER-USDT-SUCCESS-REVIEW", PaymentProviderBinance, common.TopUpStatusSuccess, 100000, day + 800, "", 0, "review", "0xSUCCESS1"},
	}
	var deposit int64
	for _, f := range fixtures {
		topup := TopUp{Id: f.id, UserId: 1, Amount: 1, Money: f.money, TradeNo: f.trade,
			PaymentMethod: f.provider, PaymentProvider: f.provider, Status: f.status, CreateTime: f.at}
		if err := db.Create(&topup).Error; err != nil {
			t.Fatal(err)
		}
		if f.sepay != "" {
			payment := SepayPayment{TopUpID: f.id, UserID: 1, RequestID: f.trade, MerchantEnvironment: "test",
				MerchantID: "merchant", OrderInvoiceNumber: f.trade, AmountVND: f.amount, Credits: 1,
				Currency: "VND", VerificationStatus: f.sepay, Flow: "bank"}
			if err := db.Create(&payment).Error; err != nil {
				t.Fatal(err)
			}
		}
		if f.binance != "" {
			deposit++
			dep := BinanceDeposit{ID: deposit, AccountScope: "scope", ProviderDepositID: f.tx, Coin: "USDT",
				Network: "BEP20", Address: "addr", AmountUSDT: "10", TxID: f.tx, MatchStatus: "matched"}
			if err := db.Create(&dep).Error; err != nil {
				t.Fatal(err)
			}
			// Distinct AmountUnits: older schemas keep a unique (scope, network, amount) index.
			order := BinanceTopUp{TopUpID: f.id, UserID: 1, RequestID: f.trade, AccountScope: "scope",
				TradeNo: f.trade, Network: "BEP20", DepositAddress: "addr", AmountUnits: int64(f.id), AmountUSDT: "10",
				Status: f.binance, MatchedDepositID: &deposit}
			if err := db.Create(&order).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
}

func adminListIDs(topups []*TopUp) []int {
	ids := make([]int, 0, len(topups))
	for _, topup := range topups {
		ids = append(ids, topup.Id)
	}
	return ids
}

func TestListAdminTopUpsFilters(t *testing.T) {
	db := setupAdminTopUpListDB(t)
	seedAdminTopUpListFixtures(t, db)
	page := &common.PageInfo{Page: 1, PageSize: 50}

	for _, tc := range []struct {
		name   string
		filter AdminTopUpFilter
		want   []int
	}{
		{"no filter", AdminTopUpFilter{}, []int{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}},
		{"status success", AdminTopUpFilter{Status: AdminTopUpStatusSuccess}, []int{10, 3, 2, 1}},
		{"status expired", AdminTopUpFilter{Status: AdminTopUpStatusExpired}, []int{7, 5}},
		{"status needs review includes SePay review and excludes successful USDT", AdminTopUpFilter{Status: AdminTopUpStatusNeedsReview}, []int{9, 8, 7, 6}},
		{"needs review with SePay method keeps only SePay review", AdminTopUpFilter{Status: AdminTopUpStatusNeedsReview, PaymentMethod: AdminTopUpMethodSepay}, []int{8}},
		{"method sepay", AdminTopUpFilter{PaymentMethod: AdminTopUpMethodSepay}, []int{8, 5, 4, 2, 1}},
		{"method binance", AdminTopUpFilter{PaymentMethod: AdminTopUpMethodBinance}, []int{10, 9, 7, 6, 3}},
		{"time from local day start", AdminTopUpFilter{StartTime: adminListDayStart}, []int{10, 8, 7, 6, 5, 4, 3, 1}},
		{"time window", AdminTopUpFilter{StartTime: adminListDayStart + 150, EndTime: adminListDayStart + 450}, []int{5, 4, 3}},
		{"keyword matches binance TxID case-insensitively", AdminTopUpFilter{Keyword: "deadbeef"}, []int{3}},
		{"keyword matches order ID case-insensitively", AdminTopUpFilter{Keyword: "order-sepay-ok"}, []int{1}},
		{"keyword matches TxID of a review order", AdminTopUpFilter{Keyword: "REVIEW01"}, []int{6}},
		{"keyword wildcard is literal", AdminTopUpFilter{Keyword: "_"}, []int{}},
		{"keyword and status combine", AdminTopUpFilter{Keyword: "usdt", Status: AdminTopUpStatusNeedsReview}, []int{9, 7, 6}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			topups, total, err := ListAdminTopUps(tc.filter, page)
			if err != nil {
				t.Fatal(err)
			}
			if got := adminListIDs(topups); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ids = %v, want %v", got, tc.want)
			}
			if total != int64(len(tc.want)) {
				t.Fatalf("total = %d, want %d", total, len(tc.want))
			}
		})
	}
}

func TestListAdminTopUpsPaginatesAndRejectsUnknownFilters(t *testing.T) {
	db := setupAdminTopUpListDB(t)
	seedAdminTopUpListFixtures(t, db)

	topups, total, err := ListAdminTopUps(AdminTopUpFilter{}, &common.PageInfo{Page: 2, PageSize: 3})
	if err != nil {
		t.Fatal(err)
	}
	if total != 10 || !reflect.DeepEqual(adminListIDs(topups), []int{7, 6, 5}) {
		t.Fatalf("page 2 = %v total %d, want [7 6 5] total 10", adminListIDs(topups), total)
	}

	if _, _, err := ListAdminTopUps(AdminTopUpFilter{Status: "bogus"}, &common.PageInfo{Page: 1, PageSize: 10}); err == nil {
		t.Fatal("unknown status filter must fail")
	}
	if _, _, err := ListAdminTopUps(AdminTopUpFilter{PaymentMethod: "paypal"}, &common.PageInfo{Page: 1, PageSize: 10}); err == nil {
		t.Fatal("unknown payment method filter must fail")
	}
}

func TestSearchAllTopUpsMatchesOrderIDAndTxID(t *testing.T) {
	db := setupAdminTopUpListDB(t)
	seedAdminTopUpListFixtures(t, db)
	page := &common.PageInfo{Page: 1, PageSize: 10}

	topups, _, err := SearchAllTopUps("0xdeadbeef", page)
	if err != nil {
		t.Fatal(err)
	}
	if got := adminListIDs(topups); !reflect.DeepEqual(got, []int{3}) {
		t.Fatalf("TxID search ids = %v, want [3]", got)
	}
	topups, _, err = SearchAllTopUps("ORDER-SEPAY-OK", page)
	if err != nil {
		t.Fatal(err)
	}
	if got := adminListIDs(topups); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("order ID search ids = %v, want [1]", got)
	}
}

func TestGetTopUpSummarySumsCollectedAndTodayVND(t *testing.T) {
	db := setupAdminTopUpListDB(t)
	seedAdminTopUpListFixtures(t, db)

	summary, err := GetTopUpSummary(adminListDayStart, adminListDayEnd)
	if err != nil {
		t.Fatal(err)
	}
	want := TopUpSummary{
		Total: 10, Pending: 4, Success: 4, Cancelled: 2, Processing: 4, NeedsReview: 4,
		// Successful rows: SePay amount where present, otherwise money.
		// Row 2 (previous day) counts in collected only.
		CollectedVND: 500000 + 300000 + 260000 + 100000,
		// Rows 1, 3 and 10 fall on the local day; row 2 does not.
		TodayVND: 500000 + 260000 + 100000,
	}
	if summary != want {
		t.Fatalf("summary = %+v, want %+v", summary, want)
	}
}
