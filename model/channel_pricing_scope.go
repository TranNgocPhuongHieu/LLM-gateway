package model

import (
	"strings"
)

type ConfiguredChannelModel struct {
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	ChannelType int    `json:"channel_type"`
	Model       string `json:"model"`
}

type ChannelPricingScope struct {
	ChannelModels []ConfiguredChannelModel `json:"channel_models"`
	UniqueModels  []string                 `json:"unique_models"`
}

// GetConfiguredChannelPricingScope returns all models declared in channels (including disabled channels).
// It queries database configuration only and never makes upstream network requests.
func GetConfiguredChannelPricingScope() (*ChannelPricingScope, error) {
	var channels []Channel
	if err := DB.Select("id", "name", "type", "status", "models").Order("id asc").Find(&channels).Error; err != nil {
		return nil, err
	}

	channelModels := make([]ConfiguredChannelModel, 0)
	uniqueMap := make(map[string]struct{})
	uniqueModels := make([]string, 0)

	for _, ch := range channels {
		models := ch.GetModels()
		for _, m := range models {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			channelModels = append(channelModels, ConfiguredChannelModel{
				ChannelId:   ch.Id,
				ChannelName: ch.Name,
				ChannelType: ch.Type,
				Model:       m,
			})
			if _, exists := uniqueMap[m]; !exists {
				uniqueMap[m] = struct{}{}
				uniqueModels = append(uniqueModels, m)
			}
		}
	}

	return &ChannelPricingScope{
		ChannelModels: channelModels,
		UniqueModels:  uniqueModels,
	}, nil
}

