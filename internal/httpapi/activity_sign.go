package httpapi

import (
	"net/http"
	"strings"

	activitycrypto "yyb_go/internal/crypto"
)

type activitySignRequest struct {
	SignKey       string `json:"sign_key"`
	Token         string `json:"token"`
	SignTimestamp string `json:"sign_timestamp"`
	SignNonce     string `json:"sign_nonce"`
}

func (a *App) handleActivitySign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body activitySignRequest
	if err := decodeOptionalJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	body.SignKey = strings.TrimSpace(body.SignKey)
	body.SignTimestamp = strings.TrimSpace(body.SignTimestamp)
	body.SignNonce = strings.TrimSpace(body.SignNonce)
	var (
		result activitycrypto.ActivitySignature
		err    error
	)
	if body.SignTimestamp != "" || body.SignNonce != "" {
		if body.SignTimestamp == "" || body.SignNonce == "" {
			writeError(w, http.StatusBadRequest, "sign_timestamp 与 sign_nonce 必须同时提供")
			return
		}
		result, err = activitycrypto.BuildActivitySignature(body.SignKey, body.SignTimestamp, body.SignNonce)
	} else {
		result, err = activitycrypto.GenerateActivitySignature(body.SignKey, body.Token)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
