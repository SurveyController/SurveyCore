package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SurveyController/SurveyCore/internal/appconfig"
	"github.com/SurveyController/SurveyCore/pkg/restapi"
)

var version = "dev"

func main() {
	cfg, err := appconfig.Load("")
	if err != nil {
		panic(err)
	}

	server, err := restapi.New(restapi.Config{DBPath: cfg.Storage.DBPath, Version: version, AIKey: cfg.AI.APIKey, AIBaseURL: cfg.AI.BaseURL, AIModel: cfg.AI.Model})
	if err != nil {
		panic(err)
	}
	defer server.Close()
	addr := cfg.ListenAddr()
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {

		errCh <- httpServer.ListenAndServe()
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-signalCh:
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}
