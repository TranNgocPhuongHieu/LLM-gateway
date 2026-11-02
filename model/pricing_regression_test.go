package model

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	oldDB := DB
	oldKeyCol := commonKeyCol
	oldGroupCol := commonGroupCol
	oldOptionMap := common.OptionMap
	commonKeyCol = "`key`"
	commonGroupCol = "`group`"
	common.OptionMap = make(map[string]string)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Option{}); err != nil {
		t.Fatal(err)
	}
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		commonKeyCol = oldKeyCol
		commonGroupCol = oldGroupCol
		common.OptionMap = oldOptionMap
	})
}

func TestUpdateOneModelDoesNotTouchOthers(t *testing.T) {
	setupTestDB(t)

	// Initialize options with multiple models
	initBillingExpr := map[string]string{
		"claude-opus-5": `tier("base", p * 0.05769230769230769 + c * 0.28846153846153846 + cr * 0.028846153846153848 + cc * 0.013)`,
		"gpt-5.5":       `tier("base", p * 0.02 + c * 0.09 + cr * 0.01)`,
		"gpt-6-sol":     `tier("base", p * 0.008 + c * 0.03 + cr * 0.004 + cc * 0.008)`,
	}
	encoded, _ := json.Marshal(initBillingExpr)
	if err := DB.Create(&Option{Key: "billing_setting.billing_expr", Value: string(encoded)}).Error; err != nil {
		t.Fatal(err)
	}
	modeMap := map[string]string{
		"claude-opus-5": "tiered_expr",
		"gpt-5.5":       "tiered_expr",
		"gpt-6-sol":     "tiered_expr",
	}
	encodedMode, _ := json.Marshal(modeMap)
	if err := DB.Create(&Option{Key: "billing_setting.billing_mode", Value: string(encodedMode)}).Error; err != nil {
		t.Fatal(err)
	}

	snapshot, err := GetModelPricingSnapshot([]string{"claude-opus-5", "gpt-5.5", "gpt-6-sol"})
	if err != nil {
		t.Fatal(err)
	}

	var opusEntry *ModelPricingEntry
	for i := range snapshot.Entries {
		if snapshot.Entries[i].ModelName == "claude-opus-5" {
			opusEntry = &snapshot.Entries[i]
		}
	}
	if opusEntry == nil {
		t.Fatal("opus entry not found")
	}

	// Update ONLY claude-opus-5
	newOpusExpr := `tier("base", p * 0.06 + c * 0.30 + cr * 0.03 + cc * 0.013)`
	change := ModelPricingChange{
		ModelName:       "claude-opus-5",
		ExpectedVersion: opusEntry.Version,
		Pricing: PricingValues{
			"billing_setting.billing_mode": "tiered_expr",
			"billing_setting.billing_expr": newOpusExpr,
		},
	}

	auditDetails, err := UpdateModelPricingWithAudit([]ModelPricingChange{change})
	if err != nil {
		t.Fatalf("UpdateModelPricingWithAudit failed: %v", err)
	}
	if len(auditDetails) != 1 || auditDetails[0].ModelName != "claude-opus-5" {
		t.Fatalf("unexpected audit details: %+v", auditDetails)
	}

	// Verify gpt-5.5 and gpt-6-sol are 100% untouched in DB
	newSnapshot, err := GetModelPricingSnapshot([]string{"claude-opus-5", "gpt-5.5", "gpt-6-sol"})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range newSnapshot.Entries {
		switch entry.ModelName {
		case "claude-opus-5":
			if entry.Configured["billing_setting.billing_expr"] != newOpusExpr {
				t.Fatalf("opus expression not updated: %v", entry.Configured["billing_setting.billing_expr"])
			}
		case "gpt-5.5":
			if entry.Configured["billing_setting.billing_expr"] != `tier("base", p * 0.02 + c * 0.09 + cr * 0.01)` {
				t.Fatalf("gpt-5.5 was clobbered! got: %v", entry.Configured["billing_setting.billing_expr"])
			}
		case "gpt-6-sol":
			if entry.Configured["billing_setting.billing_expr"] != `tier("base", p * 0.008 + c * 0.03 + cr * 0.004 + cc * 0.008)` {
				t.Fatalf("gpt-6-sol was clobbered! got: %v", entry.Configured["billing_setting.billing_expr"])
			}
		}
	}
}

func TestPreserveAstraTwoTiers(t *testing.T) {
	setupTestDB(t)

	astraMultiTier := `tier("base", p * 0.04 + c * 0.15 + cr * 0.02 + cc * 0.04), tier("tier2", p * 0.02 + c * 0.08 + cr * 0.01)`
	initBillingExpr := map[string]string{
		"claude-opus-5": `tier("base", p * 0.05769230769230769 + c * 0.28846153846153846 + cr * 0.028846153846153848 + cc * 0.013)`,
		"gpt-6-astra":   astraMultiTier,
	}
	encoded, _ := json.Marshal(initBillingExpr)
	if err := DB.Create(&Option{Key: "billing_setting.billing_expr", Value: string(encoded)}).Error; err != nil {
		t.Fatal(err)
	}
	modeMap := map[string]string{
		"claude-opus-5": "tiered_expr",
		"gpt-6-astra":   "tiered_expr",
	}
	encodedMode, _ := json.Marshal(modeMap)
	if err := DB.Create(&Option{Key: "billing_setting.billing_mode", Value: string(encodedMode)}).Error; err != nil {
		t.Fatal(err)
	}

	snapshot, err := GetModelPricingSnapshot([]string{"claude-opus-5", "gpt-6-astra"})
	if err != nil {
		t.Fatal(err)
	}

	var opusEntry *ModelPricingEntry
	for i := range snapshot.Entries {
		if snapshot.Entries[i].ModelName == "claude-opus-5" {
			opusEntry = &snapshot.Entries[i]
		}
	}

	// Update ONLY claude-opus-5
	change := ModelPricingChange{
		ModelName:       "claude-opus-5",
		ExpectedVersion: opusEntry.Version,
		Pricing: PricingValues{
			"billing_setting.billing_mode": "tiered_expr",
			"billing_setting.billing_expr": `tier("base", p * 0.05769230769230769 + c * 0.28846153846153846 + cr * 0.028846153846153848 + cc * 0.013)`,
		},
	}

	if _, err := UpdateModelPricingWithAudit([]ModelPricingChange{change}); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify gpt-6-astra in DB still has both tiers and cache-write intact
	var row Option
	if err := DB.Where("key = ?", "billing_setting.billing_expr").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	var storedExprs map[string]string
	_ = json.Unmarshal([]byte(row.Value), &storedExprs)
	if storedExprs["gpt-6-astra"] != astraMultiTier {
		t.Fatalf("Astra multi-tier expression was altered! Got: %q, expected: %q", storedExprs["gpt-6-astra"], astraMultiTier)
	}
}

func TestStaleSaveConflict(t *testing.T) {
	setupTestDB(t)

	initBillingExpr := map[string]string{
		"claude-opus-5": `tier("base", p * 0.05769230769230769 + c * 0.28846153846153846 + cr * 0.028846153846153848 + cc * 0.013)`,
	}
	encoded, _ := json.Marshal(initBillingExpr)
	_ = DB.Create(&Option{Key: "billing_setting.billing_expr", Value: string(encoded)})
	modeMap := map[string]string{"claude-opus-5": "tiered_expr"}
	encodedMode, _ := json.Marshal(modeMap)
	_ = DB.Create(&Option{Key: "billing_setting.billing_mode", Value: string(encodedMode)})

	// Client 1 loads snapshot
	snapshot1, err := GetModelPricingSnapshot([]string{"claude-opus-5"})
	if err != nil {
		t.Fatal(err)
	}
	staleVersion := snapshot1.Entries[0].Version

	// Client 2 performs an update
	newExpr := `tier("base", p * 0.06 + c * 0.30 + cr * 0.03 + cc * 0.013)`
	_, err = UpdateModelPricingWithAudit([]ModelPricingChange{
		{
			ModelName:       "claude-opus-5",
			ExpectedVersion: staleVersion,
			Pricing: PricingValues{
				"billing_setting.billing_mode": "tiered_expr",
				"billing_setting.billing_expr": newExpr,
			},
		},
	})
	if err != nil {
		t.Fatalf("Client 2 update failed: %v", err)
	}

	// Client 1 attempts to save with stale version
	_, err = UpdateModelPricingWithAudit([]ModelPricingChange{
		{
			ModelName:       "claude-opus-5",
			ExpectedVersion: staleVersion,
			Pricing: PricingValues{
				"billing_setting.billing_mode": "tiered_expr",
				"billing_setting.billing_expr": `tier("base", p * 0.07 + c * 0.35 + cr * 0.035 + cc * 0.013)`,
			},
		},
	})
	if !errors.Is(err, ErrModelPricingConflict) {
		t.Fatalf("expected ErrModelPricingConflict, got: %v", err)
	}

	// Test Import Pricing conflict
	items := []DeclaredImportPriceItem{
		{Id: "1", Provider: "provider_a", Model: "gpt-6.1-sol", InputVnd: 500, OutputVnd: 2500, CacheReadVnd: 250},
	}
	v1, err := UpdateDeclaredImportPricing(items, "")
	_ = v1
	snap, _ := GetDeclaredImportPricingSnapshot()

	// Another update changes version
	items2 := append(items, DeclaredImportPriceItem{Id: "2", Provider: "provider_b", Model: "opus", InputVnd: 1000, OutputVnd: 5000, CacheReadVnd: 500})
	_, err = UpdateDeclaredImportPricing(items2, snap.Version)
	if err != nil {
		t.Fatalf("update declared import pricing failed: %v", err)
	}

	// Stale save with snap.Version must fail
	_, err = UpdateDeclaredImportPricing(items, snap.Version)
	if !errors.Is(err, ErrImportPricingConflict) {
		t.Fatalf("expected ErrImportPricingConflict on stale save, got: %v", err)
	}
}

func TestPricingAuditHistory(t *testing.T) {
	setupTestDB(t)

	initBillingExpr := map[string]string{
		"claude-opus-5": `tier("base", p * 0.05769230769230769 + c * 0.28846153846153846 + cr * 0.028846153846153848 + cc * 0.013)`,
	}
	encoded, _ := json.Marshal(initBillingExpr)
	_ = DB.Create(&Option{Key: "billing_setting.billing_expr", Value: string(encoded)})
	modeMap := map[string]string{"claude-opus-5": "tiered_expr"}
	encodedMode, _ := json.Marshal(modeMap)
	_ = DB.Create(&Option{Key: "billing_setting.billing_mode", Value: string(encodedMode)})

	snapshot, _ := GetModelPricingSnapshot([]string{"claude-opus-5"})
	newExpr := `tier("base", p * 0.06 + c * 0.30 + cr * 0.03 + cc * 0.013)`
	auditDetails, err := UpdateModelPricingWithAudit([]ModelPricingChange{
		{
			ModelName:       "claude-opus-5",
			ExpectedVersion: snapshot.Entries[0].Version,
			Pricing: PricingValues{
				"billing_setting.billing_mode": "tiered_expr",
				"billing_setting.billing_expr": newExpr,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(auditDetails) != 1 {
		t.Fatalf("expected 1 audit detail, got %d", len(auditDetails))
	}
	d := auditDetails[0]
	if d.ModelName != "claude-opus-5" {
		t.Errorf("expected model claude-opus-5, got %s", d.ModelName)
	}
	if d.Before["billing_setting.billing_expr"] != initBillingExpr["claude-opus-5"] {
		t.Errorf("before mismatch: %v", d.Before["billing_setting.billing_expr"])
	}
	if d.After["billing_setting.billing_expr"] != newExpr {
		t.Errorf("after mismatch: %v", d.After["billing_setting.billing_expr"])
	}
	if d.EffectiveAt <= 0 {
		t.Errorf("expected valid effective_at timestamp, got %d", d.EffectiveAt)
	}
}

func TestUserSafeErrorProjection(t *testing.T) {
	// 1. Internal error containing channel/provider diagnostics must be masked
	internalRaw := "status_code=500, upstream error (channel #10 '9router'): provider_error: connection refused (request id: req-int-123)"
	projected := ProjectUserSafeErrorContent(internalRaw, "req-int-123")
	if strings.Contains(projected, "channel #10") || strings.Contains(projected, "9router") || strings.Contains(projected, "provider_error") || strings.Contains(projected, "upstream") {
		t.Fatalf("internal diagnostics leaked to user: %q", projected)
	}
	if !strings.Contains(projected, "req-int-123") {
		t.Fatalf("request ID missing from projected error: %q", projected)
	}

	// 2. Client-actionable error must be preserved with request ID
	clientRaw := "status_code=403, insufficient_user_quota: User quota is not enough (request id: req-cli-456)"
	projectedClient := ProjectUserSafeErrorContent(clientRaw, "req-cli-456")
	if !strings.Contains(projectedClient, "insufficient_user_quota") && !strings.Contains(projectedClient, "quota is not enough") {
		t.Fatalf("client-actionable error was over-masked: %q", projectedClient)
	}
	if !strings.Contains(projectedClient, "req-cli-456") {
		t.Fatalf("request ID missing: %q", projectedClient)
	}

	// 3. Incidental channel reference in client error must be stripped
	clientWithChannel := "channel #5 'openai': model 'unknown-model' not found (request id: req-789)"
	projectedCleaned := ProjectUserSafeErrorContent(clientWithChannel, "req-789")
	if strings.Contains(projectedCleaned, "channel #5") || strings.Contains(projectedCleaned, "'openai'") {
		t.Fatalf("channel reference leaked in client error: %q", projectedCleaned)
	}
	if !strings.Contains(projectedCleaned, "unknown-model") {
		t.Fatalf("actionable message lost: %q", projectedCleaned)
	}

	// 4. Stream error projection
	streamRaw := "channel #10 '9router' failed with provider upstream error: timeout"
	safeStream := ProjectUserSafeStreamError(streamRaw)
	if strings.Contains(safeStream, "channel") || strings.Contains(safeStream, "9router") || strings.Contains(safeStream, "upstream") {
		t.Fatalf("stream error leaked diagnostics: %q", safeStream)
	}
	if safeStream != "stream interrupted" {
		t.Fatalf("expected 'stream interrupted', got %q", safeStream)
	}

	if ProjectUserSafeStreamError("client_gone") != "cancelled" {
		t.Fatalf("expected 'cancelled', got %q", ProjectUserSafeStreamError("client_gone"))
	}
	if ProjectUserSafeStreamError("event:response.incomplete") != "stream incomplete" {
		t.Fatalf("expected 'stream incomplete', got %q", ProjectUserSafeStreamError("event:response.incomplete"))
	}

	// 5. Relay error message projection
	relayErr500 := types.ProjectUserSafeRelayMessage("channel #10 failed: upstream timeout (request id: req-relay-999)", 500, types.ErrorCodeDoRequestFailed)
	if strings.Contains(relayErr500, "channel #10") || strings.Contains(relayErr500, "upstream") {
		t.Fatalf("relay error 500 leaked diagnostics: %q", relayErr500)
	}
	if !strings.Contains(relayErr500, "req-relay-999") {
		t.Fatalf("relay error missing request id: %q", relayErr500)
	}

	relayErr400 := types.ProjectUserSafeRelayMessage("channel #3: invalid_request: prompt cannot be empty (request id: req-relay-400)", 400, types.ErrorCodeInvalidRequest)
	if strings.Contains(relayErr400, "channel #3") {
		t.Fatalf("relay error 400 leaked channel: %q", relayErr400)
	}
	if !strings.Contains(relayErr400, "prompt cannot be empty") || !strings.Contains(relayErr400, "req-relay-400") {
		t.Fatalf("relay error 400 lost actionable message or req id: %q", relayErr400)
	}
}
