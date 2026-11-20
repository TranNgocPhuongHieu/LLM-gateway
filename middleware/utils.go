package middleware

import (
	"errors"
	"fmt"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	"github.com/TranNgocPhuongHieu/LLM-gateway/logger"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
	"github.com/TranNgocPhuongHieu/LLM-gateway/service"
	"github.com/gin-gonic/gin"
)

// RecordEarlyRelayError records authenticated boundary/selection failures,
// including a failed selection after an earlier retryable attempt.
func RecordEarlyRelayError(c *gin.Context, message string, isStream bool, relayErrors ...*types.NewAPIError) {
	userId := c.GetInt("id")
	if userId > 0 && constant.ErrorLogEnabled {
		tokenId := c.GetInt("token_id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		if modelName == "" {
			modelName = c.GetString("model")
		}
		userGroup := c.GetString("group")
		if userGroup == "" {
			userGroup = c.GetString("token_group")
		}
		if userGroup == "" {
			userGroup = "default"
		}
		other := model.NewLogOther()
		other.SetPublic("request_final", true)
		other.SetPublic("failure_origin", "gateway")
		other.SetPublic("request_cancelled", c.Request.Context().Err() != nil)
		if len(relayErrors) > 0 && relayErrors[0] != nil {
			other.SetPublic("status_code", relayErrors[0].StatusCode)
			other.SetPublic("error_code", relayErrors[0].GetErrorCode())
			// Exhausted selection can return the last upstream error rather than
			// a new gateway error. Preserve its provenance in the terminal row.
			if relayErrors[0].UpstreamStatusCode > 0 {
				other.SetPublic("failure_origin", "upstream")
				other.SetPublic("upstream_status_code", relayErrors[0].UpstreamStatusCode)
			} else if relayErrors[0].GetErrorType() != types.ErrorTypeNewAPIError {
				other.SetPublic("failure_origin", "unknown")
			}
		}
		if c.Request != nil && c.Request.URL != nil {
			other.SetAdmin("request_path", c.Request.URL.Path)
		}
		channelID := 0
		if usedChannels := c.GetStringSlice("use_channel"); len(usedChannels) > 0 {
			other.SetAdmin("use_channel", usedChannels)
			channelID = c.GetInt("channel_id")
		}
		model.RecordErrorLog(c, userId, channelID, modelName, tokenName, message, tokenId, 0, isStream, userGroup, other)
	}
}

func abortWithOpenAiMessage(c *gin.Context, statusCode int, message string, code ...types.ErrorCode) {
	codeStr := ""
	if len(code) > 0 {
		codeStr = string(code[0])
	}
	RecordEarlyRelayError(c, message, common.GetContextKeyBool(c, constant.ContextKeyIsStream))
	userId := c.GetInt("id")
	writeOpenAiError(c, statusCode, message, codeStr)
	logger.LogError(c.Request.Context(), fmt.Sprintf("user %d | %s", userId, message))
}

// abortWithClientRejection answers a failure that retrying cannot fix, such
// as a request for a model no channel offers. Clients often retry these in a
// loop, so unlike abortWithOpenAiMessage it logs a warning and records an
// error row at most once per user, code and model per window.
func abortWithClientRejection(c *gin.Context, statusCode int, message string, code types.ErrorCode, modelName string) {
	userId := c.GetInt("id")
	if ok, suppressed := service.AllowClientRejectionLog(userId, code, modelName); ok {
		RecordEarlyRelayError(c, message, common.GetContextKeyBool(c, constant.ContextKeyIsStream),
			types.NewErrorWithStatusCode(errors.New(message), code, statusCode))
		logger.LogWarn(c.Request.Context(), "user %d | %s (%d similar suppressed)", userId, message, suppressed)
	}
	writeOpenAiError(c, statusCode, message, string(code))
}

func writeOpenAiError(c *gin.Context, statusCode int, message string, code string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
			"type":    "new_api_error", "code": code,
		},
	})
	c.Abort()
}
