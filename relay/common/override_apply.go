package common

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"maps"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func parseHeaderAppendTokens(mapping map[string]any) ([]string, error) {
	appendRaw, ok := mapping["$append"]
	if !ok {
		return nil, nil
	}
	return parseHeaderReplacementTokens(appendRaw)
}

func parseHeaderKeepOnlyDeclared(mapping map[string]any) bool {
	keepOnlyDeclaredRaw, ok := mapping["$keep_only_declared"]
	if !ok {
		return false
	}
	keepOnlyDeclared, ok := keepOnlyDeclaredRaw.(bool)
	if !ok {
		return false
	}
	return keepOnlyDeclared
}

func parseHeaderReplacementTokens(value any) ([]string, error) {
	switch raw := value.(type) {
	case nil:
		return nil, nil
	case string:
		return splitHeaderListValue(raw), nil
	case []string:
		tokens := make([]string, 0, len(raw))
		for _, item := range raw {
			tokens = append(tokens, splitHeaderListValue(item)...)
		}
		return lo.Uniq(tokens), nil
	case []any:
		tokens := make([]string, 0, len(raw))
		for _, item := range raw {
			itemTokens, err := parseHeaderReplacementTokens(item)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, itemTokens...)
		}
		return lo.Uniq(tokens), nil
	case map[string]any, map[string]string:
		return nil, fmt.Errorf("header replacement value must be string, array or null")
	default:
		token := strings.TrimSpace(fmt.Sprintf("%v", raw))
		if token == "" {
			return nil, nil
		}
		return []string{token}, nil
	}
}

func splitHeaderListValue(raw string) []string {
	items := strings.Split(raw, ",")
	return lo.FilterMap(items, func(item string, _ int) (string, bool) {
		token := strings.TrimSpace(item)
		if token == "" {
			return "", false
		}
		return token, true
	})
}

func copyHeaderInContext(context map[string]any, fromHeader, toHeader string, keepOrigin bool) error {
	fromHeader = normalizeHeaderContextKey(fromHeader)
	toHeader = normalizeHeaderContextKey(toHeader)
	if fromHeader == "" || toHeader == "" {
		return fmt.Errorf("copy_header from/to is required")
	}
	value, exists := getHeaderValueFromContext(context, fromHeader)
	if !exists {
		return fmt.Errorf("%w: %s", errSourceHeaderNotFound, fromHeader)
	}
	return setHeaderOverrideInContext(context, toHeader, value, keepOrigin)
}

func moveHeaderInContext(context map[string]any, fromHeader, toHeader string, keepOrigin bool) error {
	fromHeader = normalizeHeaderContextKey(fromHeader)
	toHeader = normalizeHeaderContextKey(toHeader)
	if fromHeader == "" || toHeader == "" {
		return fmt.Errorf("move_header from/to is required")
	}
	if err := copyHeaderInContext(context, fromHeader, toHeader, keepOrigin); err != nil {
		return err
	}
	if strings.EqualFold(fromHeader, toHeader) {
		return nil
	}
	return deleteHeaderOverrideInContext(context, fromHeader)
}

func deleteHeaderOverrideInContext(context map[string]any, headerName string) error {
	headerName = normalizeHeaderContextKey(headerName)
	if headerName == "" {
		return fmt.Errorf("header name is required")
	}
	rawHeaders := ensureMapKeyInContext(context, paramOverrideContextHeaderOverride)
	delete(rawHeaders, headerName)
	return nil
}

func parseHeaderPassThroughNames(value any) ([]string, error) {
	normalizeNames := func(values []string) []string {
		names := lo.FilterMap(values, func(item string, _ int) (string, bool) {
			headerName := normalizeHeaderContextKey(item)
			if headerName == "" {
				return "", false
			}
			return headerName, true
		})
		return lo.Uniq(names)
	}

	switch raw := value.(type) {
	case nil:
		return nil, fmt.Errorf("pass_headers value is required")
	case string:
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, fmt.Errorf("pass_headers value is required")
		}
		if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
			var parsed any
			if err := common.UnmarshalJsonStr(trimmed, &parsed); err == nil {
				return parseHeaderPassThroughNames(parsed)
			}
		}
		names := normalizeNames(strings.Split(trimmed, ","))
		if len(names) == 0 {
			return nil, fmt.Errorf("pass_headers value is invalid")
		}
		return names, nil
	case []any:
		names := lo.FilterMap(raw, func(item any, _ int) (string, bool) {
			headerName := normalizeHeaderContextKey(fmt.Sprintf("%v", item))
			if headerName == "" {
				return "", false
			}
			return headerName, true
		})
		names = lo.Uniq(names)
		if len(names) == 0 {
			return nil, fmt.Errorf("pass_headers value is invalid")
		}
		return names, nil
	case []string:
		names := lo.FilterMap(raw, func(item string, _ int) (string, bool) {
			headerName := normalizeHeaderContextKey(item)
			if headerName == "" {
				return "", false
			}
			return headerName, true
		})
		names = lo.Uniq(names)
		if len(names) == 0 {
			return nil, fmt.Errorf("pass_headers value is invalid")
		}
		return names, nil
	case map[string]any:
		candidates := make([]string, 0, 8)
		if headersRaw, ok := raw["headers"]; ok {
			names, err := parseHeaderPassThroughNames(headersRaw)
			if err == nil {
				candidates = append(candidates, names...)
			}
		}
		if namesRaw, ok := raw["names"]; ok {
			names, err := parseHeaderPassThroughNames(namesRaw)
			if err == nil {
				candidates = append(candidates, names...)
			}
		}
		if headerRaw, ok := raw["header"]; ok {
			names, err := parseHeaderPassThroughNames(headerRaw)
			if err == nil {
				candidates = append(candidates, names...)
			}
		}
		names := normalizeNames(candidates)
		if len(names) == 0 {
			return nil, fmt.Errorf("pass_headers value is invalid")
		}
		return names, nil
	default:
		return nil, fmt.Errorf("pass_headers value must be string, array or object")
	}
}

type syncTarget struct {
	kind string
	key  string
}

func parseSyncTarget(spec string) (syncTarget, error) {
	raw := strings.TrimSpace(spec)
	if raw == "" {
		return syncTarget{}, fmt.Errorf("sync_fields target is required")
	}

	before, after, ok := strings.Cut(raw, ":")
	if !ok {
		// Backward compatibility: treat bare value as JSON path.
		return syncTarget{
			kind: "json",
			key:  raw,
		}, nil
	}

	kind := strings.ToLower(strings.TrimSpace(before))
	key := strings.TrimSpace(after)
	if key == "" {
		return syncTarget{}, fmt.Errorf("sync_fields target key is required: %s", raw)
	}

	switch kind {
	case "json", "body":
		return syncTarget{
			kind: "json",
			key:  key,
		}, nil
	case "header":
		return syncTarget{
			kind: "header",
			key:  key,
		}, nil
	default:
		return syncTarget{}, fmt.Errorf("sync_fields target prefix is invalid: %s", raw)
	}
}

func readSyncTargetValue(data []byte, context map[string]any, target syncTarget) (any, bool, error) {
	switch target.kind {
	case "json":
		path := processNegativeIndex(data, target.key)
		value := gjson.GetBytes(data, path)
		if !value.Exists() || value.Type == gjson.Null {
			return nil, false, nil
		}
		if value.Type == gjson.String && strings.TrimSpace(value.String()) == "" {
			return nil, false, nil
		}
		return value.Value(), true, nil
	case "header":
		value, ok := getHeaderValueFromContext(context, target.key)
		if !ok || strings.TrimSpace(value) == "" {
			return nil, false, nil
		}
		return value, true, nil
	default:
		return nil, false, fmt.Errorf("unsupported sync_fields target kind: %s", target.kind)
	}
}

func writeSyncTargetValue(data []byte, context map[string]any, target syncTarget, value any) ([]byte, error) {
	switch target.kind {
	case "json":
		path := processNegativeIndex(data, target.key)
		nextJSON, err := sjson.SetBytes(data, path, value)
		if err != nil {
			return nil, err
		}
		return nextJSON, nil
	case "header":
		if err := setHeaderOverrideInContext(context, target.key, value, false); err != nil {
			return nil, err
		}
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported sync_fields target kind: %s", target.kind)
	}
}

func syncFieldsBetweenTargets(data []byte, context map[string]any, fromSpec string, toSpec string) ([]byte, error) {
	fromTarget, err := parseSyncTarget(fromSpec)
	if err != nil {
		return nil, err
	}
	toTarget, err := parseSyncTarget(toSpec)
	if err != nil {
		return nil, err
	}

	fromValue, fromExists, err := readSyncTargetValue(data, context, fromTarget)
	if err != nil {
		return nil, err
	}
	toValue, toExists, err := readSyncTargetValue(data, context, toTarget)
	if err != nil {
		return nil, err
	}

	// If one side exists and the other side is missing, sync the missing side.
	if fromExists && !toExists {
		return writeSyncTargetValue(data, context, toTarget, fromValue)
	}
	if toExists && !fromExists {
		return writeSyncTargetValue(data, context, fromTarget, toValue)
	}
	return data, nil
}

func ensureMapKeyInContext(context map[string]any, key string) map[string]any {
	if context == nil {
		return map[string]any{}
	}
	if existing, ok := context[key]; ok {
		if mapVal, ok := existing.(map[string]any); ok {
			return mapVal
		}
	}
	result := make(map[string]any)
	context[key] = result
	return result
}

func getHeaderValueFromContext(context map[string]any, headerName string) (string, bool) {
	headerName = normalizeHeaderContextKey(headerName)
	if headerName == "" {
		return "", false
	}
	for _, key := range []string{paramOverrideContextHeaderOverride, paramOverrideContextRequestHeaders} {
		source := ensureMapKeyInContext(context, key)
		raw, ok := source[headerName]
		if !ok {
			continue
		}
		value := strings.TrimSpace(fmt.Sprintf("%v", raw))
		if value != "" {
			return value, true
		}
	}
	return "", false
}

func normalizeHeaderContextKey(key string) string {
	return strings.TrimSpace(strings.ToLower(key))
}

func buildRequestHeadersContext(headers map[string]string) map[string]any {
	if len(headers) == 0 {
		return map[string]any{}
	}
	entries := lo.Entries(headers)
	normalizedEntries := lo.FilterMap(entries, func(item lo.Entry[string, string], _ int) (lo.Entry[string, string], bool) {
		normalized := normalizeHeaderContextKey(item.Key)
		value := strings.TrimSpace(item.Value)
		if normalized == "" || value == "" {
			return lo.Entry[string, string]{}, false
		}
		return lo.Entry[string, string]{Key: normalized, Value: value}, true
	})
	return lo.SliceToMap(normalizedEntries, func(item lo.Entry[string, string]) (string, any) {
		return item.Key, item.Value
	})
}

func syncRuntimeHeaderOverrideFromContext(info *RelayInfo, context map[string]any) {
	if info == nil || context == nil {
		return
	}
	raw, exists := context[paramOverrideContextHeaderOverride]
	if !exists {
		return
	}
	rawMap, ok := raw.(map[string]any)
	if !ok {
		return
	}
	info.RuntimeHeadersOverride = sanitizeHeaderOverrideMap(rawMap)
	info.UseRuntimeHeadersOverride = true
}

func moveValue(data []byte, fromPath, toPath string) ([]byte, error) {
	sourceValue := gjson.GetBytes(data, fromPath)
	if !sourceValue.Exists() {
		return data, fmt.Errorf("source path does not exist: %s", fromPath)
	}
	result, err := sjson.SetBytes(data, toPath, sourceValue.Value())
	if err != nil {
		return nil, err
	}
	return sjson.DeleteBytes(result, fromPath)
}

func copyValue(data []byte, fromPath, toPath string) ([]byte, error) {
	sourceValue := gjson.GetBytes(data, fromPath)
	if !sourceValue.Exists() {
		return data, fmt.Errorf("source path does not exist: %s", fromPath)
	}
	return sjson.SetBytes(data, toPath, sourceValue.Value())
}

func isPathBasedOperation(mode string) bool {
	switch mode {
	case "delete", "set", "prepend", "append", "trim_prefix", "trim_suffix", "ensure_prefix", "ensure_suffix", "trim_space", "to_lower", "to_upper", "replace", "regex_replace", "prune_objects":
		return true
	default:
		return false
	}
}

func resolveOperationPaths(data []byte, path string) ([]string, error) {
	if !strings.Contains(path, "*") {
		return []string{path}, nil
	}
	return expandWildcardPaths(data, path)
}

func expandWildcardPaths(data []byte, path string) ([]string, error) {
	var root any
	if err := common.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	segments := strings.Split(path, ".")
	paths := collectWildcardPaths(root, segments, nil)
	return lo.Uniq(paths), nil
}

func collectWildcardPaths(node any, segments []string, prefix []string) []string {
	if len(segments) == 0 {
		return []string{strings.Join(prefix, ".")}
	}

	segment := strings.TrimSpace(segments[0])
	if segment == "" {
		return nil
	}
	isLast := len(segments) == 1

	if segment == "*" {
		switch typed := node.(type) {
		case map[string]any:
			keys := lo.Keys(typed)
			sort.Strings(keys)
			return lo.FlatMap(keys, func(key string, _ int) []string {
				return collectWildcardPaths(typed[key], segments[1:], append(prefix, key))
			})
		case []any:
			return lo.FlatMap(lo.Range(len(typed)), func(index int, _ int) []string {
				return collectWildcardPaths(typed[index], segments[1:], append(prefix, strconv.Itoa(index)))
			})
		default:
			return nil
		}
	}

	switch typed := node.(type) {
	case map[string]any:
		if isLast {
			return []string{strings.Join(append(prefix, segment), ".")}
		}
		next, exists := typed[segment]
		if !exists {
			return nil
		}
		return collectWildcardPaths(next, segments[1:], append(prefix, segment))
	case []any:
		index, err := strconv.Atoi(segment)
		if err != nil || index < 0 || index >= len(typed) {
			return nil
		}
		if isLast {
			return []string{strings.Join(append(prefix, segment), ".")}
		}
		return collectWildcardPaths(typed[index], segments[1:], append(prefix, segment))
	default:
		return nil
	}
}

func deleteValue(data []byte, path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return data, nil
	}
	return sjson.DeleteBytes(data, path)
}

func modifyValue(data []byte, path string, value any, keepOrigin, isPrepend bool) ([]byte, error) {
	current := gjson.GetBytes(data, path)
	switch {
	case current.IsArray():
		return modifyArray(data, path, value, isPrepend)
	case current.Type == gjson.String:
		return modifyString(data, path, value, isPrepend)
	case current.Type == gjson.JSON:
		return mergeObjects(data, path, value, keepOrigin)
	}
	return data, fmt.Errorf("operation not supported for type: %v", current.Type)
}

func modifyArray(data []byte, path string, value any, isPrepend bool) ([]byte, error) {
	current := gjson.GetBytes(data, path)
	var newArray []any
	// 添加新值
	addValue := func() {
		if arr, ok := value.([]any); ok {
			newArray = append(newArray, arr...)
		} else {
			newArray = append(newArray, value)
		}
	}
	// 添加原值
	addOriginal := func() {
		current.ForEach(func(_, val gjson.Result) bool {
			newArray = append(newArray, val.Value())
			return true
		})
	}
	if isPrepend {
		addValue()
		addOriginal()
	} else {
		addOriginal()
		addValue()
	}
	return sjson.SetBytes(data, path, newArray)
}

func modifyString(data []byte, path string, value any, isPrepend bool) ([]byte, error) {
	current := gjson.GetBytes(data, path)
	valueStr := fmt.Sprintf("%v", value)
	var newStr string
	if isPrepend {
		newStr = valueStr + current.String()
	} else {
		newStr = current.String() + valueStr
	}
	return sjson.SetBytes(data, path, newStr)
}

func trimStringValue(data []byte, path string, value any, isPrefix bool) ([]byte, error) {
	current := gjson.GetBytes(data, path)
	if current.Type != gjson.String {
		return data, fmt.Errorf("operation not supported for type: %v", current.Type)
	}

	if value == nil {
		return data, fmt.Errorf("trim value is required")
	}
	valueStr := fmt.Sprintf("%v", value)

	var newStr string
	if isPrefix {
		newStr = strings.TrimPrefix(current.String(), valueStr)
	} else {
		newStr = strings.TrimSuffix(current.String(), valueStr)
	}
	return sjson.SetBytes(data, path, newStr)
}

func ensureStringAffix(data []byte, path string, value any, isPrefix bool) ([]byte, error) {
	current := gjson.GetBytes(data, path)
	if current.Type != gjson.String {
		return data, fmt.Errorf("operation not supported for type: %v", current.Type)
	}

	if value == nil {
		return data, fmt.Errorf("ensure value is required")
	}
	valueStr := fmt.Sprintf("%v", value)
	if valueStr == "" {
		return data, fmt.Errorf("ensure value is required")
	}

	currentStr := current.String()
	if isPrefix {
		if strings.HasPrefix(currentStr, valueStr) {
			return data, nil
		}
		return sjson.SetBytes(data, path, valueStr+currentStr)
	}

	if strings.HasSuffix(currentStr, valueStr) {
		return data, nil
	}
	return sjson.SetBytes(data, path, currentStr+valueStr)
}

func transformStringValue(data []byte, path string, transform func(string) string) ([]byte, error) {
	current := gjson.GetBytes(data, path)
	if current.Type != gjson.String {
		return data, fmt.Errorf("operation not supported for type: %v", current.Type)
	}
	return sjson.SetBytes(data, path, transform(current.String()))
}

func replaceStringValue(data []byte, path, from, to string) ([]byte, error) {
	current := gjson.GetBytes(data, path)
	if current.Type != gjson.String {
		return data, fmt.Errorf("operation not supported for type: %v", current.Type)
	}
	if from == "" {
		return data, fmt.Errorf("replace from is required")
	}
	return sjson.SetBytes(data, path, strings.ReplaceAll(current.String(), from, to))
}

func regexReplaceStringValue(data []byte, path, pattern, replacement string) ([]byte, error) {
	current := gjson.GetBytes(data, path)
	if current.Type != gjson.String {
		return data, fmt.Errorf("operation not supported for type: %v", current.Type)
	}
	if pattern == "" {
		return data, fmt.Errorf("regex pattern is required")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return data, err
	}
	return sjson.SetBytes(data, path, re.ReplaceAllString(current.String(), replacement))
}

type pruneObjectsOptions struct {
	conditions []ConditionOperation
	logic      string
	recursive  bool
}

func pruneObjects(data []byte, path, contextJSON string, value any) ([]byte, error) {
	options, err := parsePruneObjectsOptions(value)
	if err != nil {
		return nil, err
	}

	if path == "" {
		var root any
		if err := common.Unmarshal(data, &root); err != nil {
			return nil, err
		}
		cleaned, _, err := pruneObjectsNode(root, options, contextJSON, true)
		if err != nil {
			return nil, err
		}
		return common.Marshal(cleaned)
	}

	target := gjson.GetBytes(data, path)
	if !target.Exists() {
		return data, nil
	}

	var targetNode any
	if target.Type == gjson.JSON {
		if err := common.UnmarshalJsonStr(target.Raw, &targetNode); err != nil {
			return nil, err
		}
	} else {
		targetNode = target.Value()
	}

	cleaned, _, err := pruneObjectsNode(targetNode, options, contextJSON, true)
	if err != nil {
		return nil, err
	}
	cleanedBytes, err := common.Marshal(cleaned)
	if err != nil {
		return nil, err
	}
	return sjson.SetRawBytes(data, path, cleanedBytes)
}

func parsePruneObjectsOptions(value any) (pruneObjectsOptions, error) {
	opts := pruneObjectsOptions{
		logic:     "AND",
		recursive: true,
	}

	switch raw := value.(type) {
	case nil:
		return opts, fmt.Errorf("prune_objects value is required")
	case string:
		v := strings.TrimSpace(raw)
		if v == "" {
			return opts, fmt.Errorf("prune_objects value is required")
		}
		opts.conditions = []ConditionOperation{
			{
				Path:  "type",
				Mode:  "full",
				Value: v,
			},
		}
	case map[string]any:
		if logic, ok := raw["logic"].(string); ok && strings.TrimSpace(logic) != "" {
			opts.logic = logic
		}
		if recursive, ok := raw["recursive"].(bool); ok {
			opts.recursive = recursive
		}

		if condRaw, exists := raw["conditions"]; exists {
			conditions, err := parseConditionOperations(condRaw)
			if err != nil {
				return opts, err
			}
			opts.conditions = append(opts.conditions, conditions...)
		}

		if whereRaw, exists := raw["where"]; exists {
			whereMap, ok := whereRaw.(map[string]any)
			if !ok {
				return opts, fmt.Errorf("prune_objects where must be object")
			}
			for key, val := range whereMap {
				key = strings.TrimSpace(key)
				if key == "" {
					continue
				}
				opts.conditions = append(opts.conditions, ConditionOperation{
					Path:  key,
					Mode:  "full",
					Value: val,
				})
			}
		}

		if matchType, exists := raw["type"]; exists {
			opts.conditions = append(opts.conditions, ConditionOperation{
				Path:  "type",
				Mode:  "full",
				Value: matchType,
			})
		}
	default:
		return opts, fmt.Errorf("prune_objects value must be string or object")
	}

	if len(opts.conditions) == 0 {
		return opts, fmt.Errorf("prune_objects conditions are required")
	}
	return opts, nil
}

func parseConditionOperations(raw any) ([]ConditionOperation, error) {
	switch typed := raw.(type) {
	case map[string]any:
		entries := lo.Entries(typed)
		conditions := lo.FilterMap(entries, func(item lo.Entry[string, any], _ int) (ConditionOperation, bool) {
			path := strings.TrimSpace(item.Key)
			if path == "" {
				return ConditionOperation{}, false
			}
			return ConditionOperation{
				Path:  path,
				Mode:  "full",
				Value: item.Value,
			}, true
		})
		if len(conditions) == 0 {
			return nil, fmt.Errorf("conditions object must contain at least one key")
		}
		return conditions, nil
	case []any:
		items := typed
		result := make([]ConditionOperation, 0, len(items))
		for _, item := range items {
			itemMap, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("condition must be object")
			}
			path, _ := itemMap["path"].(string)
			mode, _ := itemMap["mode"].(string)
			if strings.TrimSpace(path) == "" || strings.TrimSpace(mode) == "" {
				return nil, fmt.Errorf("condition path/mode is required")
			}
			condition := ConditionOperation{
				Path: path,
				Mode: mode,
			}
			if value, exists := itemMap["value"]; exists {
				condition.Value = value
			}
			if invert, ok := itemMap["invert"].(bool); ok {
				condition.Invert = invert
			}
			if passMissingKey, ok := itemMap["pass_missing_key"].(bool); ok {
				condition.PassMissingKey = passMissingKey
			}
			result = append(result, condition)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("conditions must be an array or object")
	}
}

func pruneObjectsNode(node any, options pruneObjectsOptions, contextJSON string, isRoot bool) (any, bool, error) {
	switch value := node.(type) {
	case []any:
		result := make([]any, 0, len(value))
		for _, item := range value {
			next, drop, err := pruneObjectsNode(item, options, contextJSON, false)
			if err != nil {
				return nil, false, err
			}
			if drop {
				continue
			}
			result = append(result, next)
		}
		return result, false, nil
	case map[string]any:
		shouldDrop, err := shouldPruneObject(value, options, contextJSON)
		if err != nil {
			return nil, false, err
		}
		if shouldDrop && !isRoot {
			return nil, true, nil
		}
		if !options.recursive {
			return value, false, nil
		}
		for key, child := range value {
			next, drop, err := pruneObjectsNode(child, options, contextJSON, false)
			if err != nil {
				return nil, false, err
			}
			if drop {
				delete(value, key)
				continue
			}
			value[key] = next
		}
		return value, false, nil
	default:
		return node, false, nil
	}
}

func shouldPruneObject(node map[string]any, options pruneObjectsOptions, contextJSON string) (bool, error) {
	nodeBytes, err := common.Marshal(node)
	if err != nil {
		return false, err
	}
	return checkConditions(nodeBytes, contextJSON, options.conditions, options.logic)
}

func mergeObjects(data []byte, path string, value any, keepOrigin bool) ([]byte, error) {
	current := gjson.GetBytes(data, path)
	var currentMap, newMap map[string]any

	// 解析当前值（current.Raw 是 data 的子串，避免再分配一份）
	if err := common.UnmarshalJsonStr(current.Raw, &currentMap); err != nil {
		return nil, err
	}
	// 解析新值
	switch v := value.(type) {
	case map[string]any:
		newMap = v
	default:
		jsonBytes, _ := common.Marshal(v)
		if err := common.Unmarshal(jsonBytes, &newMap); err != nil {
			return nil, err
		}
	}
	// 合并
	result := make(map[string]any)
	maps.Copy(result, currentMap)
	for k, v := range newMap {
		if !keepOrigin || result[k] == nil {
			result[k] = v
		}
	}
	return sjson.SetBytes(data, path, result)
}
