package controller

import (
	"errors"
	"net/http"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/gin-gonic/gin"
)

func GetDeclaredImportPricing(c *gin.Context) {
	snapshot, err := model.GetDeclaredImportPricingSnapshot()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, snapshot)
}

func UpdateDeclaredImportPricing(c *gin.Context) {
	var request struct {
		Items           []model.DeclaredImportPriceItem `json:"items"`
		ExpectedVersion string                          `json:"expected_version"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	previousItems, err := model.UpdateDeclaredImportPricing(request.Items, request.ExpectedVersion)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrImportPricingConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	now := time.Now().Unix()
	recordManageAudit(c, "import.pricing.update", map[string]any{
		"count":        len(request.Items),
		"before":       previousItems,
		"after":        request.Items,
		"effective_at": now,
	})
	common.ApiSuccess(c, gin.H{
		"items":   request.Items,
		"version": model.HashImportPricing(request.Items),
	})
}

