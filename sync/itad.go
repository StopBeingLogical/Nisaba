package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"nisaba/db"
)

const itadBase = "https://api.isthereanydeal.com"

// PricingResult summarises an ITAD pricing sync run.
type PricingResult struct {
	Updated  int
	NotFound int
	Errors   []string
}

// SyncITADPricing looks up ITAD game IDs for all wishlist entries (cached after
// first run) then batch-fetches current best price and historical low.
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
	// Count how many actually need lookup so the counter is meaningful.
	var needLookup int
	for i := range entries {
		if entries[i].ITADId == "" && entries[i].SteamAppID != "" {
			needLookup++
		}
	}

	ticker := time.NewTicker(500 * time.Millisecond) // ~2 req/sec
	defer ticker.Stop()

	looked := 0
	for i := range entries {
		if entries[i].ITADId != "" || entries[i].SteamAppID == "" {
			continue
		}
		prog("ITAD ID lookup", looked, needLookup)
		<-ticker.C
		itadID, found, err := itadLookup(client, apiKey, entries[i].SteamAppID)
		looked++
		if err != nil {
			log.Printf("itad lookup %s: %v", entries[i].SteamAppID, err)
			result.Errors = append(result.Errors, fmt.Sprintf("lookup %s: %v", entries[i].SteamAppID, err))
			continue
		}
		if !found {
			result.NotFound++
			continue
		}
		entries[i].ITADId = itadID
		if err := store.SetWishlistITADId(entries[i].ID, itadID); err != nil {
			log.Printf("itad set id %s: %v", entries[i].ID, err)
		}
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

type itadLookupResp struct {
	Game  struct{ ID string `json:"id"` } `json:"game"`
	Found bool                            `json:"found"`
}

type itadPriceEntry struct {
	ID      string `json:"id"`
	Current *struct {
		Shop  struct{ Name string `json:"name"` } `json:"shop"`
		Price struct{ Amount float64 `json:"amount"` } `json:"price"`
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

func itadLookup(client *http.Client, apiKey, steamAppID string) (id string, found bool, err error) {
	url := fmt.Sprintf("%s/games/lookup/v1?key=%s&appid=%s", itadBase, apiKey, steamAppID)
	resp, err := client.Get(url)
	if err != nil {
		return "", false, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return "", false, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var parsed itadLookupResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", false, err
	}
	return parsed.Game.ID, parsed.Found, nil
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
