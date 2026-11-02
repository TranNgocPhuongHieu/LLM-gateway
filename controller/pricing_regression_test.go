package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupControllerTestDB(t *testing.T) {
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
	oldDB, oldLogs, oldRedis, oldMemory := model.DB, model.LOG_DB, common.RedisEnabled, common.MemoryCacheEnabled
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	model.DB, model.LOG_DB, common.RedisEnabled, common.MemoryCacheEnabled = db, db, false, false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.InitCol()
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.MemoryCacheEnabled = oldDB, oldLogs, oldRedis, oldMemory
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.OptionMap = oldOptionMap
		_ = conn.Close()
	})
	if err := db.AutoMigrate(&model.Option{}, &model.AuditLog{}, &model.User{}, &model.Channel{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{Id: 1, Username: "Trnhieu", Role: 100, Status: 1}
	_ = db.Create(&user)
	ch := model.Channel{Id: 1, Name: "TestChannel", Status: 1, Models: "claude-opus-5"}
	_ = db.Create(&ch)
}

func TestControllerModelPricingSaveConflictAndAudit(t *testing.T) {
	setupControllerTestDB(t)

	initBillingExpr := map[string]string{
		"claude-opus-5": `tier("base", p * 0.05769230769230769 + c * 0.28846153846153846 + cr * 0.028846153846153848 + cc * 0.013)`,
	}
	encoded, _ := json.Marshal(initBillingExpr)
	_ = model.DB.Create(&model.Option{Key: "billing_setting.billing_expr", Value: string(encoded)})
	modeMap := map[string]string{"claude-opus-5": "tiered_expr"}
	encodedMode, _ := json.Marshal(modeMap)
	_ = model.DB.Create(&model.Option{Key: "billing_setting.billing_mode", Value: string(encodedMode)})

	snapshot, err := model.GetModelPricingSnapshot([]string{"claude-opus-5"})
	if err != nil {
		t.Fatal(err)
	}
	v1 := snapshot.Entries[0].Version

	gin.SetMode(gin.TestMode)

	// Test successful save
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", 1)
	c.Set("username", "Trnhieu")
	c.Set("role", 100)

	newExpr := `tier("base", p * 0.06 + c * 0.30 + cr * 0.03 + cc * 0.013)`
	reqBody, _ := json.Marshal(gin.H{
		"changes": []gin.H{
			{
				"model_name":       "claude-opus-5",
				"expected_version": v1,
				"pricing": gin.H{
					"billing_setting.billing_mode": "tiered_expr",
					"billing_setting.billing_expr": newExpr,
				},
			},
		},
	})
	c.Request = httptest.NewRequest(http.MethodPatch, "/api/option/model_pricing", bytes.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	UpdateModelPricingConfig(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// Verify audit log was recorded
	var audit model.AuditLog
	if err := model.DB.Where("action = ?", "model.pricing.update").First(&audit).Error; err != nil {
		t.Fatalf("model.pricing.update audit log not found: %v", err)
	}
	if audit.UserId != 1 || audit.Username != "Trnhieu" {
		t.Fatalf("unexpected audit operator: user_id=%d username=%s", audit.UserId, audit.Username)
	}

	// Stale save with old version v1 must return 409 Conflict
	wStale := httptest.NewRecorder()
	cStale, _ := gin.CreateTestContext(wStale)
	cStale.Set("id", 1)
	cStale.Set("username", "Trnhieu")
	cStale.Set("role", 100)
	cStale.Request = httptest.NewRequest(http.MethodPatch, "/api/option/model_pricing", bytes.NewReader(reqBody))
	cStale.Request.Header.Set("Content-Type", "application/json")

	UpdateModelPricingConfig(cStale)
	if wStale.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on stale save, got %d: %s", wStale.Code, wStale.Body.String())
	}
}

func TestControllerImportPricingConflictAndAudit(t *testing.T) {
	setupControllerTestDB(t)

	gin.SetMode(gin.TestMode)

	// Initial save of import pricing
	items := []model.DeclaredImportPriceItem{
		{Id: "1", Provider: "provider_a", Model: "gpt-6.1-sol", InputVnd: 500, OutputVnd: 2500, CacheReadVnd: 250},
	}
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Set("id", 1)
	c1.Set("username", "Trnhieu")
	c1.Set("role", 100)

	body1, _ := json.Marshal(gin.H{
		"items":            items,
		"expected_version": "",
	})
	c1.Request = httptest.NewRequest(http.MethodPut, "/api/option/import_pricing", bytes.NewReader(body1))
	c1.Request.Header.Set("Content-Type", "application/json")

	UpdateDeclaredImportPricing(c1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w1.Code, w1.Body.String())
	}

	var res1 struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w1.Body.Bytes(), &res1)
	v1 := res1.Data.Version

	// Verify audit log for import pricing
	var audit model.AuditLog
	if err := model.DB.Where("action = ?", "import.pricing.update").First(&audit).Error; err != nil {
		t.Fatalf("import.pricing.update audit not found: %v", err)
	}

	// Update items to version 2
	items2 := append(items, model.DeclaredImportPriceItem{Id: "2", Provider: "provider_b", Model: "opus", InputVnd: 1000, OutputVnd: 5000, CacheReadVnd: 500})
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Set("id", 1)
	c2.Set("username", "Trnhieu")
	c2.Set("role", 100)
	body2, _ := json.Marshal(gin.H{
		"items":            items2,
		"expected_version": v1,
	})
	c2.Request = httptest.NewRequest(http.MethodPut, "/api/option/import_pricing", bytes.NewReader(body2))
	c2.Request.Header.Set("Content-Type", "application/json")

	UpdateDeclaredImportPricing(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	// Now attempt stale save with old version v1 -> must return 409 Conflict
	bodyStale, _ := json.Marshal(gin.H{
		"items":            items,
		"expected_version": v1,
	})
	wStale := httptest.NewRecorder()
	cStale, _ := gin.CreateTestContext(wStale)
	cStale.Set("id", 1)
	cStale.Set("username", "Trnhieu")
	cStale.Set("role", 100)
	cStale.Request = httptest.NewRequest(http.MethodPut, "/api/option/import_pricing", bytes.NewReader(bodyStale))
	cStale.Request.Header.Set("Content-Type", "application/json")

	UpdateDeclaredImportPricing(cStale)
	if wStale.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on stale import save, got %d: %s", wStale.Code, wStale.Body.String())
	}
}

func TestControllerModelPricingScopeValidation(t *testing.T) {
	setupControllerTestDB(t)
	gin.SetMode(gin.TestMode)

	// Attempt to save pricing for a model NOT in any channel ("gpt-nonexistent")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", 1)
	c.Set("username", "Trnhieu")
	c.Set("role", 100)

	reqBody, _ := json.Marshal(gin.H{
		"changes": []gin.H{
			{
				"model_name":       "gpt-nonexistent",
				"expected_version": "",
				"pricing": gin.H{
					"billing_setting.billing_mode": "tiered_expr",
					"billing_setting.billing_expr": `tier("base", p * 0.05)`,
				},
			},
		},
	})
	c.Request = httptest.NewRequest(http.MethodPatch, "/api/option/model_pricing", bytes.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	UpdateModelPricingConfig(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for unconfigured model, got %d: %s", w.Code, w.Body.String())
	}

	// Verify channel scope retrieval
	wScope := httptest.NewRecorder()
	cScope, _ := gin.CreateTestContext(wScope)
	cScope.Set("id", 1)
	cScope.Set("username", "Trnhieu")
	cScope.Set("role", 100)
	cScope.Request = httptest.NewRequest(http.MethodGet, "/api/option/model_pricing/channels", nil)

	GetModelPricingChannelScope(cScope)
	if wScope.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for channel scope, got %d: %s", wScope.Code, wScope.Body.String())
	}
	var res gin.H
	_ = json.Unmarshal(wScope.Body.Bytes(), &res)
	if !res["success"].(bool) {
		t.Fatalf("expected success true in response: %s", wScope.Body.String())
	}
}
