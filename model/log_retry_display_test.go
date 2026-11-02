package model

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func retryDisplayMetadata(t *testing.T, final bool, channels ...int) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"request_final": final,
		"admin_info":    map[string]any{"use_channel": channels},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestUsageLogQueriesHideIntermediateRetryErrorsAndProtectChannelData(t *testing.T) {
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

	if err := db.AutoMigrate(&Log{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE channels (id INTEGER PRIMARY KEY, name TEXT NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO channels (id, name) VALUES (10, 'Fallback'), (11, 'FinalFailure')").Error; err != nil {
		t.Fatal(err)
	}
	rows := []Log{
		{Id: 1, UserId: 1, TokenId: 7, Type: LogTypeError, RequestId: "success-retry", ChannelId: 8, Other: retryDisplayMetadata(t, false, 8)},
		{Id: 2, UserId: 1, TokenId: 7, Type: LogTypeError, RequestId: "success-retry", ChannelId: 9, Other: retryDisplayMetadata(t, false, 8, 9)},
		{Id: 3, UserId: 1, TokenId: 7, Type: LogTypeConsume, RequestId: "success-retry", ChannelId: 10, Other: retryDisplayMetadata(t, true, 8, 9, 10)},
		{Id: 4, UserId: 1, TokenId: 7, Type: LogTypeError, RequestId: "failed-retry", ChannelId: 8, Other: retryDisplayMetadata(t, false, 8)},
		{Id: 5, UserId: 1, TokenId: 7, Type: LogTypeError, RequestId: "failed-retry", ChannelId: 11, Other: retryDisplayMetadata(t, true, 8, 11)},
		{Id: 6, UserId: 1, TokenId: 7, Type: LogTypeError, RequestId: "legacy-final", ChannelId: 11},
	}
	for i := range rows {
		rows[i].CreatedAt = 100
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	assertVisibleRequests := func(name string, logs []*Log, total int64) {
		t.Helper()
		if total != 3 || len(logs) != 3 {
			t.Fatalf("%s returned %d rows (total %d), want 3", name, len(logs), total)
		}
		got := map[string]int{}
		for _, log := range logs {
			got[log.RequestId]++
		}
		if got["success-retry"] != 1 || got["failed-retry"] != 1 || got["legacy-final"] != 1 {
			t.Fatalf("%s returned unexpected request rows: %#v", name, got)
		}
	}

	adminLogs, adminTotal, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 50, 0, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertVisibleRequests("admin log query", adminLogs, adminTotal)
	var successLog *Log
	for _, log := range adminLogs {
		if log.RequestId == "success-retry" {
			successLog = log
			break
		}
	}
	if successLog == nil || successLog.ChannelId != 10 || successLog.ChannelName != "Fallback" {
		t.Fatalf("admin success row lost final channel: %+v", successLog)
	}
	var adminOther map[string]any
	if err := json.Unmarshal([]byte(successLog.Other), &adminOther); err != nil {
		t.Fatal(err)
	}
	if _, ok := adminOther["admin_info"]; !ok {
		t.Fatal("admin retry-chain metadata was removed from admin logs")
	}
	var failedLog *Log
	for _, log := range adminLogs {
		if log.RequestId == "failed-retry" {
			failedLog = log
			break
		}
	}
	if failedLog == nil || failedLog.ChannelId != 11 || failedLog.ChannelName != "FinalFailure" {
		t.Fatalf("admin terminal failure lost last failed channel: %+v", failedLog)
	}
	var failedOther map[string]any
	if err := json.Unmarshal([]byte(failedLog.Other), &failedOther); err != nil {
		t.Fatal(err)
	}
	adminInfo, ok := failedOther["admin_info"].(map[string]any)
	if !ok || adminInfo["use_channel"] == nil {
		t.Fatalf("admin terminal failure lost retry-chain metadata: %#v", failedOther)
	}

	userLogs, userTotal, err := GetUserLogs(1, LogTypeUnknown, 0, 0, "", "", 0, 50, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertVisibleRequests("user log query", userLogs, userTotal)
	for _, log := range userLogs {
		if log.ChannelId != 0 || log.ChannelName != "" || log.UpstreamRequestId != "" {
			t.Fatalf("user log exposed channel/upstream data: %+v", log)
		}
		var other map[string]any
		if log.Other == "" {
			continue
		}
		if err := json.Unmarshal([]byte(log.Other), &other); err != nil {
			t.Fatal(err)
		}
		if _, ok := other["admin_info"]; ok {
			t.Fatalf("user log exposed admin metadata: %s", log.Other)
		}
	}

	keyLogs, err := GetLogByTokenId(7)
	if err != nil {
		t.Fatal(err)
	}
	assertVisibleRequests("token log query", keyLogs, int64(len(keyLogs)))
	for _, log := range keyLogs {
		if log.ChannelId != 0 || log.ChannelName != "" {
			t.Fatalf("token log exposed channel data: %+v", log)
		}
	}

	var stored int64
	if err := db.Model(&Log{}).Count(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored != int64(len(rows)) {
		t.Fatalf("intermediate evidence was deleted: stored %d rows", stored)
	}
}
