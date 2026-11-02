package common

import (
	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	"testing"
)

func TestGatewayChannelSupport(t *testing.T) {
	for channelType := 0; channelType <= constant.ChannelTypeDummy; channelType++ {
		apiType, supported := ChannelType2APIType(channelType)
		if channelType == constant.ChannelTypeOpenAI {
			if !supported || apiType != constant.APITypeOpenAI || !SupportsResponsesCompact(channelType, apiType) {
				t.Fatalf("compatible channel lost support: api=%d supported=%v", apiType, supported)
			}
		} else if channelType == constant.ChannelTypeAnthropic {
			if !supported || apiType != constant.APITypeAnthropic || SupportsResponsesCompact(channelType, apiType) {
				t.Fatalf("Anthropic Messages channel lost support: api=%d supported=%v", apiType, supported)
			}
		} else if supported || apiType != -1 || SupportsResponsesCompact(channelType, apiType) {
			t.Fatalf("retired channel %d unexpectedly supported", channelType)
		}
	}
}
