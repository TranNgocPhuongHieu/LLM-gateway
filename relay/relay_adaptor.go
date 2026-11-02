package relay

import (
	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relay/channel"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relay/channel/claude"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relay/channel/openai"
)

func GetAdaptor(apiType int) channel.Adaptor {
	switch apiType {
	case constant.APITypeAnthropic:
		return &claude.Adaptor{}
	case constant.APITypeOpenAI:
		return &openai.Adaptor{}
	}
	return nil
}
