package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestGetChannelSpecMatchesExistingChannelContracts(t *testing.T) {
	for channelType := -1; channelType <= 100; channelType++ {
		spec := GetChannelSpec(channelType)
		if spec.ChannelType != channelType {
			t.Fatalf("channel type: got %d, want %d", spec.ChannelType, channelType)
		}
		if spec.Name != constant.GetChannelTypeName(channelType) {
			t.Fatalf("name for %d: got %q, want %q", channelType, spec.Name, constant.GetChannelTypeName(channelType))
		}
		if spec.DefaultBaseURL != constant.GetChannelBaseURL(channelType) {
			t.Fatalf("base URL for %d: got %q, want %q", channelType, spec.DefaultBaseURL, constant.GetChannelBaseURL(channelType))
		}
		wantAPI, wantSupported := expectedChannelAPIType(channelType)
		gotAPI, gotSupported := ChannelType2APIType(channelType)
		if gotAPI != wantAPI || gotSupported != wantSupported {
			t.Fatalf("API type for %d: got (%d, %t), want (%d, %t)", channelType, gotAPI, gotSupported, wantAPI, wantSupported)
		}
		if spec.SupportsResponsesCompact != (channelType == constant.ChannelTypeOpenAI) {
			t.Fatalf("compact support for %d: got %t", channelType, spec.SupportsResponsesCompact)
		}
	}
}

func expectedChannelAPIType(channelType int) (int, bool) {
	switch channelType {
	case constant.ChannelTypeOpenAI:
		return constant.APITypeOpenAI, true
	case constant.ChannelTypeAnthropic:
		return constant.APITypeAnthropic, true
	default:
		return -1, false
	}
}
