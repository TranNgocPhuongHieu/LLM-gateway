package model

import (
	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"sync"
	"testing"
)

func TestStopBatchUpdaterFlushesPendingQuota(t *testing.T) {
	oldDB, oldEnabled, oldInterval := DB, common.BatchUpdateEnabled, common.BatchUpdateInterval
	oldStop, oldDone := batchUpdaterStop, batchUpdaterDone
	batchUpdaterStop, batchUpdaterDone = make(chan struct{}), make(chan struct{})
	batchUpdaterStopOnce = sync.Once{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() {
		DB, common.BatchUpdateEnabled, common.BatchUpdateInterval = oldDB, oldEnabled, oldInterval
		batchUpdaterStop, batchUpdaterDone = oldStop, oldDone
		batchUpdaterStopOnce = sync.Once{}
		conn.Close()
	})
	DB, common.BatchUpdateEnabled, common.BatchUpdateInterval = db, true, 3600
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&User{Id: 1, Username: "shutdown-test", AffCode: "shutdown-test", Quota: 1000}).Error; err != nil {
		t.Fatal(err)
	}
	InitBatchUpdater()
	addNewRecord(BatchUpdateTypeUserQuota, 1, -100)
	StopBatchUpdater()
	StopBatchUpdater()
	var user User
	if err := db.First(&user, 1).Error; err != nil {
		t.Fatal(err)
	}
	if user.Quota != 900 {
		t.Fatalf("pending quota not flushed: %d", user.Quota)
	}
}
