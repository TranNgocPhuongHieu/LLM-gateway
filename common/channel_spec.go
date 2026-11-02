package common

import "github.com/QuantumNous/new-api/constant"

// ChannelSpec is the static routing contract for one channel type.
// APIType is -1 when the current checkout has no adaptor mapping.
type ChannelSpec struct {
	ChannelType              int
	APIType                  int
	Name                     string
	DefaultBaseURL           string
	SupportsResponsesCompact bool
}

// GetChannelSpec returns a stable view of static channel metadata.
func GetChannelSpec(channelType int) ChannelSpec {
	apiType := -1
	switch channelType {
	case constant.ChannelTypeOpenAI:
		apiType = constant.APITypeOpenAI
	case constant.ChannelTypeAnthropic:
		apiType = constant.APITypeAnthropic
	}
	return ChannelSpec{
		ChannelType:              channelType,
		APIType:                  apiType,
		Name:                     constant.GetChannelTypeName(channelType),
		DefaultBaseURL:           constant.GetChannelBaseURL(channelType),
		SupportsResponsesCompact: channelType == constant.ChannelTypeOpenAI && apiType == constant.APITypeOpenAI,
	}
}
