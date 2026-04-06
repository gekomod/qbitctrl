package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qbitctrl/internal/api"
	"github.com/qbitctrl/internal/auth"
	"github.com/qbitctrl/internal/config"
	"github.com/qbitctrl/internal/middleware"
	"github.com/qbitctrl/internal/scheduler"
	"github.com/qbitctrl/internal/stats"
	"github.com/qbitctrl/internal/store"
	"github.com/qbitctrl/internal/websocket"
)

func main() {
	cfg := config.Load()

	log.Printf("qBitCtrl Go v1.1 — http://0.0.0.0:%s", cfg.Port)

	// Core components
	st    := store.New(cfg.DBPath, cfg.ARRPath)
	hub   := websocket.NewHub()
	sc    := stats.NewCollector()
	am    := auth.New(cfg.AuthPath)
	sched := scheduler.New(st, hub, sc)

	if err := st.Load(); err != nil {
		log.Printf("WARN: nie można załadować danych: %v", err)
	}
	sched.Start()

	// Router z middleware
	router := api.NewRouter(st, sched, hub, sc, am, cfg)
	handler := middleware.Chain(router,
		middleware.Logger,
		middleware.Gzip,
		middleware.RateLimit(200), // 200 req/s per IP
	)

	srv := &http.Server{
		Addr:         "0.0.0.0:" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Zamykanie serwera...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sched.Stop()
	srv.Shutdown(ctx)
	st.Save()
	log.Println("Gotowe.")
}
