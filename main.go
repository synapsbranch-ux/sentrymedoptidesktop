package main

import (
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/app"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/backup"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/database"
	clinicserver "github.com/synapsbranch-ux/sentrymedoptidesktop/internal/server"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:apps/web/dist
var embeddedWeb embed.FS

type DesktopBridge struct {
	ctx     context.Context
	config  app.Config
	server  *http.Server
	clinic  *clinicserver.Server
	db      *database.DB
	closing bool
}

func (d *DesktopBridge) startup(ctx context.Context) { d.ctx = ctx }

func (d *DesktopBridge) OpenMobileAccess() {
	runtime.BrowserOpenURL(d.ctx, d.config.LoopbackURL())
}

// OpenInBrowser opens one page of the clinic application in the computer's own
// browser. The embedded webview cannot capture audio on every platform — the
// WebKitGTK view used on Linux declines the microphone without prompting — so
// recording falls back to the browser, where the permission prompt does appear.
// Only a path within this application is opened, never an arbitrary URL.
func (d *DesktopBridge) OpenInBrowser(path string) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		path = "/"
	}
	runtime.BrowserOpenURL(d.ctx, strings.TrimSuffix(d.config.LoopbackURL(), "/")+path)
}

func (d *DesktopBridge) OpenBackupFolder(path string) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(d.config.DataDir, "backups")
	}
	runtime.BrowserOpenURL(d.ctx, "file://"+filepath.ToSlash(filepath.Clean(path)))
}

func (d *DesktopBridge) SelectBackupFolder() (string, error) {
	return runtime.OpenDirectoryDialog(d.ctx, runtime.OpenDialogOptions{
		Title:                "Choose SentryMed backup folder",
		DefaultDirectory:     filepath.Join(d.config.DataDir, "backups"),
		CanCreateDirectories: true,
	})
}

func (d *DesktopBridge) SetWindowTitle(title string) {
	if strings.TrimSpace(title) == "" {
		title = clinicserver.DefaultClinicName
	}
	runtime.WindowSetTitle(d.ctx, title)
}

func (d *DesktopBridge) ClinicName() string {
	name := clinicserver.DefaultClinicName
	_ = d.db.QueryRow(`SELECT COALESCE(json_extract(value_json,'$.name'),'') FROM settings WHERE key='clinic'`).Scan(&name)
	if strings.TrimSpace(name) == "" {
		return clinicserver.DefaultClinicName
	}
	return strings.TrimSpace(name)
}

func (d *DesktopBridge) MinimizeServer() { runtime.WindowMinimise(d.ctx) }
func (d *DesktopBridge) Show()           { runtime.WindowShow(d.ctx); runtime.WindowUnminimise(d.ctx) }
func (d *DesktopBridge) BackupNow() error {
	_, err := (backup.Service{DB: d.db, DataDir: d.config.DataDir}).Create(context.Background(), "manual", nil)
	return err
}
func (d *DesktopBridge) ConnectedClients() int { return d.clinic.ConnectedClients() }
func (d *DesktopBridge) StopServer() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := d.server.Shutdown(ctx)
	if err != nil {
		_ = d.server.Close()
	}
	return err
}

func (d *DesktopBridge) Exit() {
	d.closing = true
	_ = d.StopServer()
	runtime.Quit(d.ctx)
}

func main() {
	config, err := app.LoadConfig(false)
	if err != nil {
		fatal(err)
	}
	logFile, err := os.OpenFile(filepath.Join(config.DataDir, "logs", "sentrymed.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		fatal(err)
	}
	defer logFile.Close()
	logger := slog.New(slog.NewJSONHandler(logFile, nil))
	db, err := database.Open(context.Background(), config.DataDir)
	if err != nil {
		fatal(err)
	}
	defer db.Close()
	if err := db.IntegrityCheck(context.Background()); err != nil {
		fatal(err)
	}
	webAssets, err := fs.Sub(embeddedWeb, "apps/web/dist")
	if err != nil {
		fatal(err)
	}
	server := clinicserver.New(db, config, logger, webAssets)
	schedulerContext, stopScheduler := context.WithCancel(context.Background())
	defer stopScheduler()
	server.StartBackground(schedulerContext)
	httpServer := &http.Server{Addr: config.Address, Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 30 * time.Second,
		TLSConfig:   &tls.Config{MinVersion: tls.VersionTLS12}, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
	go func() {
		var serveErr error
		if config.TLSCert != "" {
			serveErr = httpServer.ListenAndServeTLS(config.TLSCert, config.TLSKey)
		} else {
			serveErr = httpServer.ListenAndServe()
		}
		if serveErr != nil && serveErr != http.ErrServerClosed {
			logger.Error("LAN server failed", "error", serveErr)
		}
	}()
	bridge := &DesktopBridge{config: config, server: httpServer, clinic: server, db: db}
	startTray(bridge)
	err = wails.Run(&options.App{
		Title:            clinicserver.DefaultClinicName,
		Width:            1440,
		Height:           920,
		MinWidth:         1024,
		MinHeight:        700,
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		AssetServer:      &assetserver.Options{Assets: webAssets, Handler: clinicserver.DesktopHandler(server.Handler())},
		OnStartup:        bridge.startup,
		OnBeforeClose: func(ctx context.Context) bool {
			if bridge.closing {
				return false
			}
			if !trayAvailable() {
				bridge.closing = true
				_ = bridge.StopServer()
				return false
			}
			runtime.WindowMinimise(ctx)
			return true
		},
		Bind: []any{bridge},
	})
	if err != nil {
		fatal(err)
	}
	stopTray()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "SentryMed Opti:", err)
	os.Exit(1)
}
