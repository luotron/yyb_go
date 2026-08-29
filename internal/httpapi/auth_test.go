package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newAuthTestApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("GIN_MODE", "test")
	app, err := NewApp(Config{
		ResourceRoot:    t.TempDir(),
		RequestTimeout:  time.Second,
		AvatarTimeout:   time.Second,
		SessionTTL:      time.Minute,
		QRSessionTTL:    time.Minute,
		AdminUser:       "admin",
		AdminPassword:   "secret-pass",
		SessionDuration: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

func TestAuthFlow(t *testing.T) {
	app := newAuthTestApp(t)
	handler := app.Handler()

	// 未登录访问受保护 API -> 401
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/accounts", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("未登录 /accounts status = %d，期望 401", recorder.Code)
	}

	// 未登录访问页面 -> 重定向 /login
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/apps", nil))
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("未登录 /apps status = %d，期望 303", recorder.Code)
	}
	if location := recorder.Header().Get("Location"); location != "/login?next=%2Fapps" {
		t.Fatalf("重定向 Location = %q", location)
	}

	// 错误密码 -> 401
	recorder = postJSON(handler, "/login", map[string]any{"username": "admin", "password": "wrong-pass"})
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("错误密码 status = %d", recorder.Code)
	}

	// 正确登录 -> 拿到会话 cookie
	recorder = postJSON(handler, "/login", map[string]any{"username": "admin", "password": "secret-pass", "next": "/apps"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("登录 status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != "yyb_session" {
		t.Fatalf("登录响应未设置会话 cookie: %#v", cookies)
	}
	var loginBody struct {
		Data struct {
			Next string `json:"next"`
			User struct {
				Username string `json:"username"`
				Role     string `json:"role"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &loginBody); err != nil {
		t.Fatal(err)
	}
	if loginBody.Data.User.Role != "admin" || loginBody.Data.Next != "/apps" {
		t.Fatalf("登录响应 = %#v", loginBody.Data)
	}

	// 带 cookie 访问受保护 API -> 200
	request := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	request.AddCookie(cookies[0])
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("登录后 /accounts status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	// /auth/me 返回当前用户
	request = httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	request.AddCookie(cookies[0])
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("/auth/me status = %d", recorder.Code)
	}

	// 登出后 cookie 失效
	request = httptest.NewRequest(http.MethodPost, "/logout", nil)
	request.AddCookie(cookies[0])
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("/logout status = %d", recorder.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/accounts", nil)
	request.AddCookie(cookies[0])
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("登出后 /accounts status = %d，期望 401", recorder.Code)
	}
}

func TestIntegrationTokenBypass(t *testing.T) {
	app := newAuthTestApp(t)
	app.cfg.IntegrationToken = "test-token-123"
	handler := app.Handler()

	// 无 token -> 401
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/accounts", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("无 token status = %d", recorder.Code)
	}

	// 带正确 token -> 200
	request := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	request.Header.Set("X-Integration-Token", "test-token-123")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("带 token status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAuthDisabledWithoutAdminConfig(t *testing.T) {
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
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/accounts", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("未配置管理员时不应启用鉴权，status = %d", recorder.Code)
	}
	if app.authEnabled() {
		t.Fatal("未配置管理员时鉴权应为关闭")
	}
}

func TestLoginRateLimit(t *testing.T) {
	app := newAuthTestApp(t)
	handler := app.Handler()
	for i := 0; i < 8; i++ {
		recorder := postJSON(handler, "/login", map[string]any{"username": "admin", "password": "wrong"})
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("第 %d 次错误密码 status = %d", i+1, recorder.Code)
		}
	}
	recorder := postJSON(handler, "/login", map[string]any{"username": "admin", "password": "secret-pass"})
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("连续失败后 status = %d，期望 429", recorder.Code)
	}
}

func TestLoginPageServed(t *testing.T) {
	app := newAuthTestApp(t)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /login status = %d", recorder.Code)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("登录")) {
		t.Fatal("登录页内容缺失")
	}
}
