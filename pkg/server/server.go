package server

import (
	"context"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"net/http"
	"time"
)

func ListenAndServe(port string, timeout time.Duration, logger *zap.SugaredLogger, stopCh <-chan struct{})  {
	mux :=http.DefaultServeMux
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 0,
		WriteTimeout:      1 * time.Second,
		IdleTimeout:       15 * time.Second,
	}
	logger.Infof("Starting HTTP server on port %s", port)

	// run server in background
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			logger.Fatalf("HTTP Server crashed %v", err)
		}
	}()

	<- stopCh
	ctx, cancle := context.WithTimeout(context.Background(), timeout)
	defer cancle()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Errorf("HTTP Server graceful shutdown failed %v", err)
	} else {
		logger.Info("HTTP Server stopped")
	}
}