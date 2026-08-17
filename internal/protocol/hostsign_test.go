package protocol

import (
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalizeHostSignPayloadSupportsAllInputForms(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
	}{
		{
			name: "provider 简写",
			payload: map[string]any{
				"provider":      "wx63af045606be281d",
				"inner_version": float64(32),
			},
		},
		{
			name: "plugins 数组",
			payload: map[string]any{
				"plugins": []any{map[string]any{"provider": "wx63af045606be281d", "inner_version": 32}},
			},
		},
		{
			name: "原始 data 字符串",
			payload: map[string]any{
				"data": `{"plugins":[{"provider":"wx63af045606be281d","inner_version":32}]}`,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := normalizeHostSignPayload(test.payload)
			if err != nil {
				t.Fatalf("normalizeHostSignPayload() error = %v", err)
			}
			var decoded map[string]any
			if err = json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("解析归一化 JSON 失败: %v", err)
			}
			plugins, ok := decoded["plugins"].([]any)
			if !ok || len(plugins) != 1 {
				t.Fatalf("plugins = %#v", decoded["plugins"])
			}
		})
	}
}

func TestBuildHostSignRequestUsesVerifyPluginProtocol(t *testing.T) {
	payload := map[string]any{"provider": "wx63af045606be281d", "inner_version": 32}
	plain, err := buildHostSignRequestWithSessionDevice(25984988026127932, "wxdbb4c5f1b8ee7da1", payload, []byte("AA-BB-CC-DD-EE-FF"))
	if err != nil {
		t.Fatalf("buildHostSignRequestWithSessionDevice() error = %v", err)
	}

	outer := pbParse(plain)
	if got := string(safeBytes(outer[2])); got != string(verifyPluginURL) {
		t.Fatalf("VerifyPlugin URL = %q", got)
	}
	if got := int64FromAny(outer[7]); got != verifyPluginCmdID {
		t.Fatalf("VerifyPlugin cmdID = %#v", outer[7])
	}
	body := pbParse(safeBytes(outer[5]))
	if got := string(safeBytes(body[2])); got != "wxdbb4c5f1b8ee7da1" {
		t.Fatalf("请求 appID = %q", got)
	}
	var requestJSON map[string]any
	if err = json.Unmarshal(safeBytes(body[3]), &requestJSON); err != nil {
		t.Fatalf("解析 VerifyPlugin 请求 JSON 失败: %v", err)
	}
	plugins, ok := requestJSON["plugins"].([]any)
	if !ok || len(plugins) != 1 {
		t.Fatalf("VerifyPlugin plugins = %#v", requestJSON["plugins"])
	}
}

func TestCapturedVerifyPluginRequestHasExpectedFields(t *testing.T) {
	fixtureHex := "0a2f0a010010a496f0cd051a104139353433643232366362333966300020b7ae80c0022a0a616e64726f69642d3332300012127778616633353030393637356161306232611ade017b22706c7567696e73223a5b7b2270726f7669646572223a22777830653665643466353164623964303738222c22696e6e65725f76657273696f6e223a31327d2c7b2270726f7669646572223a22777832623736333164393633316536653339222c22696e6e65725f76657273696f6e223a357d2c7b2270726f7669646572223a22777835353835333539346262613436336161222c22696e6e65725f76657273696f6e223a337d2c7b2270726f7669646572223a22777839646232633136643036333363326537222c22696e6e65725f76657273696f6e223a327d5d7d"
	fixture, err := hex.DecodeString(fixtureHex)
	if err != nil {
		t.Fatalf("解析抓包 fixture 失败: %v", err)
	}
	fields := pbParse(fixture)
	if got := string(safeBytes(fields[2])); got != "wxaf35009675aa0b2a" {
		t.Fatalf("fixture appID = %q", got)
	}
	var payload map[string]any
	if err = json.Unmarshal(safeBytes(fields[3]), &payload); err != nil {
		t.Fatalf("解析 fixture payload 失败: %v", err)
	}
	plugins, ok := payload["plugins"].([]any)
	if !ok || len(plugins) != 4 {
		t.Fatalf("fixture plugins = %#v", payload["plugins"])
	}
}

func TestParseHostSignResponse(t *testing.T) {
	rawJSON := []byte(`{"list":[{"plugin_id":"wxc3b909c3d24c5417","host_sign":"0123456789abcdef0123456789abcdef01234567","noncestr":"0123456789abcdef0123456789abcdef","timestamp":1785267185}]}`)
	response := make([]byte, 0, len(rawJSON)+32)
	response = append(response, pbLen(1, []byte{0x0a, 0x00, 0x10, 0x00, 0x18, 0x00})...)
	response = append(response, pbLen(2, rawJSON)...)
	response = append(response, pbLen(3, nil)...)

	result, err := parseHostSignResponse(nil, response)
	if err != nil {
		t.Fatalf("parseHostSignResponse() error = %v", err)
	}
	if result["success"] != true {
		t.Fatalf("success = %#v", result["success"])
	}
	plugins, ok := result["plugins"].([]any)
	if !ok || len(plugins) != 1 {
		t.Fatalf("plugins = %#v", result["plugins"])
	}
	if !reflect.DeepEqual(result["plugins"], result["list"]) {
		t.Fatalf("plugins 与 list 不一致")
	}
	rawFields, ok := result["raw_fields"].(map[string]any)
	if !ok || rawFields["1"] != "bytes[6]" || rawFields["2"] != "bytes[169]" || rawFields["3"] != "bytes[0]" {
		t.Fatalf("raw_fields = %#v", result["raw_fields"])
	}
}
