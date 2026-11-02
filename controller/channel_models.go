package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	relaychannel "github.com/TranNgocPhuongHieu/LLM-gateway/relay/channel"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/service"

	"github.com/gin-gonic/gin"
)

type OpenAIModel struct {
	ID         string         `json:"id"`
	Object     string         `json:"object"`
	Created    int64          `json:"created"`
	OwnedBy    string         `json:"owned_by"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Permission []struct {
		ID                 string `json:"id"`
		Object             string `json:"object"`
		Created            int64  `json:"created"`
		AllowCreateEngine  bool   `json:"allow_create_engine"`
		AllowSampling      bool   `json:"allow_sampling"`
		AllowLogprobs      bool   `json:"allow_logprobs"`
		AllowSearchIndices bool   `json:"allow_search_indices"`
		AllowView          bool   `json:"allow_view"`
		AllowFineTuning    bool   `json:"allow_fine_tuning"`
		Organization       string `json:"organization"`
		Group              string `json:"group"`
		IsBlocking         bool   `json:"is_blocking"`
	} `json:"permission"`
	Root   string `json:"root"`
	Parent string `json:"parent"`
}

type OpenAIModelsResponse struct {
	Data    []OpenAIModel `json:"data"`
	Success bool          `json:"success"`
}

func buildFetchModelsHeaders(channel *model.Channel, key string) (http.Header, error) {
	var headers http.Header
	if channel.Type == constant.ChannelTypeAnthropic {
		if service.IsKiroBaseURL(channel.GetBaseURL()) {
			headers = GetAuthHeader(key)
		} else {
			headers = GetClaudeAuthHeader(key)
		}
	} else if channel.Type == constant.ChannelTypeOpenAI {
		headers = GetAuthHeader(key)
	} else {
		return nil, fmt.Errorf("unsupported upstream channel type")
	}

	if err := applyFetchModelsHeaderOverrides(channel, key, headers); err != nil {
		return nil, err
	}
	return headers, nil
}

func applyFetchModelsHeaderOverrides(channel *model.Channel, key string, headers http.Header) error {
	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:          key,
			HeadersOverride: channel.GetHeaderOverride(),
		},
	}
	overrides, err := relaychannel.ResolveHeaderOverride(info, nil)
	if err != nil {
		return err
	}
	for name, value := range overrides {
		headers.Set(name, value)
	}
	return nil
}

func FetchUpstreamModels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ids, err := fetchChannelUpstreamModelIDs(channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": fmt.Sprintf("获取模型列表失败: %s", err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": ids})
}

func FixChannelsAbilities(c *gin.Context) {
	success, fails, err := model.FixAbility()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"success": success, "fails": fails}})
}

type fetchModelsRequest struct {
	ChannelID      int     `json:"channel_id"`
	BaseURL        *string `json:"base_url"`
	Type           int     `json:"type"`
	Key            string  `json:"key"`
	HeaderOverride *string `json:"header_override"`
	Proxy          *string `json:"proxy"`
}

func FetchModels(c *gin.Context) {
	var req fetchModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	if (req.Type != constant.ChannelTypeOpenAI && req.Type != constant.ChannelTypeAnthropic) || req.ChannelID > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "use a supported protocol preview or the saved-channel models endpoint"})
		return
	}
	baseURL := constant.GetChannelBaseURL(req.Type)
	if req.BaseURL != nil && strings.TrimSpace(*req.BaseURL) != "" {
		baseURL = strings.TrimSpace(*req.BaseURL)
	}
	key := strings.Split(strings.TrimSpace(req.Key), "\n")[0]
	channel := &model.Channel{Type: req.Type, Key: key, BaseURL: &baseURL}
	if req.HeaderOverride != nil {
		channel.HeaderOverride = req.HeaderOverride
	}
	if req.Proxy != nil {
		settings := channel.GetSetting()
		settings.Proxy = strings.TrimSpace(*req.Proxy)
		channel.SetSetting(settings)
	}
	if err := validateChannel(channel, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	models, err := fetchChannelUpstreamModelIDs(channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": fmt.Sprintf("获取模型列表失败: %s", err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": models})
}

func GetTagModels(c *gin.Context) {
	tag := c.Query("tag")
	if tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "tag不能为空"})
		return
	}
	channels, err := model.GetChannelsByTag(tag, false, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	var longestModels string
	maxLength := 0
	for _, channel := range channels {
		if channel.Models != "" {
			currentModels := strings.Split(channel.Models, ",")
			if len(currentModels) > maxLength {
				maxLength = len(currentModels)
				longestModels = channel.Models
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": longestModels})
}
