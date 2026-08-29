package main

import (
	"log"
	"net/http"
	"os"
	"time"
	"yuexi/internal/db"
	"yuexi/internal/handler"
	"yuexi/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// buildRouter constructs the application HTTP router with all routes registered.
func buildRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Compress(5))
	r.Use(slowRequestLogger)
	r.Use(middleware.Recoverer)

	r.Get("/health", handler.Health)

	// Auth routes (no middleware)
	r.Get("/login", handler.LoginPage)
	r.Post("/login", handler.LoginPost)
	r.Get("/register", handler.RegisterPage)
	r.Post("/register", handler.RegisterPost)
	r.Post("/logout", handler.LogoutPost)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(handler.AuthMiddleware)

		r.Get("/", handler.Home)
		r.Get("/person", handler.PersonList)
		r.Post("/person/create", handler.PersonCreate)
		r.Get("/person/edit", handler.PersonEdit)
		r.Post("/person/edit", handler.PersonEdit)
		r.Post("/person/delete", handler.PersonDelete)

		r.Get("/settings", handler.Settings)

		r.Get("/settings/password", handler.PasswordPage)
		r.Post("/settings/password", handler.PasswordPost)

		r.Post("/record/create", handler.RecordCreate)
		r.Post("/record/edit", handler.RecordEdit)
		r.Post("/record/delete", handler.RecordDelete)

		r.Get("/api/records", handler.RecordAPI)

		r.Get("/export", handler.ExportPage)
		r.Get("/export/download", handler.ExportDownload)
		r.Post("/import", handler.ImportHandler)

		r.Get("/api/notification", handler.NotificationConfigAPI)
		r.Post("/api/notification", handler.NotificationConfigAPI)
		r.Post("/api/notification/test", handler.NotificationTest)
		r.Get("/api/notification/status", handler.NotificationStatus)
		r.Get("/api/anomaly", handler.CycleAnomalyAPI)

		r.Get("/stats", handler.StatsPage)
		r.Get("/api/stats", handler.StatsAPI)

		r.Route("/api/daily", func(r chi.Router) {
			r.Get("/", handler.DailyLogAPI)
			r.Post("/", handler.DailyLogAPI)
			r.Delete("/", handler.DailyLogAPI)
		})
	})

	// PWA static files (no auth)
	r.Get("/manifest.json", handler.ServeManifest)
	r.Get("/sw.js", handler.ServeSW)
	r.Get("/icon-192.png", func(w http.ResponseWriter, r *http.Request) { handler.ServeIcon(w, r, 192) })
	r.Get("/icon-512.png", func(w http.ResponseWriter, r *http.Request) { handler.ServeIcon(w, r, 512) })
	r.Get("/favicon.ico", handler.ServeFavicon)
	r.Get("/favicon.png", func(w http.ResponseWriter, r *http.Request) { handler.ServeIcon(w, r, 32) })

	return r
}

// slowRequestLogger records only requests that are slow (>200ms) or returned a
// server error. This keeps per-request log volume low under load; the nginx
// reverse proxy already writes a full access log at the edge.
func slowRequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if d := time.Since(start); d >= 200*time.Millisecond || sw.status >= 500 {
			log.Printf("[slow] %s %s %d %s", r.Method, r.URL.Path, sw.status, d)
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func main() {
	dbPath := "data/yuexi.db"
	if p := os.Getenv("YUEXI_DB_PATH"); p != "" {
		dbPath = p
	}

	db.Init(dbPath)
	defer db.Close()

	port := "8080"
	if p := os.Getenv("YUEXI_PORT"); p != "" {
		port = p
	}

	r := buildRouter()

	// Start notification checker
	service.StartNotificationChecker()
	defer service.StopNotificationChecker()

	log.Printf("月汐启动在 http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
