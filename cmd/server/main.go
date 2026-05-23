package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xzxiong/ai-coding/internal/config"
	"github.com/xzxiong/ai-coding/internal/dashboard"
	"github.com/xzxiong/ai-coding/internal/handler"
	"github.com/xzxiong/ai-coding/internal/storage"
)

func main() {
	cfg := config.Load()

	store, err := storage.New(cfg.DataFile)
	if err != nil {
		log.Fatalf("Failed to load usage data: %v", err)
	}
	defer store.Close()

	mux := http.NewServeMux()
	mux.Handle("/v1/messages", handler.NewMessagesHandler(cfg, handler.WithStore(store)))
	mux.Handle("/dashboard/", dashboard.NewHandler(store))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: mux,
	}

	log.Printf("Starting server on %s", cfg.ListenAddr)
	log.Printf("Proxying to %s", cfg.OpenAIBaseURL)
	log.Printf("Dashboard at http://localhost%s/dashboard/", cfg.ListenAddr)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}
