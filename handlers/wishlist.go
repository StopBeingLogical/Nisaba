package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	storesync "nisaba/sync"
	"nisaba/db"
)

// Wishlist renders the wishlist grid view.
func (h *Handler) Wishlist(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	p := db.ListWishlistParams{
		Sort:       q.Get("sort"),
		Store:      q.Get("store"),
		BestStore:  q.Get("best_store"),
		Owned:      q.Get("owned") == "1",
		Duplicates: q.Get("duplicates") == "1",
	}
	if v := q.Get("q"); v != "" {
		p.Search = &v
	}
	if v := q.Get("priority"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			p.Priority = &n
		}
	}
	if v := q.Get("steam_deck"); v != "" {
		p.SteamDeck = &v
	}
	if v := q.Get("max_price"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			p.MaxPrice = &f
		}
	}

	entries, err := h.store.ListWishlist(p)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	view := q.Get("view")
	if view != "grid" {
		view = "list"
	}

	if isHTMX(r) {
		partial := "wishlist_grid_partial.html"
		if view == "grid" {
			partial = "wishlist_cards_partial.html"
		}
		h.renderPartial(w, partial, entries)
		return
	}

	base, err := h.baseData("wishlist")
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	total, _ := h.store.CountWishlist()
	priCounts, _ := h.store.WishlistPriorityCounts()
	deckCounts, _ := h.store.WishlistDeckCounts()
	storeCounts, _ := h.store.WishlistStoreCounts()
	bestStoreCounts, _ := h.store.WishlistBestStoreCounts()
	thresholds, _ := h.store.ListPriceThresholds()
	dupCount, _ := h.store.WishlistDuplicateCount()

	base["TotalEntries"] = total
	base["PriorityCounts"] = priCounts
	base["DeckCounts"] = deckCounts
	base["StoreCounts"] = storeCounts
	base["BestStoreCounts"] = bestStoreCounts
	base["PriceThresholds"] = thresholds
	base["DuplicateCount"] = dupCount
	base["Entries"] = entries
	base["View"] = view

	h.render(w, "wishlist.html", base)
}

// WishlistDetail renders the detail view for a single wishlist entry.
func (h *Handler) WishlistDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	entry, err := h.store.GetWishlistDetail(id)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if entry == nil {
		http.NotFound(w, r)
		return
	}

	stores, _ := h.store.ListWishlistStores(id)
	history, _ := h.store.ListWishlistPriceHistory(id)

	base, err := h.baseData("wishlist")
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	base["Entry"] = entry
	base["Stores"] = stores
	base["PriceHistory"] = history

	h.render(w, "wishlist_detail.html", base)
}

// RemoveWishlistEntry deletes the entry and redirects to the wishlist grid.
func (h *Handler) RemoveWishlistEntry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteWishlistEntry(id); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Redirect", "/wishlist")
	w.WriteHeader(http.StatusOK)
}

// PurchasedWishlistEntry removes the entry from the wishlist (the game was
// bought). If it was already linked to a library entry, redirects there;
// otherwise redirects to the wishlist grid.
func (h *Handler) PurchasedWishlistEntry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Capture library link before deletion.
	entry, _ := h.store.GetWishlistDetail(id)

	if err := h.store.DeleteWishlistEntry(id); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	redirect := "/wishlist"
	if entry != nil && entry.LibraryID.Valid {
		redirect = "/library/" + entry.LibraryID.String
	}
	w.Header().Set("HX-Redirect", redirect)
	w.WriteHeader(http.StatusOK)
}

// SearchWishlistAdd proxies an IGDB title search for the add-to-wishlist panel.
func (h *Handler) SearchWishlistAdd(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		h.renderPartial(w, "wishlist_add_partial.html", nil)
		return
	}

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
		log.Printf("SearchWishlistAdd %s: %v", q, err)
		http.Error(w, "IGDB search failed", http.StatusBadGateway)
		return
	}

	type resultVM struct {
		IGDBID      int64
		Name        string
		CoverURL    string
		ReleaseYear string
	}
	vms := make([]resultVM, 0, len(results))
	for _, g := range results {
		vms = append(vms, resultVM{
			IGDBID:      g.ID,
			Name:        g.Name,
			CoverURL:    g.CoverURL(),
			ReleaseYear: g.ReleaseYear(),
		})
	}
	h.renderPartial(w, "wishlist_add_partial.html", vms)
}

// AddWishlistEntry creates a manual wishlist entry from an IGDB result.
func (h *Handler) AddWishlistEntry(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	igdbID, err := strconv.ParseInt(r.FormValue("igdb_id"), 10, 64)
	if err != nil || igdbID == 0 {
		http.Error(w, "igdb_id required", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}
	coverURL := r.FormValue("cover_url")

	sortTitle := strings.ToLower(title)
	for _, article := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(sortTitle, article) {
			sortTitle = sortTitle[len(article):]
			break
		}
	}

	id := fmt.Sprintf("manual-%d", igdbID)
	if err := h.store.InsertManualWishlistEntry(id, igdbID, title, sortTitle, coverURL); err != nil {
		log.Printf("AddWishlistEntry: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Return the updated grid so the new entry appears immediately.
	entries, _ := h.store.ListWishlist(db.ListWishlistParams{})
	h.renderPartial(w, "wishlist_grid_partial.html", entries)
}

// FlagWishlistRemove toggles the "flag for removal" state on a wishlist entry.
func (h *Handler) FlagWishlistRemove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.ToggleWishlistFlagRemove(id); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateWishlistUserData saves priority, notes, target price, and preferred store.
func (h *Handler) UpdateWishlistUserData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	priority, _ := strconv.Atoi(r.FormValue("priority"))
	notes := r.FormValue("notes")
	preferredStore := r.FormValue("preferred_store")

	var targetPrice *float64
	if tp := r.FormValue("target_price"); tp != "" {
		if v, err := strconv.ParseFloat(tp, 64); err == nil {
			targetPrice = &v
		}
	}

	if err := h.store.UpdateWishlistUserData(id, priority, notes, targetPrice, preferredStore); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
