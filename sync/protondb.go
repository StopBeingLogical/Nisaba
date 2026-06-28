package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"nisaba/db"
)

// ProtonResult summarises a ProtonDB rating sync run.
type ProtonResult struct {
	Updated  int
	NotFound int
	Errors   []string
}

// SyncProtonRatings fetches ProtonDB community ratings for all Steam-owned
// games that don't yet have a stored rating. Runs at 2 req/sec.
// progress(done, total) is called after each game; may be nil.
func SyncProtonRatings(store *db.Store, progress func(done, total int)) (ProtonResult, error) {
	var result ProtonResult

	games, err := store.ListGamesNeedingProtonRating()
	if err != nil {
		return result, fmt.Errorf("listing games: %w", err)
	}
	if len(games) == 0 {
		return result, nil
	}

	client := &http.Client{Timeout: 15 * time.Second}
	ticker := time.NewTicker(500 * time.Millisecond) // 2 req/sec
	defer ticker.Stop()

	total := len(games)
	for i, g := range games {
		<-ticker.C
		tier, found, err := fetchProtonTier(client, g.SteamAppID)
		if err != nil {
			log.Printf("protondb %s: %v", g.SteamAppID, err)
			result.Errors = append(result.Errors, fmt.Sprintf("app %s: %v", g.SteamAppID, err))
		} else if !found {
			result.NotFound++
		} else if err := store.SetProtonRating(g.ID, tier); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("app %s set: %v", g.SteamAppID, err))
		} else {
			result.Updated++
		}
		if progress != nil {
			progress(i+1, total)
		}
	}
	return result, nil
}

type protonSummary struct {
	Tier string `json:"tier"`
}

// fetchProtonTier calls ProtonDB's public summary endpoint for one Steam app.
// Returns the tier string, a found flag, and any error.
// Tiers: "platinum", "gold", "silver", "bronze", "borked", "native".
func fetchProtonTier(client *http.Client, steamAppID string) (string, bool, error) {
	url := fmt.Sprintf(
		"https://www.protondb.com/api/v1/reports/summaries/%s.json",
		steamAppID,
	)
	resp, err := client.Get(url)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", false, nil // no reports yet
	}
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, err
	}
	var s protonSummary
	if err := json.Unmarshal(body, &s); err != nil {
		return "", false, err
	}
	if s.Tier == "" {
		return "", false, nil
	}
	return s.Tier, true, nil
}
