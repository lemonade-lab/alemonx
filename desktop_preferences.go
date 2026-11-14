package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const desktopPreferencesPath = "/api/desktop/preferences"

type desktopPreferences struct {
	mu   sync.Mutex
	path string
}

func desktopPreferenceKey(key string) bool {
	switch key {
	case "alemonjs-theme", "persist:alemonx-guide", "persist:alemonx-data",
		"persist:alemonx-workspace", "persist:alemonx-navigation", "persist:alemonx-browser-recent",
		"alemonx-legacy-robot-list",
		"autoConnectEnabled", "ALemonTestOne:image:compression", "alx:operation-log":
		return true
	}
	return strings.HasPrefix(key, "alx:plugin-webview:") ||
		strings.HasPrefix(key, "alx:service-webview:") ||
		(strings.HasPrefix(key, "alemonx:qq-live:v2:") && strings.HasSuffix(key, ":window"))
}

func (p *desktopPreferences) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	// A cross-origin page cannot send this header without a successful preflight.
	if r.Header.Get("X-ALX-Desktop-Preferences") != "1" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	values := make(map[string]*string)
	raw, err := os.ReadFile(p.path)
	if err == nil {
		err = json.Unmarshal(raw, &values)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		http.Error(w, "Cannot read preferences", http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(values)
	case http.MethodPut:
		var change struct {
			Key    string
			Value  *string
			Import bool
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 6<<20))
		if decoder.Decode(&change) != nil || decoder.Decode(new(any)) != io.EOF || !desktopPreferenceKey(change.Key) {
			http.Error(w, "Invalid preference", http.StatusBadRequest)
			return
		}
		if values == nil {
			values = make(map[string]*string)
		}
		if _, exists := values[change.Key]; exists && change.Import {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// A null tombstone prevents older origins from restoring deleted values.
		values[change.Key] = change.Value
		if err := p.save(values); err != nil {
			http.Error(w, "Cannot save preferences", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (p *desktopPreferences) save(values map[string]*string) error {
	dir := filepath.Dir(p.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".preferences-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(values)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), p.path)
}
