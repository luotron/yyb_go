package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveCallFeatureSupportsCodeAndName(t *testing.T) {
	for _, value := range []any{1004, float64(1004), "1004", "getHostSign", "GETHOSTSIGN"} {
		feature, err := resolveCallFeature(value)
		if err != nil {
			t.Fatalf("resolveCallFeature(%#v) error = %v", value, err)
		}
		if feature.Code != 1004 || feature.Name != "getHostSign" {
			t.Fatalf("resolveCallFeature(%#v) = %#v", value, feature)
		}
	}
}

func TestAccountCallRef(t *testing.T) {
	ref, err := accountCallRef("/accounts/owNAX6p7HScULSX4kzhwnALiH1tk/call")
	if err != nil {
		t.Fatalf("accountCallRef() error = %v", err)
	}
	if ref != "owNAX6p7HScULSX4kzhwnALiH1tk" {
		t.Fatalf("accountCallRef() = %q", ref)
	}
	if _, err = accountCallRef("/accounts//call"); err == nil {
		t.Fatalf("空 ref 未返回错误")
	}
}

func TestHandleFeaturesContainsHostSign(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/features?only_enabled=true", nil)
	new(App).handleFeatures(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /features status = %d", response.Code)
	}
	var body struct {
		Code int                     `json:"code"`
		Data []callFeatureDefinition `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析 /features 响应失败: %v", err)
	}
	if body.Code != 0 || len(body.Data) != 4 {
		t.Fatalf("GET /features body = %#v", body)
	}
	if body.Data[3].Code != 1004 || body.Data[3].Name != "getHostSign" {
		t.Fatalf("HostSign feature = %#v", body.Data[3])
	}
}

func TestHandleActivitySignMatchesHAR(t *testing.T) {
	payload := []byte(`{"sign_key":"1154b7aa0d768d7bde8714d9b43065c9","sign_timestamp":"1785266267037","sign_nonce":"efbe1accbef45a1e"}`)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/activity/sign", bytes.NewReader(payload))
	new(App).handleActivitySign(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("POST /activity/sign status = %d, body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			SignTimestamp string `json:"signTimestamp"`
			SignNonce     string `json:"signNonce"`
			Sign          string `json:"sign"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析活动签名响应失败: %v", err)
	}
	const expected = "3cb98d55ba34030da6b225c836c16f6783cd35cda505475907d7cfaa21015577"
	if body.Code != 0 || body.Data.Sign != expected {
		t.Fatalf("POST /activity/sign body = %#v", body)
	}
}
