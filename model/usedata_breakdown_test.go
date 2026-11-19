package model

import (
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupSelfUsageTestDB installs an in-memory SQLite database as DB and LOG_DB
// with the tables used by the self-service usage endpoints.
func setupSelfUsageTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	oldDB, oldLogDB := DB, LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldMemoryCache := common.MemoryCacheEnabled
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.MemoryCacheEnabled = oldMemoryCache
		_ = conn.Close()
	})
	if err := db.AutoMigrate(&Log{}, &QuotaData{}, &Token{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestGetUserUsageBreakdownByModelAndToken(t *testing.T) {
	db := setupSelfUsageTestDB(t)
	tokens := []Token{
		{Id: 1, UserId: 1, Key: "key-alpha", Name: "alpha"},
		{Id: 2, UserId: 1, Key: "key-beta", Name: "beta"},
		{Id: 3, UserId: 2, Key: "key-foreign", Name: "foreign-secret"},
	}
	if err := db.Create(&tokens).Error; err != nil {
		t.Fatal(err)
	}
	// Token 2 is soft-deleted: its name must not be returned as a label.
	if err := db.Delete(&Token{}, 2).Error; err != nil {
		t.Fatal(err)
	}
	rows := []QuotaData{
		{UserID: 1, ModelName: "gpt-4o", TokenID: 1, CreatedAt: 1000, Quota: 100, Count: 2, TokenUsed: 50},
		{UserID: 1, ModelName: "gpt-4o", TokenID: 2, CreatedAt: 1000, Quota: 300, Count: 1, TokenUsed: 10},
		{UserID: 1, ModelName: "claude", TokenID: 1, CreatedAt: 2000, Quota: 50, Count: 1, TokenUsed: 5},
		// Foreign token used by the caller's own row must still get an empty label.
		{UserID: 1, ModelName: "claude", TokenID: 3, CreatedAt: 2000, Quota: 7, Count: 1, TokenUsed: 1},
		// Outside the requested range.
		{UserID: 1, ModelName: "gpt-4o", TokenID: 1, CreatedAt: 99999, Quota: 1000, Count: 9, TokenUsed: 9},
		// Another user's usage.
		{UserID: 2, ModelName: "gpt-4o", TokenID: 3, CreatedAt: 1000, Quota: 9000, Count: 9, TokenUsed: 9},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	byModel, err := GetUserUsageBreakdown(1, 500, 5000, "model")
	if err != nil {
		t.Fatal(err)
	}
	if byModel.Total != (UsageBreakdownTotal{Quota: 457, Count: 5, TokenUsed: 66}) {
		t.Fatalf("model total wrong: %+v", byModel.Total)
	}
	if len(byModel.Items) != 2 || byModel.Items[0].Key != "gpt-4o" || byModel.Items[1].Key != "claude" {
		t.Fatalf("model items not sorted by quota desc: %+v", byModel.Items)
	}
	if byModel.Items[0].Quota != 400 || byModel.Items[0].Count != 3 || byModel.Items[0].TokenUsed != 60 || byModel.Items[0].Label != "gpt-4o" {
		t.Fatalf("gpt-4o item wrong: %+v", byModel.Items[0])
	}

	byToken, err := GetUserUsageBreakdown(1, 500, 5000, "token")
	if err != nil {
		t.Fatal(err)
	}
	if byToken.Total.Quota != 457 || len(byToken.Items) != 3 {
		t.Fatalf("token breakdown wrong: %+v", byToken)
	}
	want := []UsageBreakdownItem{
		{Key: "2", Label: "", Quota: 300, Count: 1, TokenUsed: 10},
		{Key: "1", Label: "alpha", Quota: 150, Count: 3, TokenUsed: 55},
		{Key: "3", Label: "", Quota: 7, Count: 1, TokenUsed: 1},
	}
	for i := range want {
		if byToken.Items[i] != want[i] {
			t.Fatalf("token item %d = %+v, want %+v", i, byToken.Items[i], want[i])
		}
	}
}

func TestGetUserUsageBreakdownEmptyAndInvalidGroup(t *testing.T) {
	setupSelfUsageTestDB(t)
	empty, err := GetUserUsageBreakdown(1, 1, 10, "model")
	if err != nil {
		t.Fatal(err)
	}
	if empty.Items == nil || len(empty.Items) != 0 || empty.Total != (UsageBreakdownTotal{}) {
		t.Fatalf("empty breakdown should have zero totals and non-nil items: %+v", empty)
	}
	if _, err := GetUserUsageBreakdown(1, 1, 10, "user"); err == nil {
		t.Fatal("invalid group_by must return an error")
	}
}
