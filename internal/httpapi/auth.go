package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const sessionCookie = "yyb_session"
const secretKeyHeader = "X-Secret-Key"

type loginAttempt struct {
	Failures int
	Start    time.Time
}

type sessionUser struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Enabled     bool   `json:"enabled"`
}

type adminSession struct {
	Username  string
	ExpiresAt time.Time
}

type authContextKey string

const authUserKey authContextKey = "user"

// authEnabled service.json 配置了 secret_key 时启用访问控制。
func (a *App) authEnabled() bool {
	return a.cfg.SecretKey != ""
}

func (a *App) createSession(username string) string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		raw = []byte(strings.Repeat("x", 32))
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	a.sessionMu.Lock()
	a.sessions[token] = adminSession{Username: username, ExpiresAt: time.Now().Add(a.cfg.SessionDuration)}
	a.sessionMu.Unlock()
	return token
}

func (a *App) userBySessionToken(token string) *sessionUser {
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()
	session, ok := a.sessions[token]
	if !ok || time.Now().After(session.ExpiresAt) {
		delete(a.sessions, token)
		return nil
	}
	return &sessionUser{
		Username: session.Username, DisplayName: session.Username, Role: "admin", Enabled: true,
	}
}

func (a *App) deleteSessionToken(token string) {
	a.sessionMu.Lock()
	delete(a.sessions, token)
	a.sessionMu.Unlock()
}

// requireBrowserSession 拦截未验证请求：会话 Cookie 或 X-Secret-Key 请求头任一有效即放行。
// 未通过时：API 路径返回 401 JSON，页面路径重定向到 /login。
func (a *App) requireBrowserSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.authEnabled() {
			c.Next()
			return
		}
		// API 调用：请求头携带 secret key 即放行（脚本 SDK 自动附带）
		if header := strings.TrimSpace(c.GetHeader(secretKeyHeader)); header != "" &&
			subtle.ConstantTimeCompare([]byte(header), []byte(a.cfg.SecretKey)) == 1 {
			ctx := context.WithValue(c.Request.Context(), authUserKey,
				&sessionUser{Username: "secret-key", DisplayName: "API 调用", Role: "admin", Enabled: true})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
			return
		}
		// 网页：会话 Cookie 有效即放行
		token, err := c.Cookie(sessionCookie)
		if err == nil {
			if user := a.userBySessionToken(token); user != nil {
				ctx := context.WithValue(c.Request.Context(), authUserKey, user)
				c.Request = c.Request.WithContext(ctx)
				c.Next()
				return
			}
		}
		clearSessionCookie(c.Writer, a.cfg.CookieSecure)
		if isAPIRequest(c.Request.URL.Path) {
			writeError(c.Writer, http.StatusUnauthorized, "请先登录")
			c.Abort()
			return
		}
		next := c.Request.URL.RequestURI()
		http.Redirect(c.Writer, c.Request, "/login?next="+url.QueryEscape(next), http.StatusSeeOther)
		c.Abort()
	}
}

func isAPIRequest(path string) bool {
	for _, prefix := range []string{"/accounts", "/wxapp", "/scripts", "/qr", "/activity", "/features", "/auth/me"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !a.authEnabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodGet {
		if cookie, err := r.Cookie(sessionCookie); err == nil {
			if a.userBySessionToken(cookie.Value) != nil {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
		}
		serveFileOrText(w, r, filepath.Join(a.resources.Templates, "login.html"), fallbackLoginHTML)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		SecretKey string `json:"secret_key"`
		Next      string `json:"next"`
	}
	if err := decodeOptionalJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	key := clientIP(r)
	if !a.allowLogin(key) {
		writeError(w, http.StatusTooManyRequests, "验证失败次数过多，请 15 分钟后重试")
		return
	}
	secretOK := subtle.ConstantTimeCompare([]byte(body.SecretKey), []byte(a.cfg.SecretKey)) == 1
	if body.SecretKey == "" || !secretOK {
		a.recordLoginFailure(key)
		writeError(w, http.StatusUnauthorized, "secret key 错误")
		return
	}
	a.clearLoginFailures(key)
	token := a.createSession("admin")
	setSessionCookie(w, token, a.cfg.CookieSecure, a.cfg.SessionDuration)
	next := safeNext(body.Next)
	writeJSON(w, http.StatusOK, map[string]any{
		"user": &sessionUser{Username: "admin", DisplayName: "管理员", Role: "admin", Enabled: true},
		"next": next,
	})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		a.deleteSessionToken(cookie.Value)
	}
	clearSessionCookie(w, a.cfg.CookieSecure)
	writeJSON(w, http.StatusOK, map[string]any{"logged_out": true})
}

func (a *App) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.authEnabled() {
		writeJSON(w, http.StatusOK, map[string]any{
			"auth_enabled": false,
			"user": map[string]any{
				"username": "local", "display_name": "本机管理员", "role": "admin", "enabled": true,
			},
		})
		return
	}
	user := currentAuthUser(r)
	if user == nil {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"auth_enabled": true, "user": user})
}

func currentAuthUser(r *http.Request) *sessionUser {
	user, _ := r.Context().Value(authUserKey).(*sessionUser)
	return user
}

func setSessionCookie(w http.ResponseWriter, token string, secure bool, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

func clientIP(r *http.Request) string {
	if value := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); value != "" {
		return value
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (a *App) allowLogin(key string) bool {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	attempt, ok := a.loginAttempts[key]
	if !ok || time.Since(attempt.Start) >= 15*time.Minute {
		delete(a.loginAttempts, key)
		return true
	}
	return attempt.Failures < 8
}

func (a *App) recordLoginFailure(key string) {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	attempt := a.loginAttempts[key]
	if attempt.Start.IsZero() || time.Since(attempt.Start) >= 15*time.Minute {
		attempt = loginAttempt{Start: time.Now()}
	}
	attempt.Failures++
	a.loginAttempts[key] = attempt
}

func (a *App) clearLoginFailures(key string) {
	a.loginMu.Lock()
	delete(a.loginAttempts, key)
	a.loginMu.Unlock()
}

func safeNext(value string) string {
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return value
	}
	return "/"
}
