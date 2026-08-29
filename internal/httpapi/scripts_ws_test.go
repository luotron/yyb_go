package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestScriptLogsWebSocket(t *testing.T) {
	if _, err := exec.LookPath("python"); err != nil {
		t.Skip("python 不可用，跳过 WebSocket 测试")
	}
	app := newScriptsTestApp(t)
	server := httptest.NewServer(app.Handler())
	defer server.Close()

	if recorder := uploadScript(t, app, "echo.py", "import time\ntime.sleep(1)\nprint('ws-chunk-1')\ntime.sleep(0.4)\nprint('ws-chunk-2')\n", false); recorder.Code != 200 {
		t.Fatalf("upload status = %d", recorder.Code)
	}
	if recorder := postJSON(server.Config.Handler, "/scripts/echo.py/run", nil); recorder.Code != 200 {
		t.Fatalf("run status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/scripts/echo.py/logs/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	var logData strings.Builder
	statuses := 0
	var chunk1At, chunk2At time.Time
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			break // 服务端在结束时关闭连接
		}
		var message struct {
			Type      string `json:"type"`
			Content   string `json:"content"`
			Data      string `json:"data"`
			Running   bool   `json:"running"`
			ExitCode  *int   `json:"exit_code"`
			LastError string `json:"last_error"`
		}
		if err := json.Unmarshal(data, &message); err != nil {
			t.Fatalf("decode ws message: %v", err)
		}
		switch message.Type {
		case "log":
			logData.WriteString(message.Data)
			if strings.Contains(message.Data, "ws-chunk-1") && chunk1At.IsZero() {
				chunk1At = time.Now()
			}
			if strings.Contains(message.Data, "ws-chunk-2") && chunk2At.IsZero() {
				chunk2At = time.Now()
			}
		case "status":
			statuses++
			if !message.Running && message.ExitCode != nil && *message.ExitCode != 0 {
				t.Fatalf("exit code = %d, last_error=%s", *message.ExitCode, message.LastError)
			}
		}
	}
	if !strings.Contains(logData.String(), "ws-chunk-1") || !strings.Contains(logData.String(), "ws-chunk-2") {
		t.Fatalf("实时增量日志缺失: %q", logData.String())
	}
	if chunk1At.IsZero() || chunk2At.IsZero() {
		t.Fatal("未观测到分块到达时间")
	}
	if gap := chunk2At.Sub(chunk1At); gap < 150*time.Millisecond {
		t.Fatalf("两个输出块间隔 %v，疑似整段缓冲到进程结束后才推送（应为实时，脚本内 sleep 400ms）", gap)
	}
	if statuses == 0 {
		t.Fatal("未收到 status 消息")
	}
}
