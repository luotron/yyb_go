package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"yyb_go/internal/scriptrunner"
)

// handleScriptLogsWS 通过 WebSocket 实时推送脚本日志。
// 协议：服务端发送 JSON 文本帧
//
//	{"type":"init","content":"..."}   当前日志全文（客户端整体替换）
//	{"type":"log","data":"..."}       增量日志（追加）
//	{"type":"status","running":...,"started_at":...,"finished_at":...,"exit_code":...,"last_error":...}
//
// 脚本结束时发送最终 status 后由服务端关闭连接。
func (a *App) handleScriptLogsWS(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/scripts/"), "/logs/ws")
	if !scriptrunner.ValidName(name) {
		writeError(w, http.StatusBadRequest, "脚本名不合法: "+name)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	// 读循环仅用于感知客户端断开。
	go func() {
		for {
			if _, _, readErr := conn.Read(ctx); readErr != nil {
				cancel()
				return
			}
		}
	}()

	send := func(payload map[string]any) bool {
		data, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
		defer writeCancel()
		return conn.Write(writeCtx, websocket.MessageText, data) == nil
	}
	statusPayload := func(script scriptrunner.Script) map[string]any {
		return map[string]any{
			"type":        "status",
			"running":     script.Running,
			"started_at":  script.StartedAt,
			"finished_at": script.FinishedAt,
			"exit_code":   script.ExitCode,
			"last_error":  script.LastError,
		}
	}

	content, logs, done, unsubscribe, err := a.scripts.Subscribe(name)
	if err != nil {
		_ = send(map[string]any{"type": "error", "message": err.Error()})
		return
	}
	defer unsubscribe()

	script, _ := a.scripts.Status(name)
	if !send(map[string]any{"type": "init", "content": string(content)}) {
		return
	}
	if !send(statusPayload(script)) {
		return
	}
	if done == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			script, _ := a.scripts.Status(name)
			_ = send(statusPayload(script))
			return
		case chunk, ok := <-logs:
			if !ok {
				return
			}
			if !send(map[string]any{"type": "log", "data": string(chunk)}) {
				return
			}
		}
	}
}
