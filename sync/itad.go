package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"nisaba/db"
)

const itadBase = "https://api.isthereanydeal.com"

// itadSteamShopID is ITAD's numeric shop id for Steam, used by the bulk
// ID-resolution endpoint.
const itadSteamShopID = 61

// itadLookupChunkSize is how many Steam App IDs go into one bulk lookup.
const itadLookupChunkSize = 100

// PricingResult summarises an ITAD pricing sync run.
type PricingResult struct {
	Updated  int
	NotFound int
	Errors   []string
}

// SyncITADPricing looks up ITAD game IDs in bulk for wishlist entries that
// don't have one cached, then batch-fetches current best price, the storefront
// offering it, the deal URL, and the historical low.
// progress is called with (step label, done count, total count) as work advances;
// it may be nil.
func SyncITADPricing(store *db.Store, progress func(step string, done, total int)) (PricingResult, error) {
	var result PricingResult

	prog := func(step string, done, total int) {
		if progress != nil {
			progress(step, done, total)
		}
	}

	apiKey, err := store.GetConfig("itad.api_key")
	if err != nil || apiKey == "" {
		return result, fmt.Errorf("itad.api_key not configured — set it in Settings")
	}

	entries, err := store.ListWishlistForPricing()
	if err != nil {
		return result, fmt.Errorf("listing wishlist: %w", err)
	}
	if len(entries) == 0 {
		return result, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// ── Step 1: resolve ITAD IDs for entries that don't have one yet ─────────
	// One request covers up to 100 Steam App IDs, so a full wishlist costs a
	// handful of requests rather than one per entry.
	var appIDs []string
	idxByApp := make(map[string][]int, len(entries))
	for i := range entries {
		if entries[i].ITADId != "" || entries[i].SteamAppID == "" {
			continue
		}
		appID := entries[i].SteamAppID
		if _, seen := idxByApp[appID]; !seen {
			appIDs = append(appIDs, appID)
		}
		idxByApp[appID] = append(idxByApp[appID], i)
	}

	needLookup := len(appIDs)
	looked := 0
	prog("ITAD ID lookup", 0, needLookup)
	for i := 0; i < len(appIDs); i += itadLookupChunkSize {
		end := i + itadLookupChunkSize
		if end > len(appIDs) {
			end = len(appIDs)
		}
		chunk := appIDs[i:end]

		found, err := itadLookupBulk(client, apiKey, chunk)
		if err != nil {
			log.Printf("itad bulk lookup %d: %v", i/itadLookupChunkSize, err)
			result.Errors = append(result.Errors, fmt.Sprintf("lookup batch %d: %v", i/itadLookupChunkSize, err))
			looked += len(chunk)
			continue
		}

		for _, appID := range chunk {
			itadID := found[appID]
			if itadID == "" {
				result.NotFound += len(idxByApp[appID])
				looked += len(idxByApp[appID])
				continue
			}
			for _, idx := range idxByApp[appID] {
				entries[idx].ITADId = itadID
				if err := store.SetWishlistITADId(entries[idx].ID, itadID); err != nil {
					log.Printf("itad set id %s: %v", entries[idx].ID, err)
				}
				looked++
			}
		}
		prog("ITAD ID lookup", looked, needLookup)
	}

	// ── Step 2: collect all entries that now have an ITAD ID ─────────────────
	idToEntry := make(map[string]*db.WishlistPricingRow, len(entries))
	var itadIDs []string
	for i := range entries {
		if entries[i].ITADId == "" {
			continue
		}
		itadIDs = append(itadIDs, entries[i].ITADId)
		idToEntry[entries[i].ITADId] = &entries[i]
	}
	if len(itadIDs) == 0 {
		return result, nil
	}

	// ── Step 3: batch-fetch prices in chunks of 200 ──────────────────────────
	const chunkSize = 200
	fetched := 0
	prog("ITAD price fetch", 0, len(itadIDs))
	for i := 0; i < len(itadIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(itadIDs) {
			end = len(itadIDs)
		}
		prices, err := itadOverview(client, apiKey, itadIDs[i:end])
		if err != nil {
			return result, fmt.Errorf("overview batch %d: %w", i/chunkSize, err)
		}
		for _, p := range prices {
			entry := idToEntry[p.ID]
			if entry == nil {
				continue
			}
			update := db.WishlistPricingUpdate{ID: entry.ID}
			if p.Current != nil {
				update.BestCurrentPrice = &p.Current.Price.Amount
				update.BestCurrentStore = &p.Current.Shop.Name
				if p.Current.URL != "" {
					update.BestPriceURL = &p.Current.URL
				}
			}
			if p.Lowest != nil {
				update.HistoricalLowPrice = &p.Lowest.Price.Amount
				update.HistoricalLowStore = &p.Lowest.Shop.Name
			}
			if err := store.UpdateWishlistPricing(update); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("update %s: %v", entry.ID, err))
			} else {
				result.Updated++
			}
			fetched++
		}
		prog("ITAD price fetch", fetched, len(itadIDs))
	}

	return result, nil
}

// ── ITAD API types ────────────────────────────────────────────────────────────

type itadPriceEntry struct {
	ID      string `json:"id"`
	Current *struct {
		Shop  struct{ Name string `json:"name"` } `json:"shop"`
		Price struct{ Amount float64 `json:"amount"` } `json:"price"`
		URL   string `json:"url"`
	} `json:"current"`
	Lowest *struct {
		Shop  struct{ Name string `json:"name"` } `json:"shop"`
		Price struct{ Amount float64 `json:"amount"` } `json:"price"`
	} `json:"lowest"`
}

type itadOverviewResp struct {
	Prices []itadPriceEntry `json:"prices"`
}

// ── API calls ─────────────────────────────────────────────────────────────────

// itadLookupBulk resolves Steam App IDs to ITAD game IDs via
// POST /lookup/id/shop/61/v1. The body is a JSON array of "app/<id>" strings
// and the response maps each back to its ITAD id, null when unknown.
func itadLookupBulk(client *http.Client, apiKey string, steamAppIDs []string) (map[string]string, error) {
	ids := make([]string, 0, len(steamAppIDs))
	for _, appID := range steamAppIDs {
		ids = append(ids, "app/"+appID)
	}
	body, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/lookup/id/shop/%d/v1?key=%s", itadBase, itadSteamShopID, apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed map[string]*string
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	found := make(map[string]string, len(parsed))
	for key, id := range parsed {
		if id == nil {
			continue
		}
		found[strings.TrimPrefix(key, "app/")] = *id
	}
	return found, nil
}

func itadOverview(client *http.Client, apiKey string, ids []string) ([]itadPriceEntry, error) {
	body, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/games/overview/v2?key=%s&country=US", itadBase, apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var parsed itadOverviewResp
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	return parsed.Prices, nil
}
