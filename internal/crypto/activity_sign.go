package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const activityNonceAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

type ActivitySignature struct {
	SignTimestamp string `json:"signTimestamp"`
	SignNonce     string `json:"signNonce"`
	Sign          string `json:"sign"`
}

func GenerateActivitySignature(signKey, token string) (ActivitySignature, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	seed, err := randomBase36(16)
	if err != nil {
		return ActivitySignature{}, err
	}
	nonce := deriveActivityNonce(timestamp, token, seed)
	return BuildActivitySignature(signKey, timestamp, nonce)
}

func BuildActivitySignature(signKey, timestamp, nonce string) (ActivitySignature, error) {
	signKey = strings.TrimSpace(signKey)
	timestamp = strings.TrimSpace(timestamp)
	nonce = strings.TrimSpace(nonce)
	if signKey == "" {
		return ActivitySignature{}, fmt.Errorf("sign_key 不能为空")
	}
	if timestamp == "" {
		return ActivitySignature{}, fmt.Errorf("signTimestamp 不能为空")
	}
	if nonce == "" {
		return ActivitySignature{}, fmt.Errorf("signNonce 不能为空")
	}
	raw := "timestamp=" + timestamp + "&nonce=" + nonce
	return ActivitySignature{
		SignTimestamp: timestamp,
		SignNonce:     nonce,
		Sign:          hmacSHA256Hex([]byte(signKey), []byte(raw)),
	}, nil
}

func deriveActivityNonce(timestamp, token, seed string) string {
	digest := hmacSHA256Hex([]byte(timestamp), []byte(seed+token))
	return digest[:16]
}

func hmacSHA256Hex(key, message []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}

func randomBase36(length int) (string, error) {
	if length < 1 {
		return "", fmt.Errorf("随机串长度必须大于 0")
	}
	output := make([]byte, 0, length)
	buffer := make([]byte, length*2)
	for len(output) < length {
		if _, err := rand.Read(buffer); err != nil {
			return "", fmt.Errorf("生成活动签名随机串失败: %w", err)
		}
		for _, value := range buffer {
			if value >= 252 {
				continue
			}
			output = append(output, activityNonceAlphabet[int(value)%len(activityNonceAlphabet)])
			if len(output) == length {
				break
			}
		}
	}
	return string(output), nil
}
