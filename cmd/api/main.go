package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vital/rendycrm-app/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := app.LoadConfig()
	if len(os.Args) > 1 && os.Args[1] == "cleanup-demo-data" {
		cfg.EnableDemoSeed = false
		runtime, err := app.NewRuntime(ctx, cfg)
		if err != nil {
			log.Fatal(err)
		}
		defer runtime.Close()
		if err := runtime.CleanupDemoData(ctx); err != nil {
			log.Fatal(err)
		}
		log.Printf("demo data cleaned")
		return
	}

	server, err := app.NewServer(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil && err != http.ErrServerClosed {
			log.Printf("http shutdown failed: %v", err)
		}
	}()

	log.Printf("api listening on :%s", cfg.Port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
