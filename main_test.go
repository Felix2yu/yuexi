package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"yuexi/internal/db"
)

// TestMain spins up an isolated SQLite database for the main package tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "yuexi-main-test")
	if err != nil {
		panic(err)
	}
	db.Init(filepath.Join(dir, "test.db"))
	code := m.Run()
	db.Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// TestBuildRouterRoutes exercises every registered route end-to-end so the
// route wiring in main (now extracted into buildRouter) is covered.
func TestBuildRouterRoutes(t *testing.T) {
	r := buildRouter()
	ts := httptest.NewServer(r)
	defer ts.Close()

	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/health"},
		{"GET", "/login"},
		{"POST", "/login"},
		{"GET", "/register"},
		{"POST", "/register"},
		{"POST", "/logout"},
		{"GET", "/"},                      // protected -> redirect to /login
		{"GET", "/person"},                // protected
		{"POST", "/person/create"},        // protected
		{"GET", "/person/edit"},           // protected
		{"POST", "/person/edit"},          // protected
		{"POST", "/person/delete"},        // protected
		{"GET", "/settings"},              // protected
		{"GET", "/settings/password"},     // protected
		{"POST", "/settings/password"},    // protected
		{"POST", "/record/create"},        // protected
		{"POST", "/record/edit"},          // protected
		{"POST", "/record/delete"},        // protected
		{"GET", "/api/records"},           // protected
		{"GET", "/export"},                // protected
		{"GET", "/export/download"},       // protected
		{"POST", "/import"},               // protected
		{"GET", "/api/notification"},      // protected
		{"POST", "/api/notification"},     // protected
		{"POST", "/api/notification/test"},// protected
		{"GET", "/api/notification/status"},// protected
		{"GET", "/api/anomaly"},           // protected
		{"GET", "/stats"},                 // protected
		{"GET", "/api/stats"},             // protected
		{"GET", "/api/daily/"},            // protected
		{"POST", "/api/daily/"},           // protected
		{"DELETE", "/api/daily/"},         // protected
		{"GET", "/manifest.json"},
		{"GET", "/sw.js"},
		{"GET", "/icon-192.png"},
		{"GET", "/icon-512.png"},
		{"GET", "/favicon.ico"},
		{"GET", "/favicon.png"},
	}

	for _, rt := range routes {
		req, err := http.NewRequest(rt.method, ts.URL+rt.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", rt.method, rt.path, err)
		}
		resp.Body.Close()
	}
}

// TestBuildRouterReturnsRouter ensures buildRouter yields a usable router.
func TestBuildRouterReturnsRouter(t *testing.T) {
	if buildRouter() == nil {
		t.Fatal("buildRouter returned nil")
	}
}
