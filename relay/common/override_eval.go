package common

import (
	"errors"
	"fmt"
	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"net/http"
	"strings"
)

func applyOperationsLegacy(jsonData []byte, paramOverride map[string]any, auditRecorder *paramOverrideAuditRecorder) ([]byte, error) {
	if len(paramOverride) == 0 {
		return jsonData, nil
	}

	result := jsonData
	for key, value := range paramOverride {
		escaped := escapeSjsonLiteralKey(key)
		next, err := sjson.SetBytes(result, escaped, value)
		if err != nil {
			return nil, err
		}
		result = next
		auditRecorder.recordOperation("set", key, "", "", value)
	}

	return result, nil
}

func escapeSjsonLiteralKey(key string) string {
	if !strings.ContainsAny(key, ".*?\\") {
		return key
	}
	var sb strings.Builder
	sb.Grow(len(key) + 4)
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch c {
		case '.', '*', '?', '\\':
			sb.WriteByte('\\')
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

func applyOperations(jsonData []byte, operations []ParamOperation, conditionContext map[string]any) ([]byte, error) {
	context := ensureContextMap(conditionContext)
	auditRecorder := getParamOverrideAuditRecorder(context)
	contextJSON, err := marshalContextJSON(context)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal condition context: %v", err)
	}

	result := jsonData
	for _, op := range operations {
		// 检查条件是否满足
		ok, err := checkConditions(result, contextJSON, op.Conditions, op.Logic)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue // 条件不满足，跳过当前操作
		}
		// 处理路径中的负数索引
		opPath := processNegativeIndex(result, op.Path)
		var opPaths []string
		if isPathBasedOperation(op.Mode) {
			opPaths, err = resolveOperationPaths(result, opPath)
			if err != nil {
				return nil, err
			}
			if len(opPaths) == 0 {
				continue
			}
		}

		switch op.Mode {
		case "delete":
			for _, path := range opPaths {
				result, err = deleteValue(result, path)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("delete", path, "", "", nil)
			}
		case "set":
			for _, path := range opPaths {
				if op.KeepOrigin && gjson.GetBytes(result, path).Exists() {
					continue
				}
				result, err = sjson.SetBytes(result, path, op.Value)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("set", path, "", "", op.Value)
			}
		case "move":
			opFrom := processNegativeIndex(result, op.From)
			opTo := processNegativeIndex(result, op.To)
			result, err = moveValue(result, opFrom, opTo)
			if err == nil {
				auditRecorder.recordOperation("move", "", opFrom, opTo, nil)
			}
		case "copy":
			if op.From == "" || op.To == "" {
				return nil, fmt.Errorf("copy from/to is required")
			}
			opFrom := processNegativeIndex(result, op.From)
			opTo := processNegativeIndex(result, op.To)
			result, err = copyValue(result, opFrom, opTo)
			if err == nil {
				auditRecorder.recordOperation("copy", "", opFrom, opTo, nil)
			}
		case "prepend":
			for _, path := range opPaths {
				result, err = modifyValue(result, path, op.Value, op.KeepOrigin, true)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("prepend", path, "", "", op.Value)
			}
		case "append":
			for _, path := range opPaths {
				result, err = modifyValue(result, path, op.Value, op.KeepOrigin, false)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("append", path, "", "", op.Value)
			}
		case "trim_prefix":
			for _, path := range opPaths {
				result, err = trimStringValue(result, path, op.Value, true)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("trim_prefix", path, "", "", op.Value)
			}
		case "trim_suffix":
			for _, path := range opPaths {
				result, err = trimStringValue(result, path, op.Value, false)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("trim_suffix", path, "", "", op.Value)
			}
		case "ensure_prefix":
			for _, path := range opPaths {
				result, err = ensureStringAffix(result, path, op.Value, true)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("ensure_prefix", path, "", "", op.Value)
			}
		case "ensure_suffix":
			for _, path := range opPaths {
				result, err = ensureStringAffix(result, path, op.Value, false)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("ensure_suffix", path, "", "", op.Value)
			}
		case "trim_space":
			for _, path := range opPaths {
				result, err = transformStringValue(result, path, strings.TrimSpace)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("trim_space", path, "", "", nil)
			}
		case "to_lower":
			for _, path := range opPaths {
				result, err = transformStringValue(result, path, strings.ToLower)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("to_lower", path, "", "", nil)
			}
		case "to_upper":
			for _, path := range opPaths {
				result, err = transformStringValue(result, path, strings.ToUpper)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("to_upper", path, "", "", nil)
			}
		case "replace":
			for _, path := range opPaths {
				result, err = replaceStringValue(result, path, op.From, op.To)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("replace", path, op.From, op.To, nil)
			}
		case "regex_replace":
			for _, path := range opPaths {
				result, err = regexReplaceStringValue(result, path, op.From, op.To)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("regex_replace", path, op.From, op.To, nil)
			}
		case "return_error":
			auditRecorder.recordOperation("return_error", op.Path, "", "", op.Value)
			returnErr, parseErr := parseParamOverrideReturnError(op.Value)
			if parseErr != nil {
				return nil, parseErr
			}
			return nil, returnErr
		case "prune_objects":
			for _, path := range opPaths {
				result, err = pruneObjects(result, path, contextJSON, op.Value)
				if err != nil {
					break
				}
			}
		case "set_header":
			err = setHeaderOverrideInContext(context, op.Path, op.Value, op.KeepOrigin)
			if err == nil {
				auditRecorder.recordOperation("set_header", op.Path, "", "", op.Value)
				contextJSON, err = marshalContextJSON(context)
			}
		case "delete_header":
			err = deleteHeaderOverrideInContext(context, op.Path)
			if err == nil {
				auditRecorder.recordOperation("delete_header", op.Path, "", "", nil)
				contextJSON, err = marshalContextJSON(context)
			}
		case "copy_header":
			sourceHeader := strings.TrimSpace(op.From)
			targetHeader := strings.TrimSpace(op.To)
			if sourceHeader == "" {
				sourceHeader = strings.TrimSpace(op.Path)
			}
			if targetHeader == "" {
				targetHeader = strings.TrimSpace(op.Path)
			}
			err = copyHeaderInContext(context, sourceHeader, targetHeader, op.KeepOrigin)
			if errors.Is(err, errSourceHeaderNotFound) {
				err = nil
			}
			if err == nil {
				auditRecorder.recordOperation("copy_header", "", sourceHeader, targetHeader, nil)
				contextJSON, err = marshalContextJSON(context)
			}
		case "move_header":
			sourceHeader := strings.TrimSpace(op.From)
			targetHeader := strings.TrimSpace(op.To)
			if sourceHeader == "" {
				sourceHeader = strings.TrimSpace(op.Path)
			}
			if targetHeader == "" {
				targetHeader = strings.TrimSpace(op.Path)
			}
			err = moveHeaderInContext(context, sourceHeader, targetHeader, op.KeepOrigin)
			if errors.Is(err, errSourceHeaderNotFound) {
				err = nil
			}
			if err == nil {
				auditRecorder.recordOperation("move_header", "", sourceHeader, targetHeader, nil)
				contextJSON, err = marshalContextJSON(context)
			}
		case "pass_headers":
			headerNames, parseErr := parseHeaderPassThroughNames(op.Value)
			if parseErr != nil {
				return nil, parseErr
			}
			for _, headerName := range headerNames {
				if err = copyHeaderInContext(context, headerName, headerName, op.KeepOrigin); err != nil {
					if errors.Is(err, errSourceHeaderNotFound) {
						err = nil
						continue
					}
					break
				}
			}
			if err == nil {
				auditRecorder.recordOperation("pass_headers", "", "", "", headerNames)
				contextJSON, err = marshalContextJSON(context)
			}
		case "sync_fields":
			result, err = syncFieldsBetweenTargets(result, context, op.From, op.To)
			if err == nil {
				auditRecorder.recordOperation("sync_fields", "", op.From, op.To, nil)
				contextJSON, err = marshalContextJSON(context)
			}
		default:
			return nil, fmt.Errorf("unknown operation: %s", op.Mode)
		}
		if err != nil {
			return nil, fmt.Errorf("operation %s failed: %w", op.Mode, err)
		}
	}
	return result, nil
}

func parseParamOverrideReturnError(value any) (*ParamOverrideReturnError, error) {
	result := &ParamOverrideReturnError{
		StatusCode: http.StatusBadRequest,
		Code:       string(types.ErrorCodeInvalidRequest),
		Type:       "invalid_request_error",
		SkipRetry:  true,
	}

	switch raw := value.(type) {
	case nil:
		return nil, fmt.Errorf("return_error value is required")
	case string:
		result.Message = strings.TrimSpace(raw)
	case map[string]any:
		if message, ok := raw["message"].(string); ok {
			result.Message = strings.TrimSpace(message)
		}
		if result.Message == "" {
			if message, ok := raw["msg"].(string); ok {
				result.Message = strings.TrimSpace(message)
			}
		}

		if code, exists := raw["code"]; exists {
			codeStr := strings.TrimSpace(fmt.Sprintf("%v", code))
			if codeStr != "" {
				result.Code = codeStr
			}
		}
		if errType, ok := raw["type"].(string); ok {
			errType = strings.TrimSpace(errType)
			if errType != "" {
				result.Type = errType
			}
		}
		if skipRetry, ok := raw["skip_retry"].(bool); ok {
			result.SkipRetry = skipRetry
		}

		if statusCodeRaw, exists := raw["status_code"]; exists {
			statusCode, ok := parseOverrideInt(statusCodeRaw)
			if !ok {
				return nil, fmt.Errorf("return_error status_code must be an integer")
			}
			result.StatusCode = statusCode
		} else if statusRaw, exists := raw["status"]; exists {
			statusCode, ok := parseOverrideInt(statusRaw)
			if !ok {
				return nil, fmt.Errorf("return_error status must be an integer")
			}
			result.StatusCode = statusCode
		}
	default:
		return nil, fmt.Errorf("return_error value must be string or object")
	}

	if result.Message == "" {
		return nil, fmt.Errorf("return_error message is required")
	}
	if result.StatusCode < http.StatusContinue || result.StatusCode > http.StatusNetworkAuthenticationRequired {
		return nil, fmt.Errorf("return_error status code out of range: %d", result.StatusCode)
	}

	return result, nil
}

func parseOverrideInt(v any) (int, bool) {
	switch value := v.(type) {
	case int:
		return value, true
	case float64:
		if value != float64(int(value)) {
			return 0, false
		}
		return int(value), true
	default:
		return 0, false
	}
}

func ensureContextMap(conditionContext map[string]any) map[string]any {
	if conditionContext != nil {
		return conditionContext
	}
	return make(map[string]any)
}

func marshalContextJSON(context map[string]any) (string, error) {
	if context == nil || len(context) == 0 {
		return "", nil
	}
	ctxBytes, err := common.Marshal(context)
	if err != nil {
		return "", err
	}
	return string(ctxBytes), nil
}

func setHeaderOverrideInContext(context map[string]any, headerName string, value any, keepOrigin bool) error {
	headerName = normalizeHeaderContextKey(headerName)
	if headerName == "" {
		return fmt.Errorf("header name is required")
	}

	rawHeaders := ensureMapKeyInContext(context, paramOverrideContextHeaderOverride)
	if keepOrigin {
		if existing, ok := rawHeaders[headerName]; ok {
			existingValue := strings.TrimSpace(fmt.Sprintf("%v", existing))
			if existingValue != "" {
				return nil
			}
		}
	}

	headerValue, hasValue, err := resolveHeaderOverrideValue(context, headerName, value)
	if err != nil {
		return err
	}
	if !hasValue {
		delete(rawHeaders, headerName)
		return nil
	}

	rawHeaders[headerName] = headerValue
	return nil
}

func resolveHeaderOverrideValue(context map[string]any, headerName string, value any) (string, bool, error) {
	if value == nil {
		return "", false, fmt.Errorf("header value is required")
	}

	if mapping, ok := value.(map[string]any); ok {
		return resolveHeaderOverrideValueByMapping(context, headerName, mapping)
	}
	if mapping, ok := value.(map[string]string); ok {
		converted := make(map[string]any, len(mapping))
		for key, item := range mapping {
			converted[key] = item
		}
		return resolveHeaderOverrideValueByMapping(context, headerName, converted)
	}

	headerValue := strings.TrimSpace(fmt.Sprintf("%v", value))
	if headerValue == "" {
		return "", false, nil
	}
	return headerValue, true, nil
}

func resolveHeaderOverrideValueByMapping(context map[string]any, headerName string, mapping map[string]any) (string, bool, error) {
	if len(mapping) == 0 {
		return "", false, fmt.Errorf("header value mapping cannot be empty")
	}

	appendTokens, err := parseHeaderAppendTokens(mapping)
	if err != nil {
		return "", false, err
	}
	keepOnlyDeclared := parseHeaderKeepOnlyDeclared(mapping)

	sourceValue, exists := getHeaderValueFromContext(context, headerName)
	sourceTokens := make([]string, 0)
	if exists {
		sourceTokens = splitHeaderListValue(sourceValue)
	}

	wildcardValue, hasWildcard := mapping["*"]
	resultTokens := make([]string, 0, len(sourceTokens)+len(appendTokens))
	for _, token := range sourceTokens {
		replacementRaw, hasReplacement := mapping[token]
		if !hasReplacement && hasWildcard && !keepOnlyDeclared {
			replacementRaw = wildcardValue
			hasReplacement = true
		}
		if !hasReplacement {
			if keepOnlyDeclared {
				continue
			}
			resultTokens = append(resultTokens, token)
			continue
		}
		replacementTokens, err := parseHeaderReplacementTokens(replacementRaw)
		if err != nil {
			return "", false, err
		}
		resultTokens = append(resultTokens, replacementTokens...)
	}

	resultTokens = append(resultTokens, appendTokens...)
	resultTokens = lo.Uniq(resultTokens)
	if len(resultTokens) == 0 {
		return "", false, nil
	}
	return strings.Join(resultTokens, ","), true, nil
}
