package controller

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type CustomKeyItem struct {
	Label string `json:"label"`
	Key   string `json:"key"`
}

type CheckXPikiQuotaRequest struct {
	CustomKeys     []CustomKeyItem `json:"custom_keys"`
	IncludePricing *bool           `json:"include_pricing"`
}

type CheckKiroQuotaRequest struct {
	CustomKeys []CustomKeyItem `json:"custom_keys"`
}

type CheckXPikiLogsRequest struct {
	Source        string          `json:"source"`
	CustomKeys    []CustomKeyItem `json:"custom_keys"`
	KeyFilter     string          `json:"key_filter"`
	ChannelFilter string          `json:"channel_filter"`
	Search        string          `json:"search"`
	Model         string          `json:"model"`
	User          string          `json:"user"`
	Status        string          `json:"status"`
	Type          string          `json:"type"`
	Date          string          `json:"date"`
	StartDate     string          `json:"start_date"`
	EndDate       string          `json:"end_date"`
	Page          int             `json:"page"`
	PageSize      int             `json:"page_size"`
	GroupMode     string          `json:"group_mode"`
}

func buildXPikiCredentials(customKeys []CustomKeyItem, xpikiOnly bool) ([]service.XPikiQuotaCredential, error) {
	channels, err := model.GetAllChannels(0, 0, true, true)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(channels, func(a, b *model.Channel) int {
		return cmp.Compare(a.Id, b.Id)
	})

	credentials := make([]service.XPikiQuotaCredential, 0, len(channels))
	seenKeys := make(map[string]int)

	channelByKey := make(map[string]*model.Channel)
	for _, ch := range channels {
		for _, key := range ch.GetKeys() {
			k := strings.TrimSpace(key)
			if k != "" {
				channelByKey[k] = ch
			}
		}
	}

	for _, channel := range channels {
		if xpikiOnly && !service.IsXPikiBaseURL(channel.GetBaseURL()) {
			continue
		}
		for keyIdx, key := range channel.GetKeys() {
			k := strings.TrimSpace(key)
			if k == "" {
				continue
			}
			if idx, exists := seenKeys[k]; exists {
				if !slices.ContainsFunc(credentials[idx].SourceChannels, func(ch service.XPikiSourceChannel) bool { return ch.ID == channel.Id }) {
					credentials[idx].SourceChannels = append(credentials[idx].SourceChannels, service.XPikiSourceChannel{ID: channel.Id, Name: channel.Name})
				}
				continue
			}
			seenKeys[k] = len(credentials)
			label := channel.Name
			if label == "" {
				label = "Key " + strconv.Itoa(len(credentials)+1)
			} else if len(channel.GetKeys()) > 1 {
				label = fmt.Sprintf("%s (Key %d)", channel.Name, keyIdx+1)
			}
			credentials = append(credentials, service.XPikiQuotaCredential{
				Label:          label,
				APIKey:         k,
				ChannelId:      channel.Id,
				ChannelName:    channel.Name,
				ChannelStatus:  channel.Status,
				BaseURL:        channel.GetBaseURL(),
				SourceChannels: []service.XPikiSourceChannel{{ID: channel.Id, Name: channel.Name}},
			})
		}
	}

	for _, item := range customKeys {
		k := strings.TrimSpace(item.Key)
		_, seen := seenKeys[k]
		if k == "" || seen {
			continue
		}
		seenKeys[k] = len(credentials)
		label := strings.TrimSpace(item.Label)
		ch := channelByKey[k]
		chId := 0
		chName := ""
		chStatus := 0
		chBaseURL := ""
		if ch != nil {
			chId = ch.Id
			chName = ch.Name
			chStatus = ch.Status
			chBaseURL = ch.GetBaseURL()
			if label == "" {
				label = ch.Name
			}
		}
		if label == "" {
			label = "Key " + strconv.Itoa(len(credentials)+1)
		}
		credentials = append(credentials, service.XPikiQuotaCredential{
			Label:         label,
			APIKey:        k,
			ChannelId:     chId,
			ChannelName:   chName,
			ChannelStatus: chStatus,
			BaseURL:       chBaseURL,
		})
	}

	return credentials, nil
}

func CheckXPikiQuota(c *gin.Context) {
	var req CheckXPikiQuotaRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			common.ApiErrorMsg(c, "Invalid quota check request")
			return
		}
	}

	credentials, err := buildXPikiCredentials(req.CustomKeys, true)
	if err != nil {
		common.ApiErrorMsg(c, "Could not load channel configuration")
		return
	}

	includePricing := req.IncludePricing == nil || *req.IncludePricing
	common.ApiSuccess(c, service.CheckXPikiQuotasWithPricing(c.Request.Context(), credentials, includePricing))
}

func CheckKiroQuota(c *gin.Context) {
	var req CheckKiroQuotaRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			common.ApiErrorMsg(c, "Invalid Kiro quota check request")
			return
		}
	}
	channels, err := model.GetAllChannels(0, 0, true, true)
	if err != nil {
		common.ApiErrorMsg(c, "Could not load channel configuration")
		return
	}
	credentials := make([]service.KiroQuotaCredential, 0)
	seen := make(map[string]struct{})
	for _, channel := range channels {
		if !service.IsKiroBaseURL(channel.GetBaseURL()) {
			continue
		}
		for keyIndex, key := range channel.GetKeys() {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			label := channel.Name
			if label == "" {
				label = fmt.Sprintf("Kiro Key %d", len(credentials)+1)
			} else if len(channel.GetKeys()) > 1 {
				label = fmt.Sprintf("%s (Key %d)", label, keyIndex+1)
			}
			credentials = append(credentials, service.KiroQuotaCredential{Label: label, APIKey: key})
		}
	}
	for _, custom := range req.CustomKeys {
		key := strings.TrimSpace(custom.Key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		label := strings.TrimSpace(custom.Label)
		if label == "" {
			label = fmt.Sprintf("Kiro Key %d", len(credentials)+1)
		}
		credentials = append(credentials, service.KiroQuotaCredential{Label: label, APIKey: key})
	}
	common.ApiSuccess(c, service.CheckKiroQuotas(c.Request.Context(), credentials))
}

func CheckXPikiLogs(c *gin.Context) {
	var req CheckXPikiLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		common.ApiErrorMsg(c, "Invalid request log filters")
		return
	}
	req.GroupMode = strings.ToLower(strings.TrimSpace(req.GroupMode))
	if req.GroupMode != "" && req.GroupMode != "grouped" && req.GroupMode != "all" && req.GroupMode != "raw" {
		common.ApiErrorMsg(c, "Invalid group_mode: use grouped or all")
		return
	}
	if req.Source != "" && req.Source != "xpiki" && req.Source != "newapi" {
		common.ApiErrorMsg(c, "Invalid log source")
		return
	}

	credentials, err := buildXPikiCredentials(req.CustomKeys, req.Source != "newapi")
	if err != nil {
		common.ApiErrorMsg(c, "Could not load channel configuration")
		return
	}

	filter := service.XPikiLogsFilter{
		Source:        req.Source,
		KeyFilter:     req.KeyFilter,
		ChannelFilter: req.ChannelFilter,
		Search:        req.Search,
		Model:         req.Model,
		User:          req.User,
		Status:        req.Status,
		Type:          req.Type,
		Date:          req.Date,
		StartDate:     req.StartDate,
		EndDate:       req.EndDate,
		Page:          req.Page,
		PageSize:      req.PageSize,
		GroupMode:     req.GroupMode,
	}

	report, err := service.FetchXPikiLogsReport(c.Request.Context(), credentials, filter)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	common.ApiSuccess(c, report)
}
