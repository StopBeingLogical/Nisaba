package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"nisaba/db"
	storesync "nisaba/sync"
)

// ScrapeQueue handles POST /api/mystery-packs/scrape/queue
// Accepts scraped page data and queues it for review.
func (h *Handler) ScrapeQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ScrapedAt string `json:"scraped_at"`
		Pages     []struct {
			SiteID      string `json:"site_id"`
			PackTitle   string `json:"pack_title"`
			Description string `json:"description"`
			CurrentURL  string `json:"current_url"`
			Games       []struct {
				Title       string `json:"title"`
				SteamAppID  *string `json:"steam_app_id"`
			} `json:"games"`
			Offers []struct {
				SellerName string  `json:"seller_name"`
				PriceUSD   float64 `json:"price_usd"`
				URL        string  `json:"url"`
				ValidUntil *string `json:"valid_until"`
			} `json:"offers"`
		} `json:"pages"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// Validate
	if len(req.Pages) == 0 {
		http.Error(w, "pages array is empty", http.StatusBadRequest)
		return
	}

	// Get all known sites
	sites, err := h.store.ListMysteryPackSites()
	if err != nil {
		log.Printf("list sites: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	siteMap := make(map[string]struct{})
	for _, site := range sites {
		siteMap[site.ID] = struct{}{}
	}

	// Validate each page
	for i, page := range req.Pages {
		if page.SiteID == "" {
			http.Error(w, fmt.Sprintf("page %d: site_id required", i), http.StatusBadRequest)
			return
		}
		if _, found := siteMap[page.SiteID]; !found {
			http.Error(w, fmt.Sprintf("page %d: unknown site_id", i), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(page.PackTitle) == "" {
			http.Error(w, fmt.Sprintf("page %d: pack_title required", i), http.StatusBadRequest)
			return
		}
		if len(page.Offers) == 0 {
			http.Error(w, fmt.Sprintf("page %d: offers array is empty", i), http.StatusBadRequest)
			return
		}

		// Check for duplicate game titles
		gameTitleMap := make(map[string]struct{})
		for _, game := range page.Games {
			title := strings.TrimSpace(game.Title)
			if title == "" {
				continue
			}
			if _, dup := gameTitleMap[title]; dup {
				http.Error(w, fmt.Sprintf("page %d: duplicate game title: %s", i, title), http.StatusBadRequest)
				return
			}
			gameTitleMap[title] = struct{}{}
		}

		// Check offers have valid prices
		for j, offer := range page.Offers {
			if offer.PriceUSD <= 0 {
				http.Error(w, fmt.Sprintf("page %d offer %d: price must be > 0", i, j), http.StatusBadRequest)
				return
			}
		}
	}

	// Purge expired queues (older than 7 days)
	sevenDaysAgo := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	_ = h.store.DeleteExpiredScrapeQueues(sevenDaysAgo)

	// Create queue
	queueID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	pagesJSON, _ := json.Marshal(req.Pages)
	if err := h.store.CreateScrapeQueue(queueID, req.ScrapedAt, now, string(pagesJSON)); err != nil {
		log.Printf("create queue: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "queued",
		"queue_id":     queueID,
		"pages_queued": len(req.Pages),
		"message":      fmt.Sprintf("%d pages queued", len(req.Pages)),
	})
}

// ScrapeReview handles GET /api/mystery-packs/scrape/review?queue={id}
// Returns the scraped data with computed diffs against stored packs.
func (h *Handler) ScrapeReview(w http.ResponseWriter, r *http.Request) {
	queueID := r.URL.Query().Get("queue")
	if queueID == "" {
		http.Error(w, "queue param required", http.StatusBadRequest)
		return
	}

	// Fetch queue
	queue, err := h.store.GetScrapeQueue(queueID)
	if err != nil {
		log.Printf("get queue: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if queue == nil {
		http.Error(w, "queue not found", http.StatusNotFound)
		return
	}

	// Unmarshal pages
	var pages []scrapedPage
	if err := json.Unmarshal([]byte(queue.PagesJSON), &pages); err != nil {
		log.Printf("unmarshal pages: %v", err)
		http.Error(w, "corrupted queue data", http.StatusInternalServerError)
		return
	}

	// Fetch indexes for game matching
	ownedIndex, _ := h.store.ListOwnedTitleIndex()
	wishlistIndex, _ := h.store.ListWishlistTitleIndex()

	// Lazy init IGDB
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

	// Build review response - use map[string]interface{} for flexibility
	var reviewPages []map[string]interface{}

	for _, page := range pages {
		rp := make(map[string]interface{})
		rp["site_id"] = page.SiteID
		rp["pack_title"] = page.PackTitle

		// Generate candidate pack_id
		packID := storesync.NormalizePack(page.SiteID, page.PackTitle)

		// Look up existing pack
		existingPack, _ := h.store.GetMysteryPack(packID)

		if existingPack == nil {
			rp["is_new"] = true
			rp["pack_id"] = nil
			// For new packs, create a minimal diff showing the new content
			diff := buildNewPackDiff(page)
			rp["diff"] = diff
		} else {
			rp["is_new"] = false
			rp["pack_id"] = packID

			diff := buildDiff(h, existingPack, page, ownedIndex, wishlistIndex, igdb)
			rp["diff"] = diff
		}

		reviewPages = append(reviewPages, rp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"queue_id": queueID,
		"queued_at": queue.CreatedAt,
		"pages":    reviewPages,
	})
}

// scrapedPage matches the struct unmarshaled from JSON in ScrapeReview
type scrapedPage struct {
	SiteID      string `json:"site_id"`
	PackTitle   string `json:"pack_title"`
	Description string `json:"description"`
	CurrentURL  string `json:"current_url"`
	Games       []struct {
		Title      string `json:"title"`
		SteamAppID *string `json:"steam_app_id"`
	} `json:"games"`
	Offers []struct {
		SellerName string  `json:"seller_name"`
		PriceUSD   float64 `json:"price_usd"`
		URL        string  `json:"url"`
		ValidUntil *string `json:"valid_until"`
	} `json:"offers"`
}

// buildNewPackDiff creates a diff for a new pack (showing only new values).
func buildNewPackDiff(scraped scrapedPage) map[string]interface{} {
	diff := make(map[string]interface{})

	diff["title"] = map[string]interface{}{
		"old":     nil,
		"new":     scraped.PackTitle,
		"changed": false,
	}

	diff["description"] = map[string]interface{}{
		"old":        nil,
		"new":        scraped.Description,
		"changed":    false,
		"similarity": 1.0,
	}

	// Calculate prices from scraped offers
	var newLowest, newAvg *float64
	if len(scraped.Offers) > 0 {
		var prices []float64
		for _, offer := range scraped.Offers {
			prices = append(prices, offer.PriceUSD)
		}
		lowest := prices[0]
		sum := 0.0
		for _, p := range prices {
			if p < lowest {
				lowest = p
			}
			sum += p
		}
		avg := sum / float64(len(prices))
		newLowest = &lowest
		newAvg = &avg
	}

	diff["prices"] = map[string]interface{}{
		"changed":    false,
		"old_lowest": nil,
		"old_avg":    nil,
		"new_lowest": newLowest,
		"new_avg":    newAvg,
	}

	diff["games"] = map[string]interface{}{
		"changed": false,
		"added":   []interface{}{},
		"removed": []interface{}{},
	}

	diff["new_sellers"] = []interface{}{}

	return diff
}

// buildDiff computes the diff between existing and scraped pack data.
func buildDiff(h *Handler, existing *db.MysteryPackDetail, scraped scrapedPage, ownedIndex, wishlistIndex map[string]struct{}, igdb *storesync.IGDBClient) map[string]interface{} {
	scrapedDesc := scraped.Description
	scrapedTitle := scraped.PackTitle

	diff := make(map[string]interface{})

	// Title diff
	titleChanged := existing.Name != scrapedTitle
	diff["title"] = map[string]interface{}{
		"old":     existing.Name,
		"new":     scrapedTitle,
		"changed": titleChanged,
	}

	// Description diff
	var existingDesc string
	if existing.Notes != nil {
		existingDesc = *existing.Notes
	}
	sim := storesync.DescriptionSimilarity(existingDesc, scrapedDesc)
	descChanged := sim < 0.5 && existingDesc != scrapedDesc
	diff["description"] = map[string]interface{}{
		"old":        existingDesc,
		"new":        scrapedDesc,
		"changed":    descChanged,
		"similarity": sim,
	}

	// Calculate prices from scraped offers
	var newLowest, newAvg *float64
	if len(scraped.Offers) > 0 {
		var prices []float64
		for _, offer := range scraped.Offers {
			prices = append(prices, offer.PriceUSD)
		}
		lowest := prices[0]
		sum := 0.0
		for _, p := range prices {
			if p < lowest {
				lowest = p
			}
			sum += p
		}
		avg := sum / float64(len(prices))
		newLowest = &lowest
		newAvg = &avg
	}

	// Price diff
	var oldLowest, oldAvg *float64
	if existing.PriceUSD != nil {
		oldLowest = existing.PriceUSD
		oldAvg = existing.PriceUSD
	}
	pricesChanged := (oldLowest == nil && newLowest != nil) || (oldLowest != nil && newLowest == nil) || (oldLowest != nil && newLowest != nil && *oldLowest != *newLowest)
	diff["prices"] = map[string]interface{}{
		"changed":    pricesChanged,
		"old_lowest": oldLowest,
		"old_avg":    oldAvg,
		"new_lowest": newLowest,
		"new_avg":    newAvg,
	}

	diff["games"] = map[string]interface{}{
		"changed": false,
		"added":   []interface{}{},
		"removed": []interface{}{},
	}

	diff["new_sellers"] = []interface{}{}

	return diff
}

// ScrapeApply handles POST /api/mystery-packs/scrape/apply
// Applies user-approved changes from the review step.
func (h *Handler) ScrapeApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		QueueID  string `json:"queue_id"`
		Decisions []struct {
			SiteID     string `json:"site_id"`
			PackTitle  string `json:"pack_title"`
			PackID     *string `json:"pack_id"`
			Action     string `json:"action"`
			Changes    struct {
				UpdateTitle       bool `json:"update_title"`
				UpdateDescription bool `json:"update_description"`
				UpdatePrices      bool `json:"update_prices"`
				UpdateGames       bool `json:"update_games"`
			} `json:"changes"`
			GameEdits []struct {
				Title      string `json:"title"`
				SteamAppID string `json:"steam_app_id"`
			} `json:"game_edits"`
			NewSellers []struct {
				SiteID  string `json:"site_id"`
				Name    string `json:"name"`
				BaseURL string `json:"base_url"`
			} `json:"new_sellers"`
		} `json:"decisions"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if req.QueueID == "" {
		http.Error(w, "queue_id required", http.StatusBadRequest)
		return
	}

	// Fetch queue
	queue, err := h.store.GetScrapeQueue(req.QueueID)
	if err != nil {
		log.Printf("get queue: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if queue == nil {
		http.Error(w, "queue not found", http.StatusNotFound)
		return
	}

	// Check if already applied
	if queue.AppliedAt != nil {
		w.WriteHeader(http.StatusConflict)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "conflict",
			"message": fmt.Sprintf("Queue %s was already applied on %s. Applying again may duplicate data — verify before retrying.", req.QueueID, *queue.AppliedAt),
		})
		return
	}

	var updatedCount, createdCount, newSellersCount int
	var messages []string

	now := time.Now().UTC().Format(time.RFC3339)

	// Process each decision
	for _, decision := range req.Decisions {
		if decision.Action == "skip" {
			continue
		}

		if decision.Action != "update" {
			http.Error(w, "invalid action", http.StatusBadRequest)
			return
		}

		// Add new sellers
		for _, seller := range decision.NewSellers {
			seller.SiteID = strings.ToLower(strings.TrimSpace(seller.SiteID))
			if !isValidSiteID(seller.SiteID) {
				http.Error(w, fmt.Sprintf("invalid site_id: %s", seller.SiteID), http.StatusBadRequest)
				return
			}
			if err := h.store.UpsertMysteryPackSite(seller.SiteID, seller.Name, seller.BaseURL); err != nil {
				log.Printf("upsert seller: %v", err)
				http.Error(w, "db error", http.StatusInternalServerError)
				return
			}
			newSellersCount++
		}

		// Determine pack_id
		var packID string
		isNewPack := decision.PackID == nil

		if isNewPack {
			packID = storesync.NormalizePack(decision.SiteID, decision.PackTitle)
		} else {
			packID = *decision.PackID
		}

		// Create new pack if needed
		if isNewPack {
			params := db.MysteryPackParams{
				ID:       packID,
				SiteID:   decision.SiteID,
				Name:     decision.PackTitle,
				URL:      "",
				PackType: "set_list",
				KeyCount: 10,
				Enabled:  true,
			}
			if err := h.store.UpsertMysteryPack(params); err != nil {
				log.Printf("upsert pack: %v", err)
				http.Error(w, "db error", http.StatusInternalServerError)
				return
			}
			createdCount++
			messages = append(messages, fmt.Sprintf("Created pack: %s", packID))
		}

		// Apply game edits
		if decision.Changes.UpdateGames {
			for _, edit := range decision.GameEdits {
				if err := h.store.UpsertMysteryPackGame(packID, edit.Title, edit.SteamAppID, 0); err != nil {
					log.Printf("upsert game: %v", err)
					http.Error(w, "db error", http.StatusInternalServerError)
					return
				}
			}
		}

		if !isNewPack {
			updatedCount++
			messages = append(messages, fmt.Sprintf("Updated pack: %s", packID))
		}
	}

	// Mark queue as applied
	if err := h.store.MarkQueueApplied(req.QueueID, now); err != nil {
		log.Printf("mark applied: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":             "applied",
		"updated_packs":      updatedCount,
		"created_packs":      createdCount,
		"new_sellers_added":  newSellersCount,
		"messages":           messages,
	})
}

// LookupGameSteamID handles POST /api/mystery-packs/lookup-game
// Returns the Steam App ID for a game title via IGDB lookup.
func (h *Handler) LookupGameSteamID(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Title string `json:"title"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}

	// Lazy init IGDB
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

	// Set CORS headers for all responses (not just preflight)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	// If no IGDB client, return null
	if igdb == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"title":        title,
			"steam_app_id": nil,
			"note":         "IGDB not configured",
		})
		return
	}

	// Look up game via IGDB
	steamAppID, err := igdb.SearchIGDBSteamAppID(title)
	if err != nil {
		log.Printf("igdb lookup for '%s': %v", title, err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"title":        title,
			"steam_app_id": nil,
			"note":         "lookup failed",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"title":        title,
		"steam_app_id": steamAppID,
	})
}

// Helper functions

func isValidSiteID(id string) bool {
	if len(id) == 0 {
		return false
	}
	// Can't start or end with dash
	if id[0] == '-' || id[len(id)-1] == '-' {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}
