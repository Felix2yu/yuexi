package handler

import (
	"html/template"
	"strings"
	"sync"
)

var (
	templateCache = make(map[string]*template.Template)
	templateMu    sync.RWMutex
)

func parseTemplates(names ...string) (*template.Template, error) {
	// Create cache key from sorted names
	key := strings.Join(names, ":")

	// Check cache first (read lock)
	templateMu.RLock()
	if tmpl, ok := templateCache[key]; ok {
		templateMu.RUnlock()
		return tmpl, nil
	}
	templateMu.RUnlock()

	// Parse and cache (write lock)
	tmpl := template.New("")
	for _, name := range names {
		data, err := templateFS.ReadFile("template/" + name)
		if err != nil {
			return nil, err
		}
		tmpl, err = tmpl.Parse(string(data))
		if err != nil {
			return nil, err
		}
	}

	templateMu.Lock()
	templateCache[key] = tmpl
	templateMu.Unlock()

	return tmpl, nil
}
