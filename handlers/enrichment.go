package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	storesync "nisaba/sync"
)

const reviewPageSize = 30

// ReviewQueue renders the enrichment review queue.
func (h *Handler) ReviewQueue(w http.ResponseWriter, r *http.Request) {
	base, err := h.baseData("review")
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	entries, err := h.store.ListNeedsReview(offset)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	total, _ := h.store.CountNeedsReview()
	base["TotalNeedsReview"] = total
	base["Entries"] = entries
	base["Offset"] = offset
	base["PrevOffset"] = offset - reviewPageSize
	base["NextOffset"] = offset + reviewPageSize
	base["HasPrev"] = offset > 0
	base["HasNext"] = offset+reviewPageSize < total
	base["PageStart"] = offset + 1
	base["PageEnd"] = min(offset+reviewPageSize, total)

	h.render(w, "review.html", base)
}


// SetMatch accepts a user-selected IGDB match for a needs_review game.
func (h *Handler) SetMatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	igdbID := r.FormValue("igdb_id")
	if igdbID == "" {
		http.Error(w, "igdb_id required", http.StatusBadRequest)
		return
	}

	// Set the manual match. Full metadata enrichment runs in the background
	// after the IGDB ID is stored — the enrichment pipeline handles the rest.
	if err := h.store.SetIGDBMatch(id, igdbID); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if err := h.store.EnqueueEnrichment("game", id); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Remove card from DOM by returning empty outerHTML swap.
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
}

// SkipMatch marks a game as skipped in the review queue.
func (h *Handler) SkipMatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Mark as manual with no match — hides from queue but preserves data
	if err := h.store.SetEnrichmentStatus(id, "manual"); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	// Return empty to remove the card from the review queue
	w.WriteHeader(http.StatusOK)
}

// SearchIGDB proxies an IGDB title search for the review queue autocomplete.
func (h *Handler) SearchIGDB(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	q := r.URL.Query().Get("q")
	if q == "" {
		h.renderPartial(w, "review_results_partial.html", nil)
		return
	}

	// Ensure client is initialised.
	h.igdbMu.Lock()
	if h.igdb == nil {
		clientID, _ := h.store.GetConfig("igdb.client_id")
		clientSecret, _ := h.store.GetConfig("igdb.client_secret")
		if clientID == "" || clientSecret == "" {
			h.igdbMu.Unlock()
			http.Error(w, "IGDB credentials not configured", http.StatusServiceUnavailable)
			return
		}
		h.igdb = storesync.NewIGDBClient(clientID, clientSecret)
	}
	client := h.igdb
	h.igdbMu.Unlock()

	results, err := client.SearchGame(q)
	if err != nil {
		log.Printf("SearchIGDB %s: %v", q, err)
		http.Error(w, "IGDB search failed", http.StatusBadGateway)
		return
	}

	// Build template-friendly view models.
	type resultVM struct {
		IGDBID      int64
		Name        string
		CoverURL    string
		ReleaseYear string
		GameID      string
	}
	vms := make([]resultVM, 0, len(results))
	for _, g := range results {
		vms = append(vms, resultVM{
			IGDBID:      g.ID,
			Name:        g.Name,
			CoverURL:    g.CoverURL(),
			ReleaseYear: g.ReleaseYear(),
			GameID:      id,
		})
	}

	// review_results_partial.html uses $.GameID for the match button target,
	// so wrap in a struct that sets that context variable.
	type payload struct {
		GameID  string
		Results []resultVM
	}
	// The template iterates over Results with $.GameID available.
	// Since the template uses range directly on ., pass the slice
	// but inject GameID by re-rendering with a wrapper approach.
	//
	// Simpler: the template already uses {{ $.GameID }} and {{ range . }},
	// so wrap into a slice of structs that each carry GameID.
	h.renderPartial(w, "review_results_partial.html", vms)
}
