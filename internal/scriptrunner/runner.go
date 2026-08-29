package scriptrunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxScriptLogTail = 256 << 10

var validScriptName = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}._-]{0,127}\.py$`)

func ValidName(name string) bool { return validScriptName.MatchString(name) }

type Config struct {
	ScriptsDir    string
	SdkDir        string
	LogDir        string
	PythonCommand string
	ServerURL     string
}

type Script struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	UpdatedAt  int64  `json:"updated_at"`
	Running    bool   `json:"running"`
	StartedAt  int64  `json:"started_at,omitempty"`
	FinishedAt int64  `json:"finished_at,omitempty"`
	ExitCode   *int   `json:"exit_code,omitempty"`
	LastError  string `json:"last_error,omitempty"`
	Schedule   string `json:"schedule,omitempty"`
	NextRunAt  int64  `json:"next_run_at,omitempty"`
}

type scheduleEntry struct {
	Cron string `json:"cron"`
	Next int64  `json:"next_at"`
}

type runState struct {
	cancel     context.CancelFunc
	startedAt  time.Time
	finishedAt time.Time
	finished   bool
	exitCode   int
	errMsg     string
	stream     *logStream
	done       chan struct{}
}

// logStream 把一次运行的输出实时广播给所有订阅者（WebSocket 日志）。
type logStream struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func newLogStream() *logStream {
	return &logStream{subs: map[chan []byte]struct{}{}}
}

func (s *logStream) subscribe(ch chan []byte) {
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
}

func (s *logStream) unsubscribe(ch chan []byte) {
	s.mu.Lock()
	delete(s.subs, ch)
	s.mu.Unlock()
}

func (s *logStream) publish(p []byte) {
	s.mu.Lock()
	for ch := range s.subs {
		select {
		case ch <- p:
		default: // 订阅者消费不及时则丢弃，不阻塞脚本运行
		}
	}
	s.mu.Unlock()
}

// broadcastWriter 同时写日志文件并广播到订阅者。
type broadcastWriter struct {
	file   *os.File
	stream *logStream
}

func (w *broadcastWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	if n > 0 {
		// p 是调用方（os/exec 的 io.Copy）复用的缓冲区，广播前必须拷贝，
		// 否则下一次写入会覆盖已发布但尚未被订阅者消费的数据。
		chunk := append([]byte(nil), p[:n]...)
		w.stream.publish(chunk)
	}
	return n, err
}

type Runner struct {
	cfg            Config
	mu             sync.Mutex
	runs           map[string]*runState
	schedules      map[string]scheduleEntry
	schedulesPath  string
	schedulerStop  context.CancelFunc
	schedulerDone  chan struct{}
	pythonResolved string
}

func New(cfg Config) *Runner {
	return &Runner{
		cfg: Config{
			ScriptsDir:    absDir(cfg.ScriptsDir),
			SdkDir:        absDir(cfg.SdkDir),
			LogDir:        absDir(cfg.LogDir),
			PythonCommand: cfg.PythonCommand,
			ServerURL:     cfg.ServerURL,
		},
		runs:          map[string]*runState{},
		schedules:     map[string]scheduleEntry{},
		schedulesPath: filepath.Join(absDir(cfg.ScriptsDir), "schedules.json"),
	}
}

func absDir(path string) string {
	if path == "" {
		return ""
	}
	if absolute, err := filepath.Abs(path); err == nil {
		return filepath.Clean(absolute)
	}
	return filepath.Clean(path)
}

func (r *Runner) Dir() string       { return r.cfg.ScriptsDir }
func (r *Runner) SdkDir() string    { return r.cfg.SdkDir }
func (r *Runner) ServerURL() string { return r.cfg.ServerURL }

func (r *Runner) ScriptPath(name string) string {
	return filepath.Join(r.cfg.ScriptsDir, name)
}

func (r *Runner) Python() string {
	if r.pythonResolved != "" {
		return r.pythonResolved
	}
	cmd := r.cfg.PythonCommand
	if cmd == "" {
		cmd = "python"
	}
	if path, err := exec.LookPath(cmd); err == nil {
		r.pythonResolved = path
	}
	return r.pythonResolved
}

func (r *Runner) PythonAvailable() bool { return r.Python() != "" }

// Start 加载定时任务并启动调度循环。
func (r *Runner) Start() {
	r.loadSchedules()
	ctx, cancel := context.WithCancel(context.Background())
	r.schedulerStop = cancel
	r.schedulerDone = make(chan struct{})
	go func() {
		defer close(r.schedulerDone)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.schedulerTick()
			}
		}
	}()
}

// Stop 停止调度循环并终止正在运行的脚本。
func (r *Runner) Stop() {
	if r.schedulerStop != nil {
		r.schedulerStop()
		<-r.schedulerDone
		r.schedulerStop = nil
	}
	r.mu.Lock()
	names := make([]string, 0, len(r.runs))
	for name := range r.runs {
		names = append(names, name)
	}
	r.mu.Unlock()
	for _, name := range names {
		_ = r.StopRun(name)
	}
}

func (r *Runner) List() []Script {
	entries, err := os.ReadDir(r.cfg.ScriptsDir)
	if err != nil {
		return []Script{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Script, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".py") || !ValidName(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		script := Script{
			Name:      name,
			Size:      info.Size(),
			UpdatedAt: info.ModTime().Unix(),
		}
		if state, ok := r.runs[name]; ok {
			script.Running = !state.finished
			script.StartedAt = state.startedAt.Unix()
			if state.finished {
				script.FinishedAt = state.finishedAt.Unix()
				code := state.exitCode
				script.ExitCode = &code
				script.LastError = state.errMsg
			}
		}
		if schedule, ok := r.schedules[name]; ok {
			script.Schedule = schedule.Cron
			script.NextRunAt = schedule.Next
		}
		out = append(out, script)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Runner) Status(name string) (Script, bool) {
	for _, script := range r.List() {
		if script.Name == name {
			return script, true
		}
	}
	return Script{}, false
}

func (r *Runner) Run(name string) error {
	if !ValidName(name) {
		return fmt.Errorf("脚本名不合法: %s", name)
	}
	path := r.ScriptPath(name)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("脚本不存在: %s", name)
	}
	python := r.Python()
	if python == "" {
		return fmt.Errorf("找不到 Python（%s），请安装并确保在 PATH 中", defaultString(r.cfg.PythonCommand, "python"))
	}
	r.mu.Lock()
	if state, ok := r.runs[name]; ok && !state.finished {
		r.mu.Unlock()
		return fmt.Errorf("脚本正在运行: %s", name)
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &runState{
		cancel:    cancel,
		startedAt: time.Now(),
		stream:    newLogStream(),
		done:      make(chan struct{}),
	}
	r.runs[name] = state
	r.mu.Unlock()

	if err := os.MkdirAll(r.cfg.LogDir, 0o755); err != nil {
		r.finish(name, state, -1, err.Error())
		return err
	}
	logPath := filepath.Join(r.cfg.LogDir, name+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		r.finish(name, state, -1, err.Error())
		return err
	}
	writer := &broadcastWriter{file: logFile, stream: state.stream}

	command := exec.CommandContext(ctx, python, name)
	command.Dir = r.cfg.ScriptsDir
	command.Env = r.scriptEnv(name)
	command.Stdout = writer
	command.Stderr = writer

	go func() {
		runErr := command.Run()
		_ = logFile.Close()
		code, message := 0, ""
		switch {
		case runErr == nil:
		case ctx.Err() != nil:
			code, message = -1, "已停止"
		default:
			var exitError *exec.ExitError
			if errors.As(runErr, &exitError) {
				code = exitError.ExitCode()
			} else {
				code, message = -1, runErr.Error()
			}
		}
		r.finish(name, state, code, message)
		if file, openErr := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644); openErr == nil {
			_ = file.Close()
		}
	}()
	return nil
}

func (r *Runner) StopRun(name string) error {
	r.mu.Lock()
	state, ok := r.runs[name]
	r.mu.Unlock()
	if !ok || state.finished {
		return fmt.Errorf("脚本未在运行: %s", name)
	}
	state.cancel()
	return nil
}

func (r *Runner) Logs(name string, limitBytes int) (string, error) {
	if !ValidName(name) {
		return "", fmt.Errorf("脚本名不合法: %s", name)
	}
	data, err := os.ReadFile(filepath.Join(r.cfg.LogDir, name+".log"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if limitBytes > 0 && len(data) > limitBytes {
		data = data[len(data)-limitBytes:]
	}
	return string(data), nil
}

func (r *Runner) Delete(name string) error {
	if !ValidName(name) {
		return fmt.Errorf("脚本名不合法: %s", name)
	}
	if err := r.StopRun(name); err != nil && !strings.Contains(err.Error(), "未在运行") {
		return err
	}
	if err := os.Remove(r.ScriptPath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_ = os.Remove(filepath.Join(r.cfg.LogDir, name+".log"))
	r.mu.Lock()
	delete(r.schedules, name)
	r.mu.Unlock()
	return r.saveSchedules()
}

func (r *Runner) SetSchedule(name, cron string) error {
	if !ValidName(name) {
		return fmt.Errorf("脚本名不合法: %s", name)
	}
	if _, err := os.Stat(r.ScriptPath(name)); err != nil {
		return fmt.Errorf("脚本不存在: %s", name)
	}
	schedule, err := parseCron(cron)
	if err != nil {
		return err
	}
	next := schedule.next(time.Now())
	if next.IsZero() {
		return fmt.Errorf("该 cron 一年内不会触发")
	}
	r.mu.Lock()
	r.schedules[name] = scheduleEntry{Cron: schedule.Raw(), Next: next.Unix()}
	r.mu.Unlock()
	return r.saveSchedules()
}

func (r *Runner) ClearSchedule(name string) error {
	r.mu.Lock()
	delete(r.schedules, name)
	r.mu.Unlock()
	return r.saveSchedules()
}

func (r *Runner) finish(name string, state *runState, code int, message string) {
	r.mu.Lock()
	state.finished = true
	state.finishedAt = time.Now()
	state.exitCode = code
	state.errMsg = message
	r.mu.Unlock()
	if state.done != nil {
		close(state.done)
	}
}

// Subscribe 订阅脚本的实时日志流。
// 返回当前日志内容、增量日志通道、运行结束信号与退订函数。
// 脚本未在运行时 logs/done 为 nil，仅返回现有内容。
func (r *Runner) Subscribe(name string) (current []byte, logs <-chan []byte, done <-chan struct{}, cancel func(), err error) {
	noop := func() {}
	if !ValidName(name) {
		return nil, nil, nil, noop, fmt.Errorf("脚本名不合法: %s", name)
	}
	r.mu.Lock()
	state, ok := r.runs[name]
	if !ok || state.finished {
		r.mu.Unlock()
		content, readErr := r.Logs(name, maxScriptLogTail)
		if readErr != nil {
			return nil, nil, nil, noop, readErr
		}
		return []byte(content), nil, nil, noop, nil
	}
	stream := state.stream
	doneCh := state.done
	sub := make(chan []byte, 256)
	stream.subscribe(sub)
	r.mu.Unlock()

	// 先订阅再读文件：读到之前广播的增量会因 init 覆盖而不重复。
	content, readErr := r.Logs(name, maxScriptLogTail)
	if readErr != nil {
		stream.unsubscribe(sub)
		return nil, nil, nil, noop, readErr
	}
	cancelFn := func() { stream.unsubscribe(sub) }
	return []byte(content), sub, doneCh, cancelFn, nil
}

func (r *Runner) scriptEnv(name string) []string {
	env := os.Environ()
	if r.cfg.ServerURL != "" {
		env = append(env, "YYB_SERVER="+r.cfg.ServerURL, "YYB_BASE_URL="+r.cfg.ServerURL)
	}
	env = append(env, "YYB_SCRIPT_NAME="+name, "YYB_SCRIPTS_DIR="+r.cfg.ScriptsDir)
	env = append(env, "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8", "PYTHONUNBUFFERED=1")
	if r.cfg.SdkDir != "" {
		if _, err := os.Stat(r.cfg.SdkDir); err == nil {
			pythonPath := r.cfg.SdkDir
			if existing := os.Getenv("PYTHONPATH"); existing != "" {
				pythonPath += string(os.PathListSeparator) + existing
			}
			env = append(env, "PYTHONPATH="+pythonPath)
		}
	}
	return env
}

func (r *Runner) schedulerTick() {
	now := time.Now()
	var due []string
	r.mu.Lock()
	for name, entry := range r.schedules {
		if entry.Next <= 0 || now.Unix() < entry.Next {
			continue
		}
		if state, ok := r.runs[name]; ok && !state.finished {
			continue
		}
		due = append(due, name)
	}
	r.mu.Unlock()

	changed := false
	for _, name := range due {
		if err := r.Run(name); err != nil {
			log.Printf("scripts: 定时运行 %s 失败: %v", name, err)
			continue
		}
		r.mu.Lock()
		entry := r.schedules[name]
		if schedule, err := parseCron(entry.Cron); err == nil {
			if next := schedule.next(now); !next.IsZero() {
				entry.Next = next.Unix()
				r.schedules[name] = entry
				changed = true
			}
		}
		r.mu.Unlock()
	}
	if changed {
		_ = r.saveSchedules()
	}
}

func (r *Runner) loadSchedules() {
	data, err := os.ReadFile(r.schedulesPath)
	if err != nil {
		return
	}
	var raw map[string]scheduleEntry
	if err = json.Unmarshal(data, &raw); err != nil {
		log.Printf("scripts: 解析定时任务失败: %v", err)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, entry := range raw {
		if !ValidName(name) {
			continue
		}
		if _, err := parseCron(entry.Cron); err != nil {
			continue
		}
		r.schedules[name] = entry
	}
}

func (r *Runner) saveSchedules() error {
	r.mu.Lock()
	raw := make(map[string]scheduleEntry, len(r.schedules))
	for name, entry := range r.schedules {
		raw[name] = entry
	}
	r.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(r.schedulesPath), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(r.schedulesPath, data, 0o644)
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
