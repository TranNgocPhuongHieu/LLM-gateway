package model

import (
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestChannelCachePreservesLastGoodOnReadFailure(t *testing.T) {
	for _, table := range []string{"channels", "abilities", "missing-group"} {
		t.Run(table, func(t *testing.T) {
			setupRetryCandidates(t, false)
			channelSyncLock.RLock()
			oldAdvanced := channel2advancedCustomConfig
			channelSyncLock.RUnlock()
			updatePricingLock.Lock()
			oldPricing, oldVendors, oldTime := pricingMap, vendorsList, lastGetPricingTime
			updatePricingLock.Unlock()
			t.Cleanup(func() {
				channelSyncLock.Lock()
				channel2advancedCustomConfig = oldAdvanced
				channelSyncLock.Unlock()
				updatePricingLock.Lock()
				pricingMap, vendorsList, lastGetPricingTime = oldPricing, oldVendors, oldTime
				updatePricingLock.Unlock()
			})
			if err := DB.Model(&Channel{}).Where("id > ?", 0).Updates(map[string]any{"models": retryTestModel, "group": "default"}).Error; err != nil {
				t.Fatal(err)
			}
			common.MemoryCacheEnabled = true
			if err := InitChannelCache(); err != nil {
				t.Fatal(err)
			}
			channelSyncLock.RLock()
			before := channelsIDM[8]
			channelSyncLock.RUnlock()
			updatePricingLock.Lock()
			pricingMap = []Pricing{{ModelName: retryTestModel}}
			lastGetPricingTime = time.Now()
			pricingTime := lastGetPricingTime
			updatePricingLock.Unlock()
			if table == "missing-group" {
				if err := DB.Model(&Channel{}).Where("id = ?", 8).Update("group", "missing").Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := DB.Callback().Query().Before("gorm:query").Register("test:cache_read_failure", func(tx *gorm.DB) {
				deadline, ok := tx.Statement.Context.Deadline()
				if !ok || time.Until(deadline) > 5*time.Second {
					t.Error("cache read has no bounded deadline")
				}
				if tx.Statement.Table == table {
					tx.AddError(errors.New("synthetic cache read failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("cache refresh panicked: %v", r)
					}
				}()
				if err := InitChannelCache(); err == nil {
					t.Error("cache read/inconsistency failure not returned")
				}
			}()
			channelSyncLock.RLock()
			preserved := channelsIDM[8] == before && len(group2model2channels["default"][retryTestModel]) == 5
			channelSyncLock.RUnlock()
			if !preserved {
				t.Error("failed DB read replaced last good routing cache")
			}
			updatePricingLock.Lock()
			pricingPreserved := len(pricingMap) == 1 && lastGetPricingTime == pricingTime
			updatePricingLock.Unlock()
			if !pricingPreserved {
				t.Error("failed DB read invalidated good pricing")
			}
			var readers sync.WaitGroup
			for range 4 {
				readers.Go(func() {
					for range 25 {
						ch, err := GetRandomSatisfiedChannel("default", retryTestModel, 0, nil)
						if err != nil || ch == nil {
							t.Error("last good cache unavailable to concurrent reader")
						}
					}
				})
			}
			for range 5 {
				if err := InitChannelCache(); err == nil {
					t.Error("failed refresh reported success")
				}
			}
			readers.Wait()
			if err := DB.Callback().Query().Remove("test:cache_read_failure"); err != nil {
				t.Fatal(err)
			}
			if err := DB.Model(&Channel{}).Where("id = ?", 8).Updates(map[string]any{"priority": 99, "group": "default"}).Error; err != nil {
				t.Fatal(err)
			}
			if err := InitChannelCache(); err != nil {
				t.Fatal(err)
			}
			ch, err := GetRandomSatisfiedChannel("default", retryTestModel, 0, nil)
			if err != nil || ch == nil || ch.Id != 8 || ch.GetPriority() != 99 {
				t.Fatalf("refresh did not recover: channel=%v err=%v", ch, err)
			}
			updatePricingLock.Lock()
			invalidated := len(pricingMap) == 0 && lastGetPricingTime.IsZero()
			updatePricingLock.Unlock()
			if !invalidated {
				t.Error("successful recovery did not invalidate pricing")
			}
			if err := DB.Where("id > ?", 0).Delete(&Channel{}).Error; err != nil {
				t.Fatal(err)
			}
			if err := InitChannelCache(); err != nil {
				t.Fatal(err)
			}
			channelSyncLock.RLock()
			empty := len(channelsIDM) == 0 && len(channel2advancedCustomConfig) == 0
			channelSyncLock.RUnlock()
			if !empty {
				t.Error("a legitimately empty database must clear cached channels")
			}
			// Empty-model channels have no abilities/routes and remain valid.
			if err := DB.Create(&Channel{Id: 99, Type: 1, Status: common.ChannelStatusEnabled, Group: "empty"}).Error; err != nil {
				t.Fatal(err)
			}
			if err := InitChannelCache(); err != nil {
				t.Fatalf("empty-model channel must not fail initialization: %v", err)
			}
			// With caching disabled there must be no database read at all.
			common.MemoryCacheEnabled = false
			if err := DB.Callback().Query().Before("gorm:query").Register("test:unexpected_cache_read", func(tx *gorm.DB) {
				t.Error("cache-disabled refresh queried database")
				tx.AddError(errors.New("unexpected read"))
			}); err != nil {
				t.Fatal(err)
			}
			if err := InitChannelCache(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestChannelRoutingMutationsRollback(t *testing.T) {
	setupRetryCandidates(t, false)
	tag, replacement := "fixture-tag", "replacement-tag"
	if err := DB.Model(&Channel{}).Where("id IN ?", []int{8, 9}).Updates(map[string]any{"tag": tag, "models": retryTestModel}).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var rows []Channel
		var abilities []Ability
		if err := DB.Order("id").Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if err := DB.Order("channel_id").Find(&abilities).Error; err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal([]any{rows, abilities})
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	before := snapshot()
	if err := DB.Callback().Update().Before("gorm:update").Register("test:fail_ability_update", func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("synthetic ability failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := DisableChannelByTag(tag); err == nil {
		t.Fatal("tag status failure hidden")
	}
	if changed, err := UpdateChannelStatus(8, "", common.ChannelStatusManuallyDisabled, "fixture"); changed || err == nil {
		t.Fatal("status failure reported success")
	}
	if snapshot() != before {
		t.Fatal("status transaction did not rollback")
	}
	if err := DB.Callback().Update().Remove("test:fail_ability_update"); err != nil {
		t.Fatal(err)
	}
	if err := DB.Callback().Create().Before("gorm:create").Register("test:fail_ability_create", func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("synthetic ability failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	priority, weight := int64(99), uint(99)
	if err := EditChannelByTag(tag, &replacement, nil, nil, nil, &priority, &weight, nil, nil); err == nil {
		t.Fatal("tag edit failure hidden")
	}
	if err := BatchSetChannelTag([]int{8, 9}, &replacement); err == nil {
		t.Fatal("batch tag failure hidden")
	}
	if snapshot() != before {
		t.Fatal("tag transaction did not rollback")
	}
	if err := DB.Callback().Create().Remove("test:fail_ability_create"); err != nil {
		t.Fatal(err)
	}
	if err := BatchSetChannelTag([]int{8, 9}, &replacement); err != nil {
		t.Fatal(err)
	}
	var abilities []Ability
	if err := DB.Where("channel_id IN ?", []int{8, 9}).Find(&abilities).Error; err != nil {
		t.Fatal(err)
	}
	if len(abilities) != 2 {
		t.Fatalf("got %d abilities", len(abilities))
	}
	for _, ability := range abilities {
		if ability.Tag == nil || *ability.Tag != replacement {
			t.Fatal("batch tag not propagated")
		}
	}
}

const retryTestModel = "gpt-6.1-sol"

func setupRetryCandidates(t *testing.T, cached bool) {
	t.Helper()
	oldCache, oldDB := common.MemoryCacheEnabled, DB
	oldGroupCol := commonGroupCol
	commonGroupCol = `"group"`
	common.MemoryCacheEnabled = cached
	channelSyncLock.Lock()
	oldChannels, oldGroups := channelsIDM, group2model2channels
	channelsIDM = map[int]*Channel{}
	group2model2channels = map[string]map[string][]int{
		"default": {retryTestModel: {8, 9, 11, 12, 10}},
	}
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		common.MemoryCacheEnabled, DB = oldCache, oldDB
		commonGroupCol = oldGroupCol
		channelSyncLock.Lock()
		channelsIDM, group2model2channels = oldChannels, oldGroups
		channelSyncLock.Unlock()
	})
	if !cached {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		conn, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		conn.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = conn.Close() })
		DB = db
		if err := db.AutoMigrate(&Channel{}, &Ability{}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int{8, 9, 11, 12, 10} {
		priority, weight := int64(1), uint(0)
		if id == 10 {
			priority = 0
		}
		ch := &Channel{Id: id, Type: 1, Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight}
		channelSyncLock.Lock()
		channelsIDM[id] = ch
		channelSyncLock.Unlock()
		if !cached {
			if err := DB.Create(ch).Error; err != nil {
				t.Fatal(err)
			}
			ability := &Ability{Group: "default", Model: retryTestModel, ChannelId: id, Enabled: true, Priority: &priority}
			if err := DB.Create(ability).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestRetryCandidatesExhaustPriorityBeforeFallback(t *testing.T) {
	for _, cached := range []bool{true, false} {
		name := "database"
		if cached {
			name = "cache"
		}
		t.Run(name, func(t *testing.T) {
			setupRetryCandidates(t, cached)
			var attempted []int
			for attempt := 0; attempt < 5; attempt++ {
				ch, err := GetRandomSatisfiedChannel("default", retryTestModel, attempt, nil, attempted...)
				if err != nil || ch == nil {
					t.Fatalf("attempt %d: channel=%v err=%v", attempt, ch, err)
				}
				if slices.Contains(attempted, ch.Id) {
					t.Fatalf("retried channel %d: %v", ch.Id, attempted)
				}
				if attempt < 4 && ch.Id == 10 {
					t.Fatalf("fell back before exhausting primary channels: %v", attempted)
				}
				if attempt == 4 && ch.Id != 10 {
					t.Fatalf("expected fallback, got %d", ch.Id)
				}
				attempted = append(attempted, ch.Id)
			}
			ch, err := GetRandomSatisfiedChannel("default", retryTestModel, 5, nil, attempted...)
			if err != nil || ch != nil {
				t.Fatalf("exhausted candidates: channel=%v err=%v", ch, err)
			}
		})
	}
}

func TestRetryCandidatesLegacySelectionUnchanged(t *testing.T) {
	for _, cached := range []bool{true, false} {
		t.Run(map[bool]string{true: "cache", false: "database"}[cached], func(t *testing.T) {
			setupRetryCandidates(t, cached)
			ch, err := GetRandomSatisfiedChannel("default", retryTestModel, 1, nil)
			if err != nil || ch == nil || ch.Id != 10 {
				t.Fatalf("legacy retry changed: channel=%v err=%v", ch, err)
			}
		})
	}
}

func TestRetryCandidatesKeepPathAndIdentityFilters(t *testing.T) {
	for _, cached := range []bool{true, false} {
		t.Run(map[bool]string{true: "cache", false: "database"}[cached], func(t *testing.T) {
			setupRetryCandidates(t, cached)
			filters := []dto.ChannelFilter{
				{Kind: dto.FilterRequestPath, RequestPath: "/v1/chat/completions"},
				{Kind: dto.FilterTaskPluginIdentity, TaskPluginKey: "unrelated-plugin"},
			}
			ch, err := GetRandomSatisfiedChannel("default", retryTestModel, 1, filters, 8)
			if err != nil || ch != nil {
				t.Fatalf("filters bypassed: channel=%v err=%v", ch, err)
			}
		})
	}
}

func TestRetryCandidatesAreRequestScoped(t *testing.T) {
	for _, cached := range []bool{true, false} {
		t.Run(map[bool]string{true: "cache", false: "database"}[cached], func(t *testing.T) {
			setupRetryCandidates(t, cached)
			excluded := []int{8, 9, 11, 12}
			ch, err := GetRandomSatisfiedChannel("default", retryTestModel, 4, nil, excluded...)
			if err != nil || ch == nil || ch.Id != 10 {
				t.Fatalf("expected fallback: channel=%v err=%v", ch, err)
			}
			ch, err = GetRandomSatisfiedChannel("default", retryTestModel, 0, nil)
			if err != nil || ch == nil || ch.Id == 10 {
				t.Fatalf("another request lost its primary candidates: channel=%v err=%v", ch, err)
			}
			if !slices.Equal(excluded, []int{8, 9, 11, 12}) {
				t.Fatalf("exclusion list was mutated: %v", excluded)
			}
		})
	}
}

func TestRetryCandidatesHonorModelAndGroup(t *testing.T) {
	for _, cached := range []bool{true, false} {
		t.Run(map[bool]string{true: "cache", false: "database"}[cached], func(t *testing.T) {
			setupRetryCandidates(t, cached)
			for _, request := range []struct{ group, model string }{
				{"not-authorized", retryTestModel}, {"default", "not-supported"},
			} {
				ch, err := GetRandomSatisfiedChannel(request.group, request.model, 1, nil, 8)
				if err != nil || ch != nil {
					t.Fatalf("routing scope bypassed: channel=%v err=%v", ch, err)
				}
			}
		})
	}
}

func TestRetryCandidatesFallbackToAlternativeHostAfterOneFailure(t *testing.T) {
	for _, cached := range []bool{true, false} {
		t.Run(map[bool]string{true: "cache", false: "database"}[cached], func(t *testing.T) {
			setupRetryCandidates(t, cached)
			xpikiURL := "https://api.xpiki.com"
			routerURL := "https://9router.trnhieu.dev"
			channelSyncLock.Lock()
			for _, id := range []int{8, 9, 11, 12} {
				channelsIDM[id].BaseURL = &xpikiURL
			}
			channelsIDM[10].BaseURL = &routerURL
			channelSyncLock.Unlock()
			if !cached {
				DB.Model(&Channel{}).Where("id IN ?", []int{8, 9, 11, 12}).Update("base_url", xpikiURL)
				DB.Model(&Channel{}).Where("id = ?", 10).Update("base_url", routerURL)
			}

			// First attempt selects an XPiki channel (Priority 1)
			ch1, err := GetRandomSatisfiedChannel("default", retryTestModel, 0, nil)
			if err != nil || ch1 == nil || ch1.Id == 10 {
				t.Fatalf("first attempt expected xpiki, got %v", ch1)
			}

			// Second attempt (after 1 failure): with RELAY_MAX_HOST_ATTEMPTS=1, must immediately fallback to 9router (Channel 10)!
			ch2, err := GetRandomSatisfiedChannel("default", retryTestModel, 1, nil, ch1.Id)
			if err != nil || ch2 == nil || ch2.Id != 10 {
				t.Fatalf("second attempt expected immediate 9router fallback (id=10), got %v", ch2)
			}
		})
	}
}

func TestRetryCandidatesFallbackToAlternativeHostAfterTwoFailures(t *testing.T) {
	t.Setenv("RELAY_MAX_HOST_ATTEMPTS", "2")
	for _, cached := range []bool{true, false} {
		t.Run(map[bool]string{true: "cache", false: "database"}[cached], func(t *testing.T) {
			setupRetryCandidates(t, cached)
			xpikiURL := "https://api.xpiki.com"
			routerURL := "https://9router.trnhieu.dev"
			channelSyncLock.Lock()
			for _, id := range []int{8, 9, 11, 12} {
				channelsIDM[id].BaseURL = &xpikiURL
			}
			channelsIDM[10].BaseURL = &routerURL
			channelSyncLock.Unlock()
			if !cached {
				DB.Model(&Channel{}).Where("id IN ?", []int{8, 9, 11, 12}).Update("base_url", xpikiURL)
				DB.Model(&Channel{}).Where("id = ?", 10).Update("base_url", routerURL)
			}

			// First attempt selects an XPiki channel (Priority 1)
			ch1, err := GetRandomSatisfiedChannel("default", retryTestModel, 0, nil)
			if err != nil || ch1 == nil || ch1.Id == 10 {
				t.Fatalf("first attempt expected xpiki, got %v", ch1)
			}

			// Second attempt selects another XPiki channel (Priority 1)
			ch2, err := GetRandomSatisfiedChannel("default", retryTestModel, 1, nil, ch1.Id)
			if err != nil || ch2 == nil || ch2.Id == 10 || ch2.Id == ch1.Id {
				t.Fatalf("second attempt expected distinct xpiki, got %v", ch2)
			}

			// Third attempt: 2 XPiki channels have failed.
			// Must immediately fallback to 9router (Channel 10) instead of trying remaining XPiki channels!
			ch3, err := GetRandomSatisfiedChannel("default", retryTestModel, 2, nil, ch1.Id, ch2.Id)
			if err != nil || ch3 == nil || ch3.Id != 10 {
				t.Fatalf("third attempt expected immediate 9router fallback (id=10), got %v err=%v", ch3, err)
			}
		})
	}
}
