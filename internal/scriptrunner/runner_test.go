package scriptrunner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestRunner(t *testing.T) *Runner {
	t.Helper()
	root := t.TempDir()
	runner := New(Config{
		ScriptsDir:    filepath.Join(root, "scripts"),
		SdkDir:        filepath.Join(root, "sdk"),
		LogDir:        filepath.Join(root, "scripts", "logs"),
		PythonCommand: "python",
		ServerURL:     "http://127.0.0.1:8000",
	})
	if err := os.MkdirAll(runner.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	runner.Start()
	t.Cleanup(runner.Stop)
	return runner
}

func writeScript(t *testing.T, runner *Runner, name, content string) {
	t.Helper()
	if err := os.WriteFile(runner.ScriptPath(name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListAndDelete(t *testing.T) {
	runner := newTestRunner(t)
	writeScript(t, runner, "b.py", "print('b')")
	writeScript(t, runner, "a.py", "print('a')")
	scripts := runner.List()
	if len(scripts) != 2 || scripts[0].Name != "a.py" || scripts[1].Name != "b.py" {
		t.Fatalf("List() = %#v", scripts)
	}
	if err := runner.Delete("a.py"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if scripts := runner.List(); len(scripts) != 1 {
		t.Fatalf("删除后 List() = %#v", scripts)
	}
}

func TestScheduleRoundTrip(t *testing.T) {
	runner := newTestRunner(t)
	writeScript(t, runner, "task.py", "print('task')")
	if err := runner.SetSchedule("task.py", "32 16 * * *"); err != nil {
		t.Fatalf("SetSchedule() error = %v", err)
	}
	if err := runner.SetSchedule("task.py", "bad cron"); err == nil {
		t.Fatal("非法 cron 应当报错")
	}
	script, ok := runner.Status("task.py")
	if !ok || script.Schedule != "32 16 * * *" || script.NextRunAt == 0 {
		t.Fatalf("Status() = %#v", script)
	}
	if err := runner.ClearSchedule("task.py"); err != nil {
		t.Fatalf("ClearSchedule() error = %v", err)
	}
	if script, _ := runner.Status("task.py"); script.Schedule != "" {
		t.Fatalf("清除后仍有定时: %#v", script)
	}
}

func TestNextWakeDelayPrecision(t *testing.T) {
	runner := newTestRunner(t)
	writeScript(t, runner, "task.py", "print('x')")
	if err := runner.SetSchedule("task.py", "* * * * *"); err != nil {
		t.Fatalf("SetSchedule() error = %v", err)
	}
	runner.mu.Lock()
	entry := runner.schedules["task.py"]
	runner.mu.Unlock()
	want := time.Until(time.Unix(entry.Next, 0))
	if want < 0 {
		want = 0
	}
	delay := runner.nextWakeDelay(time.Now())
	if delta := delay - want; delta < -2*time.Second || delta > 2*time.Second {
		t.Fatalf("nextWakeDelay = %v, want ≈ %v（调度应精确到秒，而非轮询间隔）", delay, want)
	}
}

func TestRunAndLogs(t *testing.T) {
	if _, err := exec.LookPath("python"); err != nil {
		t.Skip("python 不可用，跳过运行测试")
	}
	runner := newTestRunner(t)
	writeScript(t, runner, "echo.py", "import os\nprint('hello from script')\nprint('YYB_SERVER=' + os.environ.get('YYB_SERVER', ''))")
	if err := runner.Run("echo.py"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		if script, ok := runner.Status("echo.py"); ok && !script.Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("脚本未在超时内结束")
		}
		time.Sleep(50 * time.Millisecond)
	}
	content, err := runner.Logs("echo.py", 0)
	if err != nil {
		t.Fatalf("Logs() error = %v", err)
	}
	if !strings.Contains(content, "hello from script") || !strings.Contains(content, "YYB_SERVER=http://127.0.0.1:8000") {
		t.Fatalf("日志内容不符: %q", content)
	}
	script, _ := runner.Status("echo.py")
	if script.ExitCode == nil || *script.ExitCode != 0 {
		t.Fatalf("exit code = %#v", script.ExitCode)
	}
}

func TestRunMissingPython(t *testing.T) {
	runner := newTestRunner(t)
	runner.cfg.PythonCommand = "definitely-not-a-real-python-binary"
	runner.pythonResolved = ""
	writeScript(t, runner, "x.py", "print('x')")
	err := runner.Run("x.py")
	if err == nil || !strings.Contains(err.Error(), "找不到 Python") {
		t.Fatalf("Run() error = %v", err)
	}
}
