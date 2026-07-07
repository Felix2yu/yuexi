package main

import (
	"log"
	"net/http"
	"os"
	"yuexi/internal/db"
	"yuexi/internal/handler"
	"yuexi/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

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

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

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

	// Start notification checker
	service.StartNotificationChecker()
	defer service.StopNotificationChecker()

	log.Printf("月汐启动在 http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
