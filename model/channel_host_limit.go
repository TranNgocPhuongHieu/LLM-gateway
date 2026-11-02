package model

import (
	"net/url"
	"strings"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
)

func channelHost(ch *Channel) string {
	if ch == nil {
		return ""
	}
	baseURL := ch.GetBaseURL()
	if baseURL == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err == nil && u.Hostname() != "" {
		return strings.ToLower(u.Hostname())
	}
	raw := baseURL
	if idx := strings.Index(raw, "://"); idx != -1 {
		raw = raw[idx+3:]
	}
	if idx := strings.Index(raw, "/"); idx != -1 {
		raw = raw[:idx]
	}
	if idx := strings.Index(raw, ":"); idx != -1 {
		raw = raw[:idx]
	}
	return strings.ToLower(raw)
}

func filterChannelsByHostAttemptLimit(untried []int, excludedChannelIDs []int) []int {
	if len(untried) == 0 || len(excludedChannelIDs) == 0 {
		return untried
	}
	maxAttempts := common.GetEnvOrDefault("RELAY_MAX_HOST_ATTEMPTS", 1)
	if maxAttempts <= 0 {
		return untried
	}

	attemptedCounts := make(map[string]int)
	for _, id := range excludedChannelIDs {
		if ch, ok := channelsIDM[id]; ok && ch != nil {
			h := channelHost(ch)
			if h != "" {
				attemptedCounts[h]++
			}
		}
	}

	exhaustedHosts := make(map[string]bool)
	for h, count := range attemptedCounts {
		if count >= maxAttempts {
			exhaustedHosts[h] = true
		}
	}
	if len(exhaustedHosts) == 0 {
		return untried
	}

	hasAlternative := false
	for _, id := range untried {
		if ch, ok := channelsIDM[id]; ok && ch != nil {
			h := channelHost(ch)
			if h != "" && !exhaustedHosts[h] {
				hasAlternative = true
				break
			}
		}
	}

	if hasAlternative {
		filtered := make([]int, 0, len(untried))
		for _, id := range untried {
			if ch, ok := channelsIDM[id]; ok && ch != nil {
				h := channelHost(ch)
				if !exhaustedHosts[h] {
					filtered = append(filtered, id)
				}
			} else {
				filtered = append(filtered, id)
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
	}

	return untried
}

func filterAbilitiesByHostAttemptLimit(abilities []Ability, excludedChannelIDs []int) []Ability {
	if len(abilities) == 0 || len(excludedChannelIDs) == 0 {
		return abilities
	}
	maxAttempts := common.GetEnvOrDefault("RELAY_MAX_HOST_ATTEMPTS", 1)
	if maxAttempts <= 0 {
		return abilities
	}

	allChannelIDs := make([]int, 0, len(excludedChannelIDs)+len(abilities))
	for _, id := range excludedChannelIDs {
		allChannelIDs = append(allChannelIDs, id)
	}
	for _, a := range abilities {
		allChannelIDs = append(allChannelIDs, a.ChannelId)
	}
	var channels []Channel
	if err := DB.Where("id IN ?", allChannelIDs).Find(&channels).Error; err != nil || len(channels) == 0 {
		return abilities
	}
	chMap := make(map[int]*Channel)
	for i := range channels {
		chMap[channels[i].Id] = &channels[i]
	}

	attemptedCounts := make(map[string]int)
	for _, id := range excludedChannelIDs {
		if ch, ok := chMap[id]; ok && ch != nil {
			h := channelHost(ch)
			if h != "" {
				attemptedCounts[h]++
			}
		}
	}

	exhaustedHosts := make(map[string]bool)
	for h, count := range attemptedCounts {
		if count >= maxAttempts {
			exhaustedHosts[h] = true
		}
	}
	if len(exhaustedHosts) == 0 {
		return abilities
	}

	hasAlternative := false
	for _, a := range abilities {
		if ch, ok := chMap[a.ChannelId]; ok && ch != nil {
			h := channelHost(ch)
			if h != "" && !exhaustedHosts[h] {
				hasAlternative = true
				break
			}
		}
	}

	if hasAlternative {
		filtered := make([]Ability, 0, len(abilities))
		for _, a := range abilities {
			if ch, ok := chMap[a.ChannelId]; ok && ch != nil {
				h := channelHost(ch)
				if !exhaustedHosts[h] {
					filtered = append(filtered, a)
				}
			} else {
				filtered = append(filtered, a)
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
	}

	return abilities
}
