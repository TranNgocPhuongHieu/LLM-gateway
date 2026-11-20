package model

import (
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestIsModelOfferedInGroup(t *testing.T) {
	oldDB := DB
	t.Cleanup(func() { DB = oldDB })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Channel{}, &Ability{}); err != nil {
		t.Fatal(err)
	}
	DB = db
	for _, ch := range []struct {
		id     int
		status int
		model  string
	}{
		{1, common.ChannelStatusEnabled, "live-model"},
		{2, common.ChannelStatusAutoDisabled, "flaky-model"},
		{3, common.ChannelStatusManuallyDisabled, "retired-model"},
	} {
		if err := db.Create(&Channel{Id: ch.id, Status: ch.status, Models: ch.model, Group: "default"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&Ability{Group: "default", Model: ch.model, ChannelId: ch.id, Enabled: ch.status == common.ChannelStatusEnabled}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		group, model string
		want         bool
	}{
		{"default", "live-model", true},
		{"default", "flaky-model", true},
		{"default", "retired-model", false},
		{"default", "never-configured", false},
		{"vip", "live-model", false},
		{"", "live-model", false},
		{"default", "", false},
	} {
		got, err := IsModelOfferedInGroup(tc.group, tc.model)
		if err != nil {
			t.Fatalf("%s/%s: %v", tc.group, tc.model, err)
		}
		if got != tc.want {
			t.Errorf("IsModelOfferedInGroup(%q, %q) = %v, want %v", tc.group, tc.model, got, tc.want)
		}
	}

	if err := db.Migrator().DropTable(&Ability{}); err != nil {
		t.Fatal(err)
	}
	if _, err := IsModelOfferedInGroup("default", "live-model"); err == nil {
		t.Fatal("a failed lookup must return an error so callers keep the transient response")
	}
}
