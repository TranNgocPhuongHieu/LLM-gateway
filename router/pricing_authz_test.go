package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRouterTestDB(t *testing.T) {
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

	if err := db.AutoMigrate(&model.User{}, &model.Option{}, &model.AuditLog{}, &model.Token{}, &model.Ability{}); err != nil {
		t.Fatal(err)
	}

	strPtr := func(s string) *string { return &s }

	// Create test users
	users := []model.User{
		{Id: 101, Username: "common_user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AccessToken: strPtr("pat_user_101"), AffCode: "aff_101", AuthVersion: 1},
		{Id: 102, Username: "admin_user", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AccessToken: strPtr("pat_admin_102"), AffCode: "aff_102", AuthVersion: 1},
		{Id: 103, Username: "root_user", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AccessToken: strPtr("pat_root_103"), AffCode: "aff_103", AuthVersion: 1},
	}
	for _, u := range users {
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestPriceManagementRouterAuthz(t *testing.T) {
	setupRouterTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	endpoints := []struct {
		name   string
		method string
		path   string
		body   func() []byte
	}{
		{
			name:   "GET model_pricing",
			method: http.MethodGet,
			path:   "/api/option/model_pricing",
			body:   func() []byte { return nil },
		},
		{
			name:   "GET model_pricing channels scope",
			method: http.MethodGet,
			path:   "/api/option/model_pricing/channels",
			body:   func() []byte { return nil },
		},
		{
			name:   "PATCH model_pricing",
			method: http.MethodPatch,
			path:   "/api/option/model_pricing",
			body: func() []byte {
				snap, _ := model.GetModelPricingSnapshot([]string{"claude-opus-5"})
				version := ""
				if len(snap.Entries) > 0 {
					version = snap.Entries[0].Version
				}
				b, _ := json.Marshal(gin.H{
					"changes": []gin.H{
						{
							"model_name":       "claude-opus-5",
							"expected_version": version,
							"pricing": gin.H{
								"billing_setting.billing_mode": "tiered_expr",
								"billing_setting.billing_expr": `tier("base", p * 0.05769230769230769 + c * 0.28846153846153846)`,
							},
						},
					},
				})
				return b
			},
		},
		{
			name:   "GET import_pricing",
			method: http.MethodGet,
			path:   "/api/option/import_pricing",
			body:   func() []byte { return nil },
		},
		{
			name:   "PUT import_pricing",
			method: http.MethodPut,
			path:   "/api/option/import_pricing",
			body: func() []byte {
				b, _ := json.Marshal(gin.H{"items": []gin.H{}})
				return b
			},
		},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			// 1. Unauthenticated -> 401
			reqUnauth := httptest.NewRequest(ep.method, ep.path, bytes.NewReader(ep.body()))
			reqUnauth.Header.Set("Content-Type", "application/json")
			recUnauth := httptest.NewRecorder()
			engine.ServeHTTP(recUnauth, reqUnauth)
			if recUnauth.Code != http.StatusUnauthorized {
				t.Errorf("unauthenticated request to %s: expected 401, got %d (body: %s)", ep.path, recUnauth.Code, recUnauth.Body.String())
			}

			// 2. Common User (Role 1) -> 403
			reqUser := httptest.NewRequest(ep.method, ep.path, bytes.NewReader(ep.body()))
			reqUser.Header.Set("Authorization", "Bearer pat_user_101")
			reqUser.Header.Set("Content-Type", "application/json")
			recUser := httptest.NewRecorder()
			engine.ServeHTTP(recUser, reqUser)
			if recUser.Code != http.StatusForbidden {
				t.Errorf("role 1 request to %s: expected 403, got %d (body: %s)", ep.path, recUser.Code, recUser.Body.String())
			}

			// 3. Admin User (Role 10) -> 403
			reqAdmin := httptest.NewRequest(ep.method, ep.path, bytes.NewReader(ep.body()))
			reqAdmin.Header.Set("Authorization", "Bearer pat_admin_102")
			reqAdmin.Header.Set("Content-Type", "application/json")
			recAdmin := httptest.NewRecorder()
			engine.ServeHTTP(recAdmin, reqAdmin)
			if recAdmin.Code != http.StatusForbidden {
				t.Errorf("role 10 request to %s: expected 403, got %d (body: %s)", ep.path, recAdmin.Code, recAdmin.Body.String())
			}

			// 4. Root User (Role 100) -> 200 OK
			reqRoot := httptest.NewRequest(ep.method, ep.path, bytes.NewReader(ep.body()))
			reqRoot.Header.Set("Authorization", "Bearer pat_root_103")
			reqRoot.Header.Set("Content-Type", "application/json")
			recRoot := httptest.NewRecorder()
			engine.ServeHTTP(recRoot, reqRoot)
			if recRoot.Code != http.StatusOK {
				t.Errorf("role 100 request to %s: expected 200 OK, got %d (body: %s)", ep.path, recRoot.Code, recRoot.Body.String())
			}
		})
	}
}
