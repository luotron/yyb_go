package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"yyb_go/internal/httpapi"
	"yyb_go/internal/serviceconfig"
)

const serviceConfigFilename = "config/service.json"

// locateServiceConfig 定位 config/service.json 并返回其绝对路径。
// 依次从当前工作目录、可执行文件所在目录向上逐级查找，
// 保证无论从项目根目录、bin 目录还是双击 exe 启动都能找到配置。
func locateServiceConfig() (string, error) {
	var starts []string
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	seen := map[string]bool{}
	for _, start := range starts {
		dir, err := filepath.Abs(start)
		if err != nil {
			continue
		}
		for {
			if seen[dir] {
				break
			}
			seen[dir] = true
			if info, err := os.Stat(filepath.Join(dir, serviceConfigFilename)); err == nil && !info.IsDir() {
				return filepath.Join(dir, serviceConfigFilename), nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", fmt.Errorf("未找到 %s：请确认配置随程序放在同一目录树中", serviceConfigFilename)
}

// listenBaseURL 将监听地址转换为脚本环境变量 YYB_SERVER 可用的本机地址。
func listenBaseURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "http://127.0.0.1:8000"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func main() {
	if err := run(); err != nil {
		log.Printf("YYB Go 运行失败: %v", err)
		os.Exit(1)
	}
}

func run() error {
	configPath, err := locateServiceConfig()
	if err != nil {
		return err
	}
	fileConfig, err := serviceconfig.Load(configPath)
	if err != nil {
		return err
	}
	// 配置里的 data_root / asset_root 若为相对路径，以配置所在项目根目录为基准解析，
	// 与启动时的工作目录无关。
	projectRoot := filepath.Dir(filepath.Dir(configPath))
	if !filepath.IsAbs(fileConfig.DataRoot) {
		fileConfig.DataRoot = filepath.Join(projectRoot, fileConfig.DataRoot)
	}
	if !filepath.IsAbs(fileConfig.AssetRoot) {
		fileConfig.AssetRoot = filepath.Join(projectRoot, fileConfig.AssetRoot)
	}

	config := httpapi.Config{
		ResourceRoot:      fileConfig.DataRoot,
		AssetRoot:         fileConfig.AssetRoot,
		DBFilename:        fileConfig.DatabaseFilename,
		TCPProxy:          fileConfig.TCPProxy,
		SessionTTL:        30 * time.Minute,
		RequestTimeout:    8 * time.Second,
		AvatarTimeout:     10 * time.Second,
		ScanTimeout:       180 * time.Second,
		QRSessionTTL:      5 * time.Minute,
		KeepAliveInterval: time.Duration(fileConfig.KeepAliveIntervalMinute) * time.Minute,
		KeepAliveAhead:    time.Duration(fileConfig.KeepAliveAheadMinute) * time.Minute,
		PythonCommand:     fileConfig.PythonCommand,
		ScriptsServerURL:  listenBaseURL(fileConfig.ListenAddress),
		SecretKey:         fileConfig.SecretKey,
		SessionDuration:   time.Duration(fileConfig.SessionDurationMinute) * time.Minute,
		CookieSecure:      fileConfig.CookieSecure,
	}

	app, err := httpapi.NewApp(config)
	if err != nil {
		return fmt.Errorf("初始化应用失败: %w", err)
	}
	defer app.Close()

	go app.RefreshAccountsOnStartup(context.Background())

	server := &http.Server{
		Addr:              fileConfig.ListenAddress,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("YYB Go 已启动: http://%s", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	select {
	case signalValue := <-stop:
		log.Printf("收到退出信号 %s，正在关闭服务", signalValue)
		shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err = server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("关闭 HTTP 服务失败: %w", err)
		}
		if err = <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP 服务异常退出: %w", err)
		}
		log.Printf("YYB Go 已安全停止")
		return nil
	case err = <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP 服务异常退出: %w", err)
	}
}
