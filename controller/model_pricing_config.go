package controller

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetModelPricingConfig(c *gin.Context) {
	snapshot, err := model.GetModelPricingSnapshot(c.QueryArray("model"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, snapshot)
}

func GetModelPricingChannelScope(c *gin.Context) {
	scope, err := model.GetConfiguredChannelPricingScope()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, scope)
}

func PreviewModelPricingConversion(c *gin.Context) {
	var request struct {
		ModelName string              `json:"model_name"`
		Pricing   model.PricingValues `json:"pricing"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	preview, err := model.PreviewModelPricingConversion(request.ModelName, request.Pricing)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, preview)
}

func PreviewModelPricing(c *gin.Context) {
	var request struct {
		ModelName string              `json:"model_name"`
		Pricing   model.PricingValues `json:"pricing"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	preview, err := model.PreviewModelPricing(request.ModelName, request.Pricing)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, model.ModelPricingDescription{
		Effective:      preview,
		CacheWriteMode: model.ResolveCacheWriteMode(request.ModelName, request.Pricing),
		BillingDetails: model.ResolveLegacyBillingDetails(request.ModelName, preview, request.Pricing),
	})
}

func UpdateModelPricingConfig(c *gin.Context) {
	var request struct {
		Changes []model.ModelPricingChange `json:"changes"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	// Backend scope validation: ensure every changed model is configured in at least one channel
	scope, err := model.GetConfiguredChannelPricingScope()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	allowedModels := make(map[string]bool, len(scope.UniqueModels))
	for _, m := range scope.UniqueModels {
		allowedModels[m] = true
	}
	for _, change := range request.Changes {
		if !allowedModels[change.ModelName] {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Model '" + change.ModelName + "' is not configured in any channel",
			})
			return
		}
	}

	auditDetails, err := model.UpdateModelPricingWithAudit(request.Changes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrModelPricingConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	names := make([]string, 0, len(request.Changes))
	for _, change := range request.Changes {
		names = append(names, change.ModelName)
	}
	now := time.Now().Unix()
	recordManageAudit(c, "model.pricing.update", map[string]any{
		"models":       strings.Join(names, ", "),
		"model_list":   names,
		"changes":      auditDetails,
		"effective_at": now,
	})
	common.ApiSuccess(c, gin.H{"updated_models": names})
}
