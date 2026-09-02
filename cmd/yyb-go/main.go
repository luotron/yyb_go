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
	"syscall"
	"time"

	"yyb_go/internal/httpapi"
	"yyb_go/internal/serviceconfig"
)

const serviceConfigFilename = "config/service.json"

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
	fileConfig, err := serviceconfig.Load(serviceConfigFilename)
	if err != nil {
		return err
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
