package common

// Channel types map to the adaptor used for the selected upstream protocol.
func ChannelType2APIType(channelType int) (int, bool) {
	spec := GetChannelSpec(channelType)
	return spec.APIType, spec.APIType >= 0
}

func SupportsResponsesCompact(channelType, apiType int) bool {
	spec := GetChannelSpec(channelType)
	return spec.SupportsResponsesCompact && apiType == spec.APIType
}
