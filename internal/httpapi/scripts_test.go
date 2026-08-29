package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func newScriptsTestApp(t *testing.T) *App {
	t.Helper()
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
	t.Cleanup(func() { _ = app.Close() })
	return app
}

func postJSON(handler http.Handler, path string, body map[string]any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestScriptListEmpty(t *testing.T) {
	app := newScriptsTestApp(t)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/scripts", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /scripts status = %d", recorder.Code)
	}
	var body struct {
		Data struct {
			Scripts   []any  `json:"scripts"`
			ServerURL string `json:"server_url"`
			PythonOK  bool   `json:"python_ok"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data.Scripts) != 0 {
		t.Fatalf("scripts = %#v", body.Data.Scripts)
	}
}

func uploadScript(t *testing.T, app *App, name, content string, overwrite bool) *httptest.ResponseRecorder {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := "/scripts/upload"
	if overwrite {
		path += "?overwrite=1"
	}
	request := httptest.NewRequest(http.MethodPost, path, &buffer)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestScriptUploadListScheduleDelete(t *testing.T) {
	app := newScriptsTestApp(t)
	handler := app.Handler()

	if recorder := uploadScript(t, app, "task.py", "print('hi')", false); recorder.Code != http.StatusOK {
		t.Fatalf("upload status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder := uploadScript(t, app, "task.py", "print('hi')", false); recorder.Code != http.StatusConflict {
		t.Fatalf("重复上传 status = %d", recorder.Code)
	}
	if recorder := uploadScript(t, app, "evil.txt", "x", false); recorder.Code != http.StatusBadRequest {
		t.Fatalf("非 py 上传 status = %d", recorder.Code)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/scripts", nil))
	var list struct {
		Data struct {
			Scripts []map[string]any `json:"scripts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data.Scripts) != 1 || list.Data.Scripts[0]["name"] != "task.py" {
		t.Fatalf("scripts = %#v", list.Data.Scripts)
	}

	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/scripts/task.py/schedule", bytes.NewReader([]byte(`{"cron":"32 16 * * *"}`)))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT schedule status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var scheduled struct {
		Data struct {
			Schedule  string `json:"schedule"`
			NextRunAt int64  `json:"next_run_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &scheduled); err != nil {
		t.Fatal(err)
	}
	if scheduled.Data.Schedule != "32 16 * * *" || scheduled.Data.NextRunAt == 0 {
		t.Fatalf("schedule response = %#v", scheduled.Data)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/scripts/task.py/schedule", bytes.NewReader([]byte(`{"cron":"bad"}`)))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("非法 cron status = %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/scripts/task.py/logs", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET logs status = %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/scripts/task.py", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d", recorder.Code)
	}
	if _, err := os.Stat(filepath.Join(app.resources.Scripts, "task.py")); !os.IsNotExist(err) {
		t.Fatal("脚本文件删除后仍存在")
	}
}

func TestScriptRunAndLogs(t *testing.T) {
	if _, err := exec.LookPath("python"); err != nil {
		t.Skip("python 不可用，跳过运行测试")
	}
	app := newScriptsTestApp(t)
	handler := app.Handler()
	if recorder := uploadScript(t, app, "echo.py", "print('hello from http test')", false); recorder.Code != http.StatusOK {
		t.Fatalf("upload status = %d", recorder.Code)
	}
	if recorder := postJSON(handler, "/scripts/echo.py/run", nil); recorder.Code != http.StatusOK {
		t.Fatalf("run status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/scripts/echo.py/logs?limit=262144", nil))
		var body struct {
			Data struct {
				Content  string `json:"content"`
				Running  bool   `json:"running"`
				ExitCode *int   `json:"exit_code"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if !body.Data.Running && body.Data.ExitCode != nil {
			if body.Data.Content == "" || !bytes.Contains([]byte(body.Data.Content), []byte("hello from http test")) {
				t.Fatalf("日志内容 = %q", body.Data.Content)
			}
			if *body.Data.ExitCode != 0 {
				t.Fatalf("exit code = %d", *body.Data.ExitCode)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("脚本未在超时内结束")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
