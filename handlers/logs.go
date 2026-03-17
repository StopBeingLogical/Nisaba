package handlers

import (
	"html/template"
	"net/http"
)

// Logs renders the sync error log viewer.
func (h *Handler) Logs(w http.ResponseWriter, r *http.Request) {
	base, err := h.baseData("logs")
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	typeFilter := r.URL.Query().Get("type")
	consoleMode := r.URL.Query().Get("mode")
	if consoleMode == "" {
		consoleMode = "human"
	}

	errors, err := h.store.RecentSyncErrors(typeFilter, 500)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	types, _ := h.store.SyncErrorTypes()
	recentRuns, _ := h.store.RecentSyncs(50)

	base["ErrorLog"] = errors
	base["ErrorTypes"] = types
	base["TypeFilter"] = typeFilter
	base["RecentRuns"] = recentRuns
	base["ConsoleMode"] = consoleMode
	base["ConsoleHTML"] = template.HTML(ConsoleLogHTML(globalLogBuffer.Lines(), consoleMode))

	h.render(w, "logs.html", base)
}

// LogsConsole returns just the console log HTML fragment for HTMX polling.
func (h *Handler) LogsConsole(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "human"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(ConsoleLogHTML(globalLogBuffer.Lines(), mode))) //nolint:errcheck
}
