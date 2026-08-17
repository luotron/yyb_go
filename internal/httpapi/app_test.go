package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestHandlerServesGinRoutesAndSwaggerDocs(t *testing.T) {
	t.Setenv("GIN_MODE", "test")

	app, err := NewApp(Config{
		ResourceRoot:   t.TempDir(),
		RequestTimeout: time.Second,
		AvatarTimeout:  time.Second,
		SessionTTL:     time.Minute,
		QRSessionTTL:   time.Minute,
	})
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}
	defer app.Close()

	handler := app.Handler()

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("GET /health status = %d", health.Code)
	}
	var healthBody struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(health.Body.Bytes(), &healthBody); err != nil {
		t.Fatalf("decode health JSON: %v", err)
	}
	if healthBody.Code != 0 || healthBody.Msg != "success" || healthBody.Data["ok"] != true {
		t.Fatalf("GET /health body = %#v", healthBody)
	}

	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("GET /ready status = %d，期望 200", ready.Code)
	}
	var readyBody struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(ready.Body.Bytes(), &readyBody); err != nil {
		t.Fatalf("解析就绪响应失败: %v", err)
	}
	if readyBody.Code != 0 || readyBody.Data["ready"] != true {
		t.Fatalf("GET /ready body = %#v", readyBody)
	}

	openapi := httptest.NewRecorder()
	handler.ServeHTTP(openapi, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if openapi.Code != http.StatusOK {
		t.Fatalf("GET /openapi.json status = %d", openapi.Code)
	}
	var spec map[string]any
	if err := json.Unmarshal(openapi.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode OpenAPI JSON: %v", err)
	}
	if spec["openapi"] != "3.0.3" {
		t.Fatalf("openapi version = %v", spec["openapi"])
	}
	if _, ok := spec["code"]; ok {
		t.Fatalf("OpenAPI JSON should not be wrapped in API envelope")
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI paths missing or invalid")
	}
	if _, exists := paths["/ready"]; !exists {
		t.Fatalf("OpenAPI 缺少 /ready")
	}
	for _, path := range []string{"/wxapp/getCode", "/wxapp/getPhoneNumber", "/wxapp/operateWxData", "/wxapp/getHostSign", "/accounts/{ref}/call", "/features", "/activity/sign", "/accounts/avatar"} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("OpenAPI path %s missing", path)
		}
	}
	for _, path := range []string{"/wxapp/getCode", "/accounts", "/qr"} {
		pathItem := paths[path].(map[string]any)
		method := "get"
		if path == "/wxapp/getCode" || path == "/qr" {
			method = "post"
		}
		operation := pathItem[method].(map[string]any)
		parameters, _ := operation["parameters"].([]any)
		for _, rawParameter := range parameters {
			parameter, _ := rawParameter.(map[string]any)
			if parameter["in"] == "header" && parameter["name"] == "cardKey" {
				t.Fatalf("OpenAPI 路径 %s 仍包含 cardKey", path)
			}
		}
	}
	for _, path := range []string{"/wxapp/getCode", "/wxapp/getPhoneNumber", "/wxapp/operateWxData", "/wxapp/getHostSign"} {
		pathItem := paths[path].(map[string]any)
		post := pathItem["post"].(map[string]any)
		tags := post["tags"].([]any)
		if len(tags) != 1 || tags[0] != "wxapp" {
			t.Fatalf("OpenAPI path %s tags = %#v, want [wxapp]", path, tags)
		}
	}
	for _, path := range []string{"/accounts/{ref}", "/accounts/{ref}/getCode", "/accounts/{ref}/getPhoneNumber", "/accounts/{ref}/operateWxData", "/accounts/getCode", "/accounts/getPhoneNumber", "/accounts/operateWxData"} {
		if _, ok := paths[path]; ok {
			t.Fatalf("OpenAPI still exposes old account feature route %s", path)
		}
	}
	docs := httptest.NewRecorder()
	handler.ServeHTTP(docs, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if docs.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /docs status = %d", docs.Code)
	}
	if got := docs.Header().Get("Location"); got != "/docs/index.html" {
		t.Fatalf("GET /docs Location = %q", got)
	}

	features := httptest.NewRecorder()
	handler.ServeHTTP(features, httptest.NewRequest(http.MethodGet, "/features", nil))
	if features.Code != http.StatusOK {
		t.Fatalf("GET /features status = %d，期望 200", features.Code)
	}
	var featuresBody struct {
		Code int                     `json:"code"`
		Msg  string                  `json:"msg"`
		Data []callFeatureDefinition `json:"data"`
	}
	if err := json.Unmarshal(features.Body.Bytes(), &featuresBody); err != nil {
		t.Fatalf("decode /features JSON: %v", err)
	}
	if featuresBody.Code != 0 || featuresBody.Msg != "success" || len(featuresBody.Data) != len(callFeatureDefinitions) {
		t.Fatalf("GET /features body = %#v", featuresBody)
	}

	oldPath := httptest.NewRecorder()
	handler.ServeHTTP(oldPath, httptest.NewRequest(http.MethodPost, "/accounts/getCode", nil))
	if oldPath.Code != http.StatusNotFound {
		t.Fatalf("POST old account feature route status = %d", oldPath.Code)
	}
}

func TestBusinessRoutesNeedNoCardHeader(t *testing.T) {
	t.Setenv("GIN_MODE", "test")
	app, err := NewApp(Config{
		ResourceRoot:   t.TempDir(),
		RequestTimeout: time.Second,
		AvatarTimeout:  time.Second,
		SessionTTL:     time.Minute,
		QRSessionTTL:   time.Minute,
	})
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}
	defer app.Close()

	handler := app.Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/accounts", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("无卡密请求 /accounts 状态=%d，期望 200", response.Code)
	}
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data []any  `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析账号列表响应失败: %v", err)
	}
	if body.Code != 0 || body.Msg != "success" {
		t.Fatalf("GET /accounts body = %#v", body)
	}
}

func TestLocalAvatarCandidatesSupportWindowsPathsOnLinux(t *testing.T) {
	app := &App{resources: resources{Avatars: filepath.Join("/app", "data", "avatars")}}
	candidates := app.localAvatarCandidates(`resource\avatars\avatar.jpg`)
	if len(candidates) != 1 {
		t.Fatalf("头像候选数量=%d，期望 1", len(candidates))
	}
	want := filepath.Join("/app", "data", "avatars", "avatar.jpg")
	if candidates[0] != want {
		t.Fatalf("Linux 头像候选=%q，期望 %q", candidates[0], want)
	}
}

func TestLocalOpenAPIExcludesAggregatorRoutes(t *testing.T) {
	spec := newOpenAPISpec()
	paths := spec["paths"].(map[string]any)
	for _, path := range []string{
		"/aggregator/accounts",
		"/aggregator/sync",
		"/aggregator/api/getcode",
		"/client/accounts/acquire",
		"/client/api/getcode",
	} {
		if _, exists := paths[path]; exists {
			t.Fatalf("本地 OpenAPI 仍包含聚合路径 %s", path)
		}
	}
}
