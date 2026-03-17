package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"nisaba/db"
)

// deckEntry holds an item (game or wishlist) needing a deck status check.
type deckEntry struct {
	id        string
	steamID   string
	isWishlist bool
}

// SyncSteamDeckStatus fetches Steam Deck compatibility for all Steam games
// and wishlist entries that don't yet have a status. Uses the correct
// saleaction endpoint. Runs up to 5 concurrent requests with a shared
// 5 req/sec rate limit.
func SyncSteamDeckStatus(store *db.Store, progress func(done, total int)) (int, error) {
	games, err := store.ListGamesNeedingDeckStatus()
	if err != nil {
		return 0, fmt.Errorf("list games: %w", err)
	}
	wishlist, err := store.ListWishlistNeedingDeckStatus()
	if err != nil {
		return 0, fmt.Errorf("list wishlist: %w", err)
	}

	var entries []deckEntry
	for _, g := range games {
		entries = append(entries, deckEntry{id: g.ID, steamID: g.SteamAppID, isWishlist: false})
	}
	for _, w := range wishlist {
		entries = append(entries, deckEntry{id: w.ID, steamID: w.SteamAppID, isWishlist: true})
	}

	total := len(entries)
	if total == 0 {
		return 0, nil
	}

	client := &http.Client{Timeout: 15 * time.Second}

	// Shared rate limiter: 5 requests/sec via a ticker feeding a buffered channel.
	const concurrency = 5
	tokens := make(chan struct{}, concurrency)
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond) // 5/sec
		defer ticker.Stop()
		for range ticker.C {
			tokens <- struct{}{}
		}
	}()

	type result struct {
		entry  deckEntry
		status string
	}
	results := make(chan result, total)

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	for _, e := range entries {
		wg.Add(1)
		e := e
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			<-tokens // rate-limit slot
			status := fetchDeckCompatibility(client, e.steamID)
			results <- result{entry: e, status: status}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	updated := 0
	done := 0
	for r := range results {
		var writeErr error
		if r.entry.isWishlist {
			writeErr = store.SetWishlistDeckVerified(r.entry.id, r.status)
		} else {
			writeErr = store.SetSteamDeckVerified(r.entry.id, r.status)
		}
		if writeErr == nil && (r.status == "verified" || r.status == "playable" || r.status == "unsupported") {
			updated++
		}
		done++
		if progress != nil {
			progress(done, total)
		}
	}
	return updated, nil
}

// fetchDeckCompatibility returns the Steam Deck status for a single app.
// Uses the saleaction endpoint which is the only one that actually returns
// deck compatibility data. Falls back to "unknown" on any error or no data.
func fetchDeckCompatibility(client *http.Client, appID string) string {
	url := "https://store.steampowered.com/saleaction/ajaxgetdeckappcompatibilityreport?nAppID=" + appID
	resp, err := client.Get(url)
	if err != nil {
		return "unknown"
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "unknown"
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "unknown"
	}

	var parsed struct {
		Success int `json:"success"`
		Results struct {
			Category int `json:"resolved_category"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Success != 1 {
		return "unknown"
	}

	switch parsed.Results.Category {
	case 3:
		return "verified"
	case 2:
		return "playable"
	case 1:
		return "unsupported"
	default:
		return "unknown"
	}
}

// Legacy single-fetch kept for reference; no longer called directly.
func fetchDeckStatus(client *http.Client, appID string) (string, error) {
	return fetchDeckCompatibility(client, appID), nil
}
