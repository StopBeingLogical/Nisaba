package handlers

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"nisaba/db"
	storesync "nisaba/sync"
)

// enrichState tracks a running IGDB enrichment job.
type enrichState struct {
	mu      sync.Mutex
	running bool
	prog    storesync.EnrichProgress
	lastMsg string
}

// mysteryPackState tracks a running mystery pack analysis.
type mysteryPackState struct {
	mu      sync.Mutex
	running bool
	done    int
	total   int
	lastMsg string
}

// syncAllState tracks the sequenced full-sync job.
type syncAllState struct {
	mu      sync.Mutex
	running bool
	step    string
	lastMsg string
}

// pageTemplates lists every template that defines a "content" block (full page).
var pageTemplates = []string{
	"dashboard.html",
	"library.html",
	"game_detail.html",
	"game_add.html",
	"wishlist.html",
	"wishlist_detail.html",
	"sync.html",
	"settings.html",
	"logs.html",
	"review.html",
	"mystery_packs.html",
	"mystery_pack_detail.html",
}

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	store   *db.Store
	tmplFS  fs.FS
	funcMap template.FuncMap
	// partials is a shared template set containing only the partial snippets
	// (fragments that are never full-page renders and have no "content" block).
	partials *template.Template
	// templates caches pre-parsed full-page templates so render() avoids
	// parsing from embed.FS on every request.
	pageTmpls map[string]*template.Template
	// igdb and rawg are lazily initialised on first enrichment run.
	igdb       *storesync.IGDBClient
	igdbMu     sync.Mutex
	rawg       *storesync.RAWGClient
	rawgMu     sync.Mutex
	enrichment  enrichState
	syncAll     syncAllState
	mysteryPack mysteryPackState
	dataDir     string
}

// New creates a Handler. tmplFS is the embed.FS subtree for templates/.
func New(store *db.Store, tmplFS fs.FS, funcMap template.FuncMap, dataDir string) *Handler {
	// Pre-parse the partial templates (no base.html, no content block conflict).
	parsedPartials := template.Must(
		template.New("").Funcs(funcMap).ParseFS(tmplFS,
			"game_grid_partial.html",
			"wishlist_grid_partial.html",
			"wishlist_cards_partial.html",
			"wishlist_add_partial.html",
			"sync_status_partial.html",
			"review_results_partial.html",
			"settings_thresholds_partial.html",
			"mystery_pack_analysis_partial.html",
			"mystery_pack_game_add_partial.html",
			"mystery_pack_games_section_partial.html",
		),
	)

	// Pre-parse every full-page template once and cache it. Each page has its
	// own template instance to avoid the "content block conflict" (multiple
	// {{ define "content" }}).
	pageTmpls := make(map[string]*template.Template, len(pageTemplates))
	for _, name := range pageTemplates {
		files := append([]string{"base.html", name}, partials...)
		t := template.Must(template.New("").Funcs(funcMap).ParseFS(tmplFS, files...))
		pageTmpls[name] = t
	}

	return &Handler{
		store:     store,
		tmplFS:    tmplFS,
		funcMap:   funcMap,
		partials:  parsedPartials,
		pageTmpls: pageTmpls,
		dataDir:   dataDir,
	}
}

// baseData returns the fields every page needs.
func (h *Handler) baseData(page string) (map[string]any, error) {
	needsReview, err := h.store.CountNeedsReview()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"Page":        page,
		"NeedsReview": needsReview,
	}, nil
}

// partials lists templates embedded inside page templates via {{ template "..." }}.
// They use unique define-names so they never trigger the "content block conflict".
var partials = []string{
	"game_grid_partial.html",
	"wishlist_grid_partial.html",
	"wishlist_cards_partial.html",
	"wishlist_add_partial.html",
	"sync_status_partial.html",
	"review_results_partial.html",
	"settings_thresholds_partial.html",
	"mystery_pack_analysis_partial.html",
	"mystery_pack_game_add_partial.html",
	"mystery_pack_games_section_partial.html",
}

// cleanupWishlistLinks runs the full wishlist cleanup pipeline after a sync:
// 1. Link wishlist entries to library by IGDB ID
// 2. Link by store ID (for entries without IGDB enrichment)
// 3. Delete linked entries (skipping those manually flagged for review)
func (h *Handler) cleanupWishlistLinks() {
	linked, _ := h.store.LinkWishlistToLibrary()
	if linked > 0 {
		log.Printf("sync: linked %d wishlist entries to library (by IGDB ID)", linked)
	}
	storeLinked, _ := h.store.LinkWishlistToLibraryByStore()
	if storeLinked > 0 {
		log.Printf("sync: linked %d wishlist entries to library (by store ID)", storeLinked)
	}
	deleted, _ := h.store.DeleteLinkedWishlistEntries()
	if deleted > 0 {
		log.Printf("sync: cleaned up %d wishlist entries that are now owned", deleted)
	}
}

// render uses the pre-parsed page template cache.
func (h *Handler) render(w http.ResponseWriter, name string, data any) {
	t, ok := h.pageTmpls[name]
	if !ok {
		log.Printf("render: unknown template %q", name)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base.html", data); err != nil {
		log.Printf("render exec %s: %v", name, err)
	}
}

func (h *Handler) renderPartial(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.partials.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render partial %s: %v", name, err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// isHTMX returns true if the request came from an HTMX swap.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// TemplateFuncMap returns the template function map used by all templates.
func TemplateFuncMap() template.FuncMap {
	return template.FuncMap{
		"formatBytes":     formatBytes,
		"formatTime":      formatTime,
		"priceClass":      priceClass,
		"priorityColor":   priorityColor,
		"iterate":         iterate,
		"sparkline":       sparkline,
		"storeShortLabel": storeShortLabel,
		"seq":             seq,
		"dict":            dict,
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"ptrFloat": func(p *float64) float64 {
			if p == nil {
				return 0
			}
			return *p
		},
		"ptrStr": func(p *string) string {
			if p == nil {
				return ""
			}
			return *p
		},
		"proxyURL": func(u string) string {
			if u == "" {
				return ""
			}
			return "/img/proxy?url=" + url.QueryEscape(u)
		},
	}
}

// sparkline generates an inline SVG price-history chart.
func sparkline(history []db.PriceHistoryRow) template.HTML {
	if len(history) < 2 {
		return ""
	}
	const (
		w   = 160.0
		h   = 32.0
		pad = 3.0
	)
	minP, maxP := history[0].Price, history[0].Price
	for _, r := range history {
		if r.Price < minP {
			minP = r.Price
		}
		if r.Price > maxP {
			maxP = r.Price
		}
	}
	prange := maxP - minP
	if prange == 0 {
		prange = 1
	}
	n := float64(len(history) - 1)
	pts := make([]string, len(history))
	for i, r := range history {
		x := pad + (float64(i)/n)*(w-2*pad)
		y := (h - pad) - ((r.Price - minP) / prange * (h - 2*pad))
		pts[i] = fmt.Sprintf("%.1f,%.1f", x, y)
	}
	last := history[len(history)-1]
	lx := w - pad
	ly := (h - pad) - ((last.Price - minP) / prange * (h - 2*pad))
	svg := fmt.Sprintf(
		`<svg width="%d" height="%d" viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg">`+
			`<polyline points="%s" fill="none" stroke="#6b7280" stroke-width="1.5" stroke-linejoin="round"/>`+
			`<circle cx="%.1f" cy="%.1f" r="2.5" fill="#f59e0b"/>`+
			`</svg>`,
		int(w), int(h), int(w), int(h),
		strings.Join(pts, " "),
		lx, ly,
	)
	return template.HTML(svg)
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func formatTime(s string) string {
	if s == "" {
		return "—"
	}
	// SQLite CURRENT_TIMESTAMP returns "2006-01-02 15:04:05"
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("Jan 2, 2006 15:04")
		}
	}
	return s
}

func priceClass(price, target float64) string {
	if price <= 2.00 {
		return "price-instant"
	}
	if price <= 5.00 {
		return "price-consider"
	}
	return "price-normal"
}

func priorityColor(priority int) string {
	switch {
	case priority >= 3:
		return "bg-red-500"
	case priority == 2:
		return "bg-amber-500"
	default:
		return "bg-gray-600"
	}
}

// storeShortLabel strips the source prefix from a store label.
// "gg.deals/retail" → "Retail", "allkeyshop/Kinguin" → "Kinguin",
// "instant-gaming" → "Instant Gaming", "loaded" → "Loaded".
func storeShortLabel(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	switch strings.ToLower(s) {
	case "retail":
		return "Retail stores"
	case "keyshop", "keyshops":
		return "Key resellers"
	case "instant-gaming":
		return "Instant Gaming"
	case "loaded":
		return "Loaded"
	case "":
		return "—"
	default:
		return s
	}
}

func iterate(start, end int) []int {
	result := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		result = append(result, i)
	}
	return result
}

func seq(start, end int) []int {
	result := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		result = append(result, i)
	}
	return result
}

func dict(values ...interface{}) (map[string]interface{}, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict: uneven number of arguments")
	}
	result := make(map[string]interface{})
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key at index %d is not a string", i)
		}
		result[key] = values[i+1]
	}
	return result, nil
}
