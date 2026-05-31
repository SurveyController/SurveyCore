package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SurveyController/SurveyCore/internal/api"
	"github.com/SurveyController/SurveyCore/internal/logging"
	"github.com/SurveyController/SurveyCore/internal/tasks"
)

var version = "0.1.0"

func main() {
	addr, err := listenAddr()
	if err != nil {
		logging.ErrorFields("监听端口配置错误", logging.F("error", err))
		os.Exit(1)
	}

	manager, err := tasks.DefaultTaskManager()
	if err != nil {
		logging.ErrorFields("初始化任务存储失败", logging.F("error", err))
		os.Exit(1)
	}
	for _, loadErr := range manager.Load() {
		logging.WarnFields("恢复任务时跳过坏记录", logging.F("error", loadErr))
	}

	server := api.NewServer(manager, version)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logging.InfoFields("API 服务已启动", logging.F("addr", addr))
		errCh <- httpServer.ListenAndServe()
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-signalCh:
		logging.WarnFields("收到停止信号", logging.F("signal", sig.String()))
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logging.ErrorFields("API 服务启动失败", logging.F("error", err))
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logging.ErrorFields("API 服务关闭失败", logging.F("error", err))
		os.Exit(1)
	}
	manager.StopAll()
	if err := manager.Close(); err != nil {
		logging.ErrorFields("关闭任务存储失败", logging.F("error", err))
	}
	logging.Info("API 服务已关闭")
}

func listenAddr() (string, error) {
	port := strings.TrimSpace(os.Getenv("SURVEY_PORT"))
	if port == "" {
		port = "19178"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("SURVEY_PORT 必须是 1 到 65535 之间的端口号")
	}
	return net.JoinHostPort("localhost", port), nil
}
