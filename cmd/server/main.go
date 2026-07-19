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
	"greenpark/perencanaan/internal/authmw"
	"greenpark/perencanaan/internal/config"
	"greenpark/perencanaan/internal/repository"
	"greenpark/perencanaan/internal/service"
	httptransport "greenpark/perencanaan/internal/transport/http"
)

func main() {
	cfg := config.Load()

	// Dependency wiring (composition root). Storage is ALWAYS PostgreSQL — there
	// is no in-memory fallback, so data is persistent. PERENCANAAN_DATABASE_URL
	// overrides the default (the shared greenpark DB); a connection failure is
	// fatal rather than silently degrading to a volatile store.
	dsn := os.Getenv("PERENCANAAN_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@127.0.0.1:5434/greenpark?sslmode=disable"
	}
	// PERENCANAAN_SEED_MASTER=false starts an empty portfolio on a fresh DB (no
	// example projects / seed accounts) — real projects + the SSO-synced roster only.
	seedMaster := os.Getenv("PERENCANAAN_SEED_MASTER") != "false"
	pg, err := repository.NewPersistent(dsn, seedMaster)
	if err != nil {
		log.Fatalf("perencanaan: PostgreSQL required (no in-memory fallback): %v", err)
	}
	defer func() { _ = pg.Close() }()
	var repo repository.Store = pg
	freshStore := pg.Fresh()
	log.Println("perencanaan: using PostgreSQL store")
	sessions := auth.NewSessionStore(12 * time.Hour)
	gkCfg := service.GKConfig{
		OllamaModel: cfg.OllamaModel,
		PythonBin:   cfg.PythonBin,
		ScriptsDir:  cfg.GKScriptsDir,
		SkillPath:   cfg.GKSkillPath,
		AuthAPIBase: cfg.AuthAPIBase,
	}
	log.Printf("perencanaan: Deep Revisi AI — vision model %s via auth %s (central key)", gkCfg.OllamaModel, gkCfg.AuthAPIBase)
	svc := service.New(repo, sessions, gkCfg)
	handler := httptransport.NewHandler(svc)
	if v := authmw.New(authmw.Options{JWKSURL: os.Getenv("AUTH_JWKS_URL"), Issuer: os.Getenv("AUTH_ISSUER")}); v != nil {
		handler.SetSSO(v)
		log.Printf("perencanaan: SSO token acceptance enabled (jwks=%s)", os.Getenv("AUTH_JWKS_URL"))
	}
	router := httptransport.NewRouter(handler, cfg.AllowOrigin)

	// Populate sample data on first run only (persistent stores keep prior
	// data). Disable with PERENCANAAN_SEED_DEMO=false.
	if cfg.SeedDemo && freshStore {
		svc.SeedDemoSystem()
		log.Println("perencanaan: seeded sample data (set PERENCANAAN_SEED_DEMO=false to disable)")
	}

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
