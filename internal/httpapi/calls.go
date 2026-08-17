package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"yyb_go/internal/store"
)

type callFeatureDefinition struct {
	Code        int    `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

var callFeatureDefinitions = []callFeatureDefinition{
	{Code: 1001, Name: "getCode", Description: "wx.login code", Enabled: true},
	{Code: 1002, Name: "getPhoneNumber", Description: "获取手机号", Enabled: true},
	{Code: 1003, Name: "operateWxData", Description: "通用云函数代理", Enabled: true},
	{Code: 1004, Name: "getHostSign", Description: "verifyplugin HostSign", Enabled: true},
}

type accountCallRequest struct {
	Feature any            `json:"feature"`
	AppID   string         `json:"app_id"`
	Payload map[string]any `json:"payload"`
}

func (a *App) handleFeatures(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	onlyEnabled := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("only_enabled")), "true")
	features := make([]callFeatureDefinition, 0, len(callFeatureDefinitions))
	for _, feature := range callFeatureDefinitions {
		if onlyEnabled && !feature.Enabled {
			continue
		}
		features = append(features, feature)
	}
	writeJSON(w, http.StatusOK, features)
}

func (a *App) handleAccountCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ref, err := accountCallRef(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body accountCallRequest
	if err = decodeOptionalJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	feature, err := resolveCallFeature(body.Feature)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	body.AppID = strings.TrimSpace(body.AppID)
	if body.AppID == "" {
		writeError(w, http.StatusBadRequest, "app_id is required")
		return
	}
	if (feature.Name == "operateWxData" || feature.Name == "getHostSign") && body.Payload == nil {
		writeError(w, http.StatusBadRequest, "payload is required")
		return
	}
	account, ok := a.resolveAccountRef(w, r, ref)
	if !ok {
		return
	}
	call, err := a.callForFeature(feature.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.invokeWXApp(r.Context(), account, body.AppID, body.Payload, call)
	if err != nil {
		writeWXAppInvocationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"feature": feature.Name,
		"openid":  account.OpenID,
		"result":  result,
	})
}

func accountCallRef(path string) (string, error) {
	const prefix = "/accounts/"
	const suffix = "/call"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", fmt.Errorf("账号调用路径无效")
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if raw == "" || strings.Contains(raw, "/") {
		return "", fmt.Errorf("ref is required")
	}
	ref, err := url.PathUnescape(raw)
	if err != nil || strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("ref is invalid")
	}
	return strings.TrimSpace(ref), nil
}

func resolveCallFeature(value any) (callFeatureDefinition, error) {
	var code int
	var name string
	switch feature := value.(type) {
	case string:
		name = strings.TrimSpace(feature)
		if parsed, err := strconv.Atoi(name); err == nil {
			code = parsed
			name = ""
		}
	case float64:
		if feature == float64(int(feature)) {
			code = int(feature)
		}
	case json.Number:
		parsed, err := strconv.Atoi(feature.String())
		if err == nil {
			code = parsed
		}
	case int:
		code = feature
	case int64:
		code = int(feature)
	}
	for _, candidate := range callFeatureDefinitions {
		if code != 0 && candidate.Code == code {
			return candidate, nil
		}
		if name != "" && strings.EqualFold(candidate.Name, name) {
			return candidate, nil
		}
	}
	return callFeatureDefinition{}, fmt.Errorf("feature 不受支持")
}

func (a *App) callForFeature(name string) (wxappCall, error) {
	switch name {
	case "getCode":
		return a.invokeGetCode, nil
	case "getPhoneNumber":
		return a.invokeGetPhoneNumber, nil
	case "operateWxData":
		return a.invokeOperateWXData, nil
	case "getHostSign":
		return a.invokeGetHostSign, nil
	default:
		return nil, fmt.Errorf("feature 不受支持: %s", name)
	}
}

func (a *App) handleGetHostSign(w http.ResponseWriter, r *http.Request) {
	if !acceptWXAppRoute(w, r, "/wxapp/getHostSign") {
		return
	}
	a.callWXApp(w, r, true, a.invokeGetHostSign)
}

func (a *App) invokeGetHostSign(ctx context.Context, account *store.WechatAccount, appID string, payload map[string]any) (map[string]any, error) {
	return a.pool.GetHostSign(ctx, account.LoginBuffer, appID, payload, account.ID, a.cfg.TCPProxy)
}

func writeWXAppInvocationError(w http.ResponseWriter, err error) {
	var expired accountExpiredError
	if errors.As(err, &expired) {
		writeError(w, http.StatusConflict, "account login_buffer expired (refresh failed); re-scan required")
		return
	}
	writeError(w, http.StatusBadGateway, "call failed: "+err.Error())
}
