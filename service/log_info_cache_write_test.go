package service

import (
	"testing"
	"time"

	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Cache creation is settled as input, so Claude logs must not advertise an
// unused write price (the usage-log UI renders cache_creation_ratio != 1).
func TestGenerateClaudeOtherInfo_OmitsUnusedCacheWritePrice(t *testing.T) {
	ctx, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{}}
	other := GenerateClaudeOtherInfo(ctx, info, 1, 1, 5, 200, 0.1, 0, 1.25, 0, 1.25, 0, 2, 0, -1).Snapshot()
	require.Equal(t, true, other["claude"])
	require.EqualValues(t, 200, other["cache_tokens"])
	for _, key := range []string{
		"cache_creation_tokens", "cache_creation_ratio",
		"cache_creation_tokens_5m", "cache_creation_ratio_5m",
		"cache_creation_tokens_1h", "cache_creation_ratio_1h",
	} {
		require.NotContains(t, other, key)
	}
}
