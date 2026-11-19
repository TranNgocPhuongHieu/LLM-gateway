package controller

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCreateBinanceOrderQRPayloadIsBareAddress(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	general := operation_setting.GetGeneralSetting()
	oldDB, oldLog, oldRedis, oldType, oldDisplay := model.DB, model.LOG_DB, common.RedisEnabled, common.MainDatabaseType(), general.QuotaDisplayType
	model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	general.QuotaDisplayType = operation_setting.QuotaDisplayTypeCustom
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled = oldDB, oldLog, oldRedis
		common.SetDatabaseTypes(oldType, oldType)
		general.QuotaDisplayType = oldDisplay
		_ = conn.Close()
	})
	if err := db.AutoMigrate(&model.Option{}, &model.User{}, &model.TopUp{}, &model.BinanceTopUp{}); err != nil {
		t.Fatal(err)
	}
	const address = "0x00000000000000000000000000000000000000b1"
	cfg := model.DefaultBinanceConfig()
	cfg.Enabled, cfg.CheckoutEnabled, cfg.AddressesVerified = true, true, true
	cfg.FeePercent = "2"
	cfg.Addresses = map[string]string{model.BinanceNetworkBEP20: address}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Option{Key: model.BinanceConfigOption, Value: string(raw)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.User{Id: 1, Username: "usdt-fixture"}).Error; err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.POST("/orders", func(c *gin.Context) { c.Set("id", 1); CreateBinanceOrder(c) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/orders", strings.NewReader(`{"network":"BEP20","amount_usdt":"10","request_id":"qr-fixture"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	var body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Address    string `json:"address"`
			AmountUSDT string `json:"amount_usdt"`
			QRPayload  string `json:"qr_payload"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	if !body.Success {
		t.Fatalf("order rejected: %s", body.Message)
	}
	if body.Data.QRPayload != address || body.Data.Address != address {
		t.Fatalf("QR payload must be the bare deposit address: qr=%q address=%q", body.Data.QRPayload, body.Data.Address)
	}
	if body.Data.AmountUSDT != "10.200000" {
		t.Fatalf("payable amount changed: %q", body.Data.AmountUSDT)
	}
}
