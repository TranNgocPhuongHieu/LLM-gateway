package common_handler

import (
	"io"
	"net/http"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/logger"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/dto"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
	"github.com/TranNgocPhuongHieu/LLM-gateway/service"

	"github.com/gin-gonic/gin"
)

func RerankHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	logger.LogDebug(c, "reranker response body: %s", responseBody)
	var jinaResp dto.RerankResponse
	err = common.Unmarshal(responseBody, &jinaResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	jinaResp.Usage.PromptTokens = jinaResp.Usage.TotalTokens

	c.Writer.Header().Set("Content-Type", "application/json")
	c.JSON(http.StatusOK, jinaResp)
	return &jinaResp.Usage, nil
}
