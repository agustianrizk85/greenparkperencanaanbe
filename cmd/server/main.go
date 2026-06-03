// Command server starts the Perencanaan (planning / demand) control tower API.
//
// It wires the layers together — repository -> service -> HTTP transport — and
// runs an HTTP server with graceful shutdown.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"greenpark/perencanaan/internal/auth"
	"greenpark/perencanaan/internal/config"
	"greenpark/perencanaan/internal/repository"
	"greenpark/perencanaan/internal/service"
	httptransport "greenpark/perencanaan/internal/transport/http"
)

func main() {
	cfg := config.Load()

	// Dependency wiring (composition root).
	repo := repository.NewMemory()
	sessions := auth.NewSessionStore(12 * time.Hour)
	svc := service.New(repo, sessions)
	handler := httptransport.NewHandler(svc)
	router := httptransport.NewRouter(handler, cfg.AllowOrigin)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Run the server in a goroutine so main can wait for shutdown signals.
	go func() {
		log.Printf("perencanaan API listening on http://localhost:%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("perencanaan: server error: %v", err)
		}
	}()

	// Wait for an interrupt signal for graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("perencanaan: shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("perencanaan: graceful shutdown failed: %v", err)
	}
	log.Println("perencanaan: stopped")
}
