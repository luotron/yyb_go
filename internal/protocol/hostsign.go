package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const verifyPluginCmdID = 1714

var verifyPluginURL = []byte("/cgi-bin/mmbiz-bin/wxaapp/verifyplugin")

func buildHostSignRequest(uin int64, appID string, payload map[string]any) ([]byte, error) {
	return buildHostSignRequestWithSessionDevice(uin, appID, payload, randomSessDevice())
}

func buildHostSignRequestWithSessionDevice(uin int64, appID string, payload map[string]any, sessDevice []byte) ([]byte, error) {
	requestJSON, err := normalizeHostSignPayload(payload)
	if err != nil {
		return nil, err
	}
	aid := []byte(appID)
	body := make([]byte, 0, 160+len(requestJSON))
	body = append(body, pbLen(1, sessionInfo(uint32(uin), unifiedPCWindows, sessDevice))...)
	body = append(body, pbLen(2, aid)...)
	body = append(body, pbLen(3, requestJSON)...)
	return buildJSAPIPlaintext(uin, appID, verifyPluginURL, verifyPluginCmdID, body, nil, sessDevice), nil
}

func normalizeHostSignPayload(payload map[string]any) ([]byte, error) {
	if payload == nil {
		return nil, fmt.Errorf("getHostSign payload 不能为空")
	}
	if rawData, exists := payload["data"]; exists {
		switch value := rawData.(type) {
		case string:
			value = strings.TrimSpace(value)
			if value == "" || !json.Valid([]byte(value)) {
				return nil, fmt.Errorf("getHostSign data 必须是有效 JSON 字符串")
			}
			return []byte(value), nil
		case map[string]any:
			return json.Marshal(value)
		case json.RawMessage:
			if !json.Valid(value) {
				return nil, fmt.Errorf("getHostSign data 必须是有效 JSON")
			}
			return append([]byte(nil), value...), nil
		default:
			return nil, fmt.Errorf("getHostSign data 仅支持 JSON 字符串或对象")
		}
	}

	provider := strings.TrimSpace(stringAny(payload["provider"]))
	if provider != "" {
		innerVersion, exists := payload["inner_version"]
		if !exists {
			return nil, fmt.Errorf("provider 简写必须提供 inner_version")
		}
		version, err := normalizeInnerVersion(innerVersion)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{
			"plugins": []map[string]any{{
				"provider":      provider,
				"inner_version": version,
			}},
		})
	}

	plugins, exists := payload["plugins"]
	if !exists || plugins == nil {
		return nil, fmt.Errorf("getHostSign payload 需要 provider、plugins 或 data")
	}
	return json.Marshal(payload)
}

func normalizeInnerVersion(value any) (int64, error) {
	switch number := value.(type) {
	case int:
		return int64(number), nil
	case int32:
		return int64(number), nil
	case int64:
		return number, nil
	case uint:
		return int64(number), nil
	case uint32:
		return int64(number), nil
	case uint64:
		if number > math.MaxInt64 {
			return 0, fmt.Errorf("inner_version 超出 int64 范围")
		}
		return int64(number), nil
	case float64:
		if math.Trunc(number) != number || number < 0 || number > math.MaxInt64 {
			return 0, fmt.Errorf("inner_version 必须是非负整数")
		}
		return int64(number), nil
	case json.Number:
		parsed, err := number.Int64()
		if err != nil || parsed < 0 {
			return 0, fmt.Errorf("inner_version 必须是非负整数")
		}
		return parsed, nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(number), 10, 64)
		if err != nil || parsed < 0 {
			return 0, fmt.Errorf("inner_version 必须是非负整数")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("inner_version 必须是非负整数")
	}
}

func parseHostSignResponse(codeOrJSON, response []byte) (map[string]any, error) {
	for _, candidate := range [][]byte{codeOrJSON, response} {
		if result, ok := findHostSignResponse(candidate, 0, nil); ok {
			return result, nil
		}
	}
	return nil, fmt.Errorf("VerifyPlugin 响应中未找到 HostSign JSON")
}

func findHostSignResponse(candidate []byte, depth int, parentFields map[string]any) (map[string]any, bool) {
	if depth > 5 {
		return nil, false
	}
	if len(candidate) == 0 {
		return nil, false
	}
	trimmed := bytes.TrimSpace(candidate)
	if json.Valid(trimmed) {
		result, err := decodeHostSignJSON(trimmed, parentFields)
		if err == nil {
			return result, true
		}
	}
	fields := pbParse(candidate)
	if len(fields) == 0 {
		return nil, false
	}
	described := describeProtobufFields(fields)
	keys := make([]int, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	for _, key := range keys {
		value, ok := fields[key].([]byte)
		if !ok || len(value) == 0 {
			continue
		}
		if result, found := findHostSignResponse(value, depth+1, described); found {
			return result, true
		}
	}
	return nil, false
}

func decodeHostSignJSON(raw []byte, rawFields map[string]any) (map[string]any, error) {
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("解析 VerifyPlugin JSON 失败: %w", err)
	}
	list, ok := decoded["list"].([]any)
	if !ok {
		list, ok = decoded["plugins"].([]any)
	}
	if !ok {
		return nil, fmt.Errorf("VerifyPlugin JSON 缺少 list")
	}
	return map[string]any{
		"success":    true,
		"plugins":    list,
		"raw_json":   string(raw),
		"raw_fields": rawFields,
		"list":       list,
	}, nil
}

func describeProtobufFields(fields map[int]any) map[string]any {
	keys := make([]int, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		name := strconv.Itoa(key)
		switch value := fields[key].(type) {
		case []byte:
			out[name] = fmt.Sprintf("bytes[%d]", len(value))
		case uint64:
			out[name] = fmt.Sprintf("varint[%d]", value)
		case int64:
			out[name] = fmt.Sprintf("varint[%d]", value)
		default:
			out[name] = fmt.Sprintf("%T", value)
		}
	}
	return out
}
