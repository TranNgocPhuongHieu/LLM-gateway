package common

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unsafe"

	kitutil "github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/relayconvert/kitutil"

	"github.com/samber/lo"
)

const LocalLogContentLimit = 2048

// LocalLogPreview limits log-only content unless debug logging is enabled.
func LocalLogPreview(content string) string {
	if DebugEnabled || len(content) <= LocalLogContentLimit {
		return content
	}
	return fmt.Sprintf("%s... [truncated, original_length=%d, limit=%d]", content[:LocalLogContentLimit], len(content), LocalLogContentLimit)
}

func GetStringIfEmpty(str string, defaultValue string) string {
	if str == "" {
		return defaultValue
	}
	return str
}

func GetRandomString(length int) string {
	if length <= 0 {
		return ""
	}
	return lo.RandomString(length, lo.AlphanumericCharset)
}

func String2Int(str string) int {
	num, err := strconv.Atoi(str)
	if err != nil {
		return 0
	}
	return num
}

func StringsContains(strs []string, str string) bool {
	return slices.Contains(strs, str)
}

// StringToByteSlice []byte only read, panic on append
func StringToByteSlice(s string) []byte {
	tmp1 := (*[2]uintptr)(unsafe.Pointer(&s))
	tmp2 := [3]uintptr{tmp1[0], tmp1[1], tmp1[1]}
	return *(*[]byte)(unsafe.Pointer(&tmp2))
}

func GetJsonString(data any) string {
	if data == nil {
		return ""
	}
	b, _ := json.Marshal(data)
	return string(b)
}

// NormalizeBillingPreference clamps the billing preference to valid values.
// NormalizeBillingPreference keeps the wallet-only contract for saved legacy settings.
func NormalizeBillingPreference(_ string) string {
	return "wallet_only"
}

// MaskEmail masks a user email to prevent PII leakage in logs
// Returns "***masked***" if email is empty, otherwise shows only the domain part
func MaskEmail(email string) string {
	if email == "" {
		return "***masked***"
	}

	// Find the @ symbol
	_, after, ok := strings.Cut(email, "@")
	if !ok {
		// No @ symbol found, return masked
		return "***masked***"
	}

	// Return only the domain part with @ symbol
	return "***@" + after
}

// MaskSensitiveInfo moved to the conversion kit (kitutil) because the types
// package error formatting depends on it; host callers keep this name.
func MaskSensitiveInfo(str string) string {
	return kitutil.MaskSensitiveInfo(str)
}
