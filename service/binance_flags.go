package service

import (
	"os"
	"strconv"
	"strings"
)

// BinanceFeatureFlags are intentionally fail-closed in phase 1. No route,
// worker, or settlement path consumes these flags until phase 2 wires the
// reviewed configuration into runtime startup.
type BinanceFeatureFlags struct {
	CheckoutEnabled       bool
	RealPollingEnabled    bool
	AutoSettlementEnabled bool
	MockMode              bool
	IngestOnly            bool
}

var DefaultBinanceFeatureFlags = BinanceFeatureFlags{
	CheckoutEnabled:       false,
	RealPollingEnabled:    false,
	AutoSettlementEnabled: false,
	MockMode:              true,
	IngestOnly:            true,
}

// IsBinanceCanaryUser is an explicit, fail-closed allowlist used for a
// production canary while public checkout remains disabled. IDs are supplied
// as a comma-separated environment variable and no user is allowed when it
// is empty or malformed.
func IsBinanceCanaryUser(userID int) bool {
	if userID <= 0 {
		return false
	}
	for _, raw := range strings.Split(os.Getenv("BINANCE_CANARY_USER_IDS"), ",") {
		id, err := strconv.Atoi(strings.TrimSpace(raw))
		if err == nil && id == userID {
			return true
		}
	}
	return false
}
