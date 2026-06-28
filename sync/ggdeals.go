package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nisaba/db"
)

const ggDealsBase = "https://api.gg.deals/v1/prices/by-steam-app-id/"
const ggDealsChunkSize = 100 // max IDs per request per API docs

// GGDealsResult summarises a gg.deals pricing sync run.
type GGDealsResult struct {
	Updated  int
	NotFound int // no entry in gg.deals for this Steam App ID
	Errors   []string
}

// SyncGGDealsPricing fetches current and historical prices from the gg.deals API
// for all wishlist entries that have a Steam App ID, updating both
// best_current_price and historical_low in one batched pass.
// progress is called with (step label, done count, total count); it may be nil.
func SyncGGDealsPricing(store *db.Store, progress func(step string, done, total int)) (GGDealsResult, error) {
	var result GGDealsResult

	prog := func(step string, done, total int) {
		if progress != nil {
			progress(step, done, total)
		}
	}

	apiKey, err := store.GetConfig("ggdeals.api_key")
	if err != nil || apiKey == "" {
		return result, fmt.Errorf("ggdeals.api_key not configured — set it in Settings")
	}

	entries, err := store.ListWishlistForPricing()
	if err != nil {
		return result, fmt.Errorf("listing wishlist: %w", err)
	}
	if len(entries) == 0 {
		return result, nil
	}

	// Index entries by Steam App ID; skip entries with no Steam ID.
	type entryRef struct {
		id    string
		title string
	}
	byAppID := make(map[string]entryRef, len(entries))
	for _, e := range entries {
		if e.SteamAppID == "" {
			continue
		}
		byAppID[e.SteamAppID] = entryRef{id: e.ID, title: e.Title}
	}
	if len(byAppID) == 0 {
		return result, nil
	}

	appIDs := make([]string, 0, len(byAppID))
	for id := range byAppID {
		appIDs = append(appIDs, id)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	total := len(appIDs)
	fetched := 0
	prog("GG.deals price fetch", 0, total)

	for i := 0; i < len(appIDs); i += ggDealsChunkSize {
		end := i + ggDealsChunkSize
		if end > len(appIDs) {
			end = len(appIDs)
		}
		chunk := appIDs[i:end]

		prices, err := ggDealsFetch(client, apiKey, chunk)
		if err != nil {
			return result, fmt.Errorf("gg.deals fetch batch %d: %w", i/ggDealsChunkSize, err)
		}

		for _, appID := range chunk {
			entry, ok := byAppID[appID]
			if !ok {
				continue
			}
			p, found := prices[appID]
			if !found || p == nil {
				result.NotFound++
				fetched++
				continue
			}

			if err := applyGGDealsPrice(store, entry.id, entry.title, p, &result); err != nil {
				log.Printf("ggdeals apply %s: %v", entry.title, err)
			}
			fetched++
		}
		prog("GG.deals price fetch", fetched, total)
	}

	return result, nil
}

// applyGGDealsPrice writes the best current and historical prices to the DB.
func applyGGDealsPrice(store *db.Store, entryID, title string, p *ggDealsGamePrices, result *GGDealsResult) error {
	// Best current price: min of retail and keyshop current prices.
	currentPrice, currentStore := bestGGPrice(p.Prices.CurrentRetail, "retail", p.Prices.CurrentKeyshops, "keyshop")
	historicalPrice, historicalStore := bestGGPrice(p.Prices.HistoricalRetail, "retail", p.Prices.HistoricalKeyshops, "keyshop")

	if currentPrice == 0 && historicalPrice == 0 {
		result.NotFound++
		return nil
	}

	update := db.WishlistPricingUpdate{ID: entryID}
	if currentPrice > 0 {
		src := "gg.deals/" + currentStore
		update.BestCurrentPrice = &currentPrice
		update.BestCurrentStore = &src
		if p.URL != "" {
			update.BestPriceURL = &p.URL
		}
	}
	if historicalPrice > 0 {
		src := "gg.deals/" + historicalStore
		update.HistoricalLowPrice = &historicalPrice
		update.HistoricalLowStore = &src
	}

	if err := store.UpdateWishlistPricing(update); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("update %q: %v", title, err))
		return err
	}
	result.Updated++
	return nil
}

// bestGGPrice returns the lower of two price strings along with which source it came from.
func bestGGPrice(aStr, aLabel, bStr, bLabel string) (float64, string) {
	a := parseGGPrice(aStr)
	b := parseGGPrice(bStr)
	switch {
	case a > 0 && b > 0:
		if a <= b {
			return a, aLabel
		}
		return b, bLabel
	case a > 0:
		return a, aLabel
	case b > 0:
		return b, bLabel
	default:
		return 0, ""
	}
}

func parseGGPrice(s string) float64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

// ── API types ─────────────────────────────────────────────────────────────────

type ggDealsPriceFields struct {
	CurrentRetail      string `json:"currentRetail"`
	CurrentKeyshops    string `json:"currentKeyshops"`
	HistoricalRetail   string `json:"historicalRetail"`
	HistoricalKeyshops string `json:"historicalKeyshops"`
	Currency           string `json:"currency"`
}

type ggDealsGamePrices struct {
	Title  string             `json:"title"`
	URL    string             `json:"url"`
	Prices ggDealsPriceFields `json:"prices"`
}

type ggDealsResponse struct {
	Success bool                          `json:"success"`
	Data    map[string]*ggDealsGamePrices `json:"data"`
}

// ggDealsFetch calls the gg.deals API for a slice of Steam App IDs.
func ggDealsFetch(client *http.Client, apiKey string, appIDs []string) (map[string]*ggDealsGamePrices, error) {
	url := fmt.Sprintf("%s?key=%s&ids=%s&region=us",
		ggDealsBase, apiKey, strings.Join(appIDs, ","))

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limit exceeded (429)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var parsed ggDealsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if !parsed.Success {
		return nil, fmt.Errorf("API error: %s", string(body))
	}
	return parsed.Data, nil
}
