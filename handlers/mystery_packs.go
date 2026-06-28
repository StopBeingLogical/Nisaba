package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"nisaba/db"
	storesync "nisaba/sync"
)

// MysteryPacks returns the mystery packs list page.
func (h *Handler) MysteryPacks(w http.ResponseWriter, r *http.Request) {
	packs, err := h.store.ListMysteryPacks()
	if err != nil {
		log.Printf("list packs: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	sites, err := h.store.ListMysteryPackSites()
	if err != nil {
		log.Printf("list sites: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	data, err := h.baseData("mystery-packs")
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}

	data["Packs"] = packs
	data["Sites"] = sites
	h.render(w, "mystery_packs.html", data)
}

// MysteryPackDetail returns the detail page for a single mystery pack.
func (h *Handler) MysteryPackDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	pack, err := h.store.GetMysteryPack(id)
	if err != nil {
		log.Printf("get pack: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	if pack == nil {
		http.NotFound(w, r)
		return
	}

	data, err := h.baseData("mystery-pack-detail")
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}

	data["Pack"] = pack
	h.render(w, "mystery_pack_detail.html", data)
}

// AddMysteryPackSite adds a new mystery pack site (POST).
func (h *Handler) AddMysteryPackSite(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form parse error", http.StatusBadRequest)
		return
	}

	id := r.FormValue("id")
	name := r.FormValue("name")
	baseURL := r.FormValue("base_url")

	if id == "" || name == "" || baseURL == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	if err := h.store.UpsertMysteryPackSite(id, name, baseURL); err != nil {
		log.Printf("upsert site: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "Site added")
}

// AddMysteryPack adds a new mystery pack (POST).
func (h *Handler) AddMysteryPack(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form parse error", http.StatusBadRequest)
		return
	}

	id := r.FormValue("id")
	siteID := r.FormValue("site_id")
	name := r.FormValue("name")
	url := r.FormValue("url")
	packType := r.FormValue("pack_type")
	keyCountStr := r.FormValue("key_count")

	if id == "" || siteID == "" || name == "" || url == "" || packType == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	keyCount := 10
	if keyCountStr != "" {
		if k, err := strconv.Atoi(keyCountStr); err == nil && k > 0 {
			keyCount = k
		}
	}

	params := db.MysteryPackParams{
		ID:       id,
		SiteID:   siteID,
		Name:     name,
		URL:      url,
		PackType: packType,
		KeyCount: keyCount,
		Enabled:  true,
	}

	if err := h.store.UpsertMysteryPack(params); err != nil {
		log.Printf("upsert pack: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "Pack added")
}

// AddPackGame adds a game to a mystery pack (POST).
// Attempts to auto-resolve Steam App ID via IGDB; if not found, prompts for manual entry.
func (h *Handler) AddPackGame(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form parse error", http.StatusBadRequest)
		return
	}

	packID := chi.URLParam(r, "id")
	title := r.FormValue("title")
	steamAppIDStr := r.FormValue("steam_app_id")

	if title == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}

	steamAppID := steamAppIDStr

	// If no Steam App ID provided, try IGDB lookup
	if steamAppID == "" {
		h.igdbMu.Lock()
		if h.igdb == nil {
			clientID, _ := h.store.GetConfig("igdb.client_id")
			clientSecret, _ := h.store.GetConfig("igdb.client_secret")
			if clientID != "" && clientSecret != "" {
				h.igdb = storesync.NewIGDBClient(clientID, clientSecret)
			}
		}
		igdb := h.igdb
		h.igdbMu.Unlock()

		if igdb != nil {
			if appID, err := igdb.SearchIGDBSteamAppID(title); err == nil && appID != "" {
				steamAppID = appID
			}
		}
	}

	// If still no Steam App ID, return the form with a prompt for manual entry
	if steamAppID == "" {
		data := map[string]any{
			"PackID":        packID,
			"Title":         title,
			"NotFound":      true,
			"ErrorMessage":  "Could not auto-resolve Steam App ID. Please enter it manually.",
		}
		h.renderPartial(w, "mystery_pack_game_add_partial.html", data)
		return
	}

	// Add the game with the resolved App ID
	if err := h.store.UpsertMysteryPackGame(packID, title, steamAppID, 0); err != nil {
		log.Printf("upsert game: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	// Return updated games list
	games, err := h.store.ListMysteryPackGames(packID)
	if err != nil {
		log.Printf("list games: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"PackID": packID,
		"Games":  games,
	}
	h.renderPartial(w, "mystery_pack_games_section_partial.html", data)
}

// RemovePackGame removes a game from a mystery pack (POST).
func (h *Handler) RemovePackGame(w http.ResponseWriter, r *http.Request) {
	packID := chi.URLParam(r, "id")
	title := chi.URLParam(r, "title")

	if err := h.store.DeleteMysteryPackGame(packID, title); err != nil {
		log.Printf("delete game: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	games, err := h.store.ListMysteryPackGames(packID)
	if err != nil {
		log.Printf("list games: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"PackID": packID,
		"Games":  games,
	}
	h.renderPartial(w, "mystery_pack_games_section_partial.html", data)
}

// UpdatePackPrice updates the price of a mystery pack (POST).
func (h *Handler) UpdatePackPrice(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form parse error", http.StatusBadRequest)
		return
	}

	packID := chi.URLParam(r, "id")
	priceStr := r.FormValue("price_usd")

	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil || price <= 0 {
		http.Error(w, "invalid price", http.StatusBadRequest)
		return
	}

	if err := h.store.UpdateMysteryPackPrice(packID, price); err != nil {
		log.Printf("update price: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "$%.2f", price)
}

// DeleteMysteryPack deletes a mystery pack (POST).
func (h *Handler) DeleteMysteryPack(w http.ResponseWriter, r *http.Request) {
	packID := chi.URLParam(r, "id")

	if err := h.store.DeleteMysteryPack(packID); err != nil {
		log.Printf("delete pack: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "Pack deleted")
}

// AnalyzePack analyzes a single mystery pack synchronously and returns the analysis result.
func (h *Handler) AnalyzePack(w http.ResponseWriter, r *http.Request) {
	packID := chi.URLParam(r, "id")

	pack, err := h.store.GetMysteryPack(packID)
	if err != nil {
		log.Printf("get pack: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	if pack == nil {
		http.NotFound(w, r)
		return
	}

	// Get API key
	apiKey, err := h.store.GetConfig("ggdeals.api_key")
	if err != nil || apiKey == "" {
		data := map[string]any{
			"Error": "GG.deals API key not configured. Set it in Settings.",
		}
		h.renderPartial(w, "mystery_pack_analysis_partial.html", data)
		return
	}

	// Get owned title index
	ownedIndex, err := h.store.ListOwnedTitleIndex()
	if err != nil {
		log.Printf("list owned titles: %v", err)
		data := map[string]any{
			"Error": "Failed to fetch owned games.",
		}
		h.renderPartial(w, "mystery_pack_analysis_partial.html", data)
		return
	}

	// Analyze the pack
	var analysis storesync.MysteryPackAnalysisResult
	switch pack.PackType {
	case "set_list":
		var analysisErr error
		analysis, analysisErr = storesync.AnalyzeSetListPack(
			http.DefaultClient,
			apiKey,
			pack,
			ownedIndex,
			nil,
		)
		if analysisErr != nil {
			log.Printf("analyze set_list: %v", analysisErr)
		}
	case "min_value":
		analysis = storesync.AnalyzeMinValuePack(pack)
	default:
		http.Error(w, "unknown pack type", http.StatusBadRequest)
		return
	}

	// Set analyzed timestamp
	now := time.Now().UTC().Format(time.RFC3339)
	analysis.AnalyzedAt = now

	// Save to database
	params := db.MysteryPackAnalysisParams{
		PackID:            packID,
		AnalyzedAt:        now,
		PackPriceUSD:      &analysis.PackPriceUSD,
		PoolSize:          &analysis.PoolSize,
		OverlapCount:      &analysis.OverlapCount,
		NewGamesCount:     &analysis.NewGamesCount,
		KeyshopValueTotal: &analysis.KeyshopValueTotal,
		KeyshopValueNew:   &analysis.KeyshopValueNew,
		ROIKeyshop:        &analysis.ROIKeyshop,
		ROIPerKey:         &analysis.ROIPerKey,
		VarianceScore:     &analysis.VarianceScore,
		Recommendation:    &analysis.Recommendation,
		OverlapTitles:     analysis.OverlapTitles,
		NotableGames:      analysis.NotableGames,
	}
	if err := h.store.SaveMysteryPackAnalysis(params); err != nil {
		log.Printf("save analysis: %v", err)
	}

	// Return the analysis partial
	data := map[string]any{
		"Analysis": analysis,
		"Pack":     pack,
	}
	h.renderPartial(w, "mystery_pack_analysis_partial.html", data)
}
