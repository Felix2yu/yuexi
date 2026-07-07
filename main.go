package main

import (
	"log"
	"net/http"
	"os"
	"yuexi/internal/db"
	"yuexi/internal/handler"

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

	// Pages
	r.Get("/", handler.Home)
	r.Get("/person", handler.PersonList)
	r.Post("/person/create", handler.PersonCreate)
	r.Get("/person/edit", handler.PersonEdit)
	r.Post("/person/edit", handler.PersonEdit)
	r.Post("/person/delete", handler.PersonDelete)

	// Settings
	r.Get("/settings", handler.Settings)

	// Records
	r.Post("/record/create", handler.RecordCreate)
	r.Post("/record/edit", handler.RecordEdit)
	r.Post("/record/delete", handler.RecordDelete)

	// API
	r.Get("/api/records", handler.RecordAPI)

	// Export/Import
	r.Get("/export", handler.ExportPage)
	r.Get("/export/download", handler.ExportDownload)
	r.Post("/import", handler.ImportHandler)

	log.Printf("月汐启动在 http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
