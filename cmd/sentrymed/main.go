package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/app"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/database"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/server"
)

func main() {
	dev := flag.Bool("dev", false, "store runtime data in ./data")
	seed := flag.Bool("seed", false, "install development-only sample data and exit")
	flag.Parse()

	config, err := app.LoadConfig(*dev || *seed)
	if err != nil {
		fatal(err)
	}
	logFile, err := os.OpenFile(config.DataDir+"/logs/sentrymed.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		fatal(err)
	}
	defer logFile.Close()
	logger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx := context.Background()
	db, err := database.Open(ctx, config.DataDir)
	if err != nil {
		fatal(err)
	}
	defer db.Close()
	if err := db.IntegrityCheck(ctx); err != nil {
		fatal(err)
	}
	if *seed {
		if err := server.SeedDevelopment(ctx, db); err != nil {
			fatal(err)
		}
		fmt.Println("Development seed installed. Do not use seed credentials in production.")
		return
	}

	application := server.New(db, config, logger)
	schedulerContext, stopScheduler := context.WithCancel(context.Background())
	defer stopScheduler()
	application.StartBackground(schedulerContext)
	httpServer := &http.Server{
		Addr:              config.Address,
		Handler:           application.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		logger.Info("clinic server started", "address", config.Address, "data_dir", config.DataDir)
		var serveErr error
		if config.TLSCert != "" {
			serveErr = httpServer.ListenAndServeTLS(config.TLSCert, config.TLSKey)
		} else {
			serveErr = httpServer.ListenAndServe()
		}
		if serveErr != nil && serveErr != http.ErrServerClosed {
			logger.Error("clinic server stopped unexpectedly", "error", serveErr)
			fmt.Fprintln(os.Stderr, serveErr)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdown); err != nil {
		logger.Error("server shutdown failed", "error", err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "SentryMed Opti:", err)
	os.Exit(1)
}
