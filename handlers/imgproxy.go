package handlers

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProxyImage fetches an external image URL, caches it on disk, and serves it.
// GET /img/proxy?url=<encoded_url>
//
// This allows remote browsers (e.g. behind a corporate firewall) to receive
// store artwork and icon assets without ever contacting the upstream CDNs directly.
func (h *Handler) ProxyImage(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		http.Error(w, "missing url", http.StatusBadRequest)
		return
	}

	// Only allow http/https to prevent SSRF against internal services.
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}

	cacheDir := filepath.Join(h.dataDir, "imgcache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		http.Error(w, "cache dir error", http.StatusInternalServerError)
		return
	}

	// Cache key: hex-encoded SHA-256 of the URL.
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(rawURL)))
	cachePath := filepath.Join(cacheDir, hash)
	ctPath := cachePath + ".ct"

	// Serve from cache if present.
	if data, err := os.ReadFile(cachePath); err == nil {
		ct := "image/jpeg"
		if ctBytes, err := os.ReadFile(ctPath); err == nil {
			ct = strings.TrimSpace(string(ctBytes))
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "public, max-age=604800") // 7 days
		w.Write(data)                                              //nolint:errcheck
		return
	}

	// Fetch from upstream.
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		http.Error(w, "fetch error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadGateway)
		return
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}

	// Write to cache (best-effort; errors don't block the response).
	_ = os.WriteFile(cachePath, data, 0644)
	_ = os.WriteFile(ctPath, []byte(ct), 0644)

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=604800")
	w.Write(data) //nolint:errcheck
}
