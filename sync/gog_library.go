package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"nisaba/db"
)

// gogLibraryURL is the account's library view: games only, 100 per page.
const gogLibraryURL = "https://embed.gog.com/account/getFilteredProducts"

type gogLibraryProduct struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	IsMovie bool   `json:"isMovie"`
	WorksOn struct {
		Windows bool `json:"Windows"`
		Mac     bool `json:"Mac"`
		Linux   bool `json:"Linux"`
	} `json:"worksOn"`
}

type gogFilteredProducts struct {
	TotalPages int                 `json:"totalPages"`
	Products   []gogLibraryProduct `json:"products"`
}

// SyncGOGLibrary reads the account's GOG library and upserts games and their
// gog store links. It pages the library view rather than the raw owned list:
// /user/data/games returns 1397 ids for this account, 358 of which are
// entitlements the library view does not show — packs, Prime Gaming and Luna
// rewards, and delisted products (proved 2026-09-16, evidence/GOGL-001.md).
func SyncGOGLibrary(store *db.Store) (ImportResult, error) {
	result := ImportResult{Store: "gog"}

	accessToken, err := gogGetAccessToken(store)
	if err != nil {
		return result, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	totalPages := 1
	for page := 1; page <= totalPages; page++ {
		body, status, err := gogLibraryPage(client, accessToken, page)
		if err != nil {
			return result, fmt.Errorf("GOG library page %d: %w", page, err)
		}
		if status != http.StatusOK {
			return result, fmt.Errorf("GOG library page %d: HTTP %d", page, status)
		}

		var parsed gogFilteredProducts
		if err := json.Unmarshal(body, &parsed); err != nil {
			return result, fmt.Errorf("GOG library page %d: %w", page, err)
		}
		if parsed.TotalPages > totalPages {
			totalPages = parsed.TotalPages
		}

		for _, p := range parsed.Products {
			if p.ID == 0 || p.Title == "" || p.IsMovie {
				result.Skipped++
				continue
			}

			storeID := strconv.FormatInt(p.ID, 10)
			gameID, err := store.FindGameByStoreID("gog", storeID)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", p.Title, err))
				continue
			}

			// Deduplicate by title before inserting, the same way the Playnite
			// sync does: a game owned on two stores is one game with two links.
			if gameID == "" {
				gameID, err = store.FindGameByTitle(p.Title)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", p.Title, err))
					continue
				}
			}

			if gameID == "" {
				gameID = uuid.New().String()
				if err := store.InsertGame(db.InsertGameParams{
					ID:          gameID,
					Title:       p.Title,
					SortTitle:   makeSortTitle(p.Title),
					ArtworkJSON: "{}",
					Windows:     p.WorksOn.Windows,
					Mac:         p.WorksOn.Mac,
					Linux:       p.WorksOn.Linux,
				}); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("%s: insert: %v", p.Title, err))
					continue
				}
				result.Added++
			}

			storeURL := ""
			if p.URL != "" {
				storeURL = "https://www.gog.com" + p.URL
			}
			if err := store.UpsertGameStoreLink(gameID, "gog", storeID, storeURL); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: store link: %v", p.Title, err))
			}
		}
	}

	return result, nil
}

// gogLibraryPage fetches one page of the library view. A 302 here means the
// access token was rejected — GOG redirects rather than answering 401.
func gogLibraryPage(client *http.Client, accessToken string, page int) ([]byte, int, error) {
	query := url.Values{
		"mediaType": {"1"},
		"page":      {strconv.Itoa(page)},
	}

	req, err := http.NewRequest("GET", gogLibraryURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}
