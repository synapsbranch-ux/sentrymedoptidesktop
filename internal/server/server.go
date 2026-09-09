package server

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/app"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/backup"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/database"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

type Server struct {
	db          *database.DB
	config      app.Config
	broker      *realtime.Broker
	logger      *slog.Logger
	maintenance sync.RWMutex
	router      chi.Router
	web         fs.FS
	// Who has opened which patient record recently, so a record left open on
	// screen is not logged once per live refresh.
	patientAccess      *patientAccessLog
	loginSlots         chan struct{}
	transcriptionSlots chan struct{}
	// The bundled diagnosis reference is loaded into the database the first time
	// it is searched, per server instance.
	diagnosisReference sync.Once
}

func New(db *database.DB, config app.Config, logger *slog.Logger, webAssets ...fs.FS) *Server {
	server := &Server{db: db, config: config, broker: realtime.New(), logger: logger, patientAccess: newPatientAccessLog(), loginSlots: make(chan struct{}, 2), transcriptionSlots: make(chan struct{}, 1)}
	if len(webAssets) > 0 {
		server.web = webAssets[0]
	}
	server.router = server.routes()
	return server
}

func (s *Server) Handler() http.Handler { return s.router }
func (s *Server) ConnectedClients() int { return s.broker.Connected() }

func (s *Server) StartBackground(ctx context.Context) {
	go (backup.Service{DB: s.db, DataDir: s.config.DataDir}).RunScheduler(ctx, s.logger, &s.maintenance)
}

func (s *Server) routes() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer, s.timeoutRequests)
	r.Use(s.securityHeaders, s.protectBrowserRequests, s.guardDatabase)
	r.Get("/health", s.handleHealth)
	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/setup/status", s.handleSetupStatus)
		api.Post("/setup/complete", s.handleSetupComplete)
		api.Post("/auth/login", s.handleLogin)
		api.Get("/public/branding/logo", s.handleClinicLogoGet)
		api.Get("/public/localization", s.handlePublicLocalization)
		api.Get("/public/display", s.handlePublicDisplay)
		api.Get("/public/events", s.handlePublicEvents)
		s.registerKioskRoutes(api)
		api.Group(func(protected chi.Router) {
			protected.Use(s.authenticate)
			protected.Post("/auth/logout", s.handleLogout)
			protected.Get("/auth/me", s.handleMe)
			protected.Get("/events", s.handleEvents)
			protected.Get("/events/revision", s.handleEventRevision)
			protected.With(s.requireDoctor).Post("/backups/restore", s.handleBackupRestore)
			protected.Group(func(live chi.Router) {
				// Request lifetime is guarded before authentication.
				s.registerClinicalRoutes(live)
				s.registerOperationsRoutes(live)
				s.registerSystemRoutes(live)
			})
		})
	})
	s.mountWebApp(r)
	return r
}

func (s *Server) timeoutRequests(next http.Handler) http.Handler {
	timed := middleware.Timeout(30 * time.Second)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && (r.URL.Path == "/api/v1/backups" || r.URL.Path == "/api/v1/backups/restore") {
			middleware.Timeout(10*time.Minute)(next).ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/api/v1/events" || r.URL.Path == "/api/v1/public/events" {
			next.ServeHTTP(w, r)
			return
		}
		timed.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "private, no-store")
		}
		if s.secureRequest(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(self), geolocation=()")
		// blob: is needed by the in-app document viewer, which fetches a stored
		// file through the authenticated API and renders it from an object URL;
		// a direct URL cannot carry the desktop shell's session header.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'; connect-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; frame-src 'self' blob:; object-src 'none'; style-src 'self' 'unsafe-inline'; font-src 'self'; script-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withMaintenanceRead(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.maintenance.RLock()
		defer s.maintenance.RUnlock()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "The clinic database is unavailable.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "product": "SentryMed Opti", "connectedClients": s.broker.Connected()})
}

func (s *Server) mountWebApp(r chi.Router) {
	webFS := s.web
	if webFS == nil {
		dist := filepath.Join("apps", "web", "dist")
		if configured := os.Getenv("SENTRYMED_WEB_DIR"); configured != "" {
			dist = configured
		}
		webFS = os.DirFS(dist)
	}
	fileServer := http.FileServer(http.FS(webFS))
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "ROUTE_NOT_FOUND", "API route was not found.")
			return
		}
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if info, err := fs.Stat(webFS, path); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, req)
			return
		}
		index, err := fs.ReadFile(webFS, "index.html")
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "WEB_CLIENT_NOT_BUILT", "Build the web client with `make web`.")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}

func (s *Server) nextNumber(ctx context.Context, tx *sql.Tx, name, prefix string, yearly bool) (string, error) {
	year := 0
	if yearly {
		year = time.Now().UTC().Year()
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sequences(name, year, value) VALUES(?, ?, 1)
		ON CONFLICT(name, year) DO UPDATE SET value = value + 1`, name, year); err != nil {
		return "", err
	}
	var value int
	if err := tx.QueryRowContext(ctx, "SELECT value FROM sequences WHERE name = ? AND year = ?", name, year).Scan(&value); err != nil {
		return "", err
	}
	if yearly {
		return prefix + "-" + time.Now().UTC().Format("2006") + "-" + leftPad(value, 6), nil
	}
	return prefix + "-" + leftPad(value, 6), nil
}

func leftPad(value int, width int) string {
	result := ""
	for value > 0 {
		result = string(rune('0'+value%10)) + result
		value /= 10
	}
	if result == "" {
		result = "0"
	}
	for len(result) < width {
		result = "0" + result
	}
	return result
}

func isUniqueViolation(err error) bool {
	return err != nil && (strings.Contains(strings.ToLower(err.Error()), "unique constraint") || strings.Contains(strings.ToLower(err.Error()), "constraint failed"))
}

func scanNullable(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func nilIfEmpty(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

var errConcurrentModification = errors.New("concurrent modification")
