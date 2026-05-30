package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SurveyController/SurveyConsole/internal/api"
	"github.com/SurveyController/SurveyConsole/internal/logging"
)

var version = "0.1.0"

func main() {
	addr := os.Getenv("SURVEYCONSOLE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:19178"
	}

	manager, err := api.DefaultTaskManager()
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

	manager.StopAll()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logging.ErrorFields("API 服务关闭失败", logging.F("error", err))
		os.Exit(1)
	}
	logging.Info("API 服务已关闭")
}
