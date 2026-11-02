package controller

import (
	"fmt"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

// validateChannel is the shared add/update channel validation boundary.
func validateChannel(channel *model.Channel, isAdd bool) error {
	if channel == nil {
		return fmt.Errorf("channel cannot be empty")
	}
	if channel.Type != constant.ChannelTypeOpenAI && channel.Type != constant.ChannelTypeAnthropic {
		return fmt.Errorf("unsupported upstream channel type: %d", channel.Type)
	}

	if err := channel.ValidateSettings(); err != nil {
		return fmt.Errorf("渠道额外设置[channel setting] 格式错误：%s", err.Error())
	}

	if isAdd {
		if channel.Key == "" {
			return fmt.Errorf("channel cannot be empty")
		}
		for _, m := range channel.GetModels() {
			if len(m) > 255 {
				return fmt.Errorf("模型名称过长: %s", m)
			}
		}
	}

	return nil
}
