package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/anthropic/foreman/internal/config"
	"github.com/anthropic/foreman/internal/orchestrator"
	"github.com/anthropic/foreman/internal/server"
)

func main() {
	configPath := flag.String("c", "foreman.yaml", "Path to foreman.yaml config file")
	flag.StringVar(configPath, "config", "foreman.yaml", "Path to foreman.yaml config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Set up file logging if configured
	if cfg.LogFile != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.LogFile), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating log directory: %v\n", err)
			os.Exit(1)
		}
		logFile, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening log file %s: %v\n", cfg.LogFile, err)
			os.Exit(1)
		}
		defer logFile.Close()
		// Write to both stderr and log file
		multiWriter := io.MultiWriter(os.Stderr, logFile)
		log.SetOutput(multiWriter)
		log.Printf("Logging to file: %s", cfg.LogFile)
	}

	log.Printf("Foreman starting — project root: %s", cfg.ProjectRoot)

	// Create orchestrator
	orch := orchestrator.New(cfg, *configPath)

	// Create HTTP server (uses inline fallback frontend when no embedded assets)
	srv := server.New(orch, cfg.Password, cfg.APIToken)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("Web UI: http://%s", addr)

	// Auto-start services
	orch.StartAutoStart()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down...")
		orch.StopAll()
		os.Exit(0)
	}()

	// Start HTTP server
	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.Handler(),
	}

	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
