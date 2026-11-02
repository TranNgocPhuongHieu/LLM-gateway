package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRegistrationWithoutReferralOrPersonalSidebar(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	connection.SetMaxOpenConns(1)
	oldDB, oldLogDB, oldQuota := DB, LOG_DB, common.QuotaForNewUser
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB, common.QuotaForNewUser = db, db, 123
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB, LOG_DB, common.QuotaForNewUser = oldDB, oldLogDB, oldQuota
		common.RedisEnabled = oldRedis
		common.SetDatabaseTypes(oldMainType, oldLogType)
		_ = connection.Close()
	})
	if err := db.AutoMigrate(&User{}, &Log{}); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"first", "second", "admin-created"} {
		user := User{Username: name, Password: "fixture-password", Role: common.RoleCommonUser, InviterId: 99}
		if i == 2 {
			err = db.Transaction(func(tx *gorm.DB) error { return user.InsertWithTx(tx) })
			if err == nil {
				user.FinishInsert()
			}
		} else {
			err = user.Insert()
		}
		if err != nil {
			t.Fatal(err)
		}
		if user.Quota != 123 || user.InviterId != 0 || user.AffQuota != 0 || user.AffCount != 0 || user.AffHistoryQuota != 0 {
			t.Fatalf("unexpected wallet/referral state for %s", name)
		}
		if user.Password == "fixture-password" {
			t.Fatal("password was not hashed")
		}
		if user.GetSetting().SidebarModules != "" {
			t.Fatal("personal sidebar initialized")
		}
	}
	var count int64
	if err := db.Model(&User{}).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("users=%d, error=%v", count, err)
	}
}
