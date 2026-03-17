package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"nisaba/db"
)

// GOGClientID is the GOG Galaxy public client ID.
const GOGClientID = "46899977096215655"

// SyncGOGWishlist fetches the authenticated GOG wishlist and upserts entries.
func SyncGOGWishlist(store *db.Store) (WishlistResult, error) {
	result := WishlistResult{Store: "gog"}

	accessToken, err := gogGetAccessToken(store)
	if err != nil {
		return result, err
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// ── Step 2: fetch wishlist ────────────────────────────────────────────────
	wishlistIDs, err := fetchGOGWishlist(client, accessToken)
	if err != nil {
		return result, fmt.Errorf("GOG wishlist fetch: %w", err)
	}
	if len(wishlistIDs) == 0 {
		return result, nil
	}

	// ── Step 3: fetch titles and upsert ──────────────────────────────────────
	keepIDs := make([]string, 0, len(wishlistIDs))
	for _, gogID := range wishlistIDs {
		id := "gog-wish-" + gogID
		keepIDs = append(keepIDs, id)

		title, err := fetchGOGProductTitle(client, gogID)
		if err != nil || title == "" {
			title = fmt.Sprintf("GOG Product %s", gogID)
		}

		sortTitle := makeSortTitle(title)
		if err := store.UpsertWishlistEntry(id, title, sortTitle); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", gogID, err))
			continue
		}
		storeURL := "https://www.gog.com/game/" + gogID
		if err := store.UpsertWishlistStoreLink(id, "gog", gogID, storeURL); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s store link: %v", gogID, err))
		}
		result.Added++
	}

	// ── Step 4: remove stale entries ─────────────────────────────────────────
	removed, err := store.DeleteStaleWishlistEntries("gog-wish-", keepIDs)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("cleanup: %v", err))
	}
	result.Removed = removed

	return result, nil
}

// gogGetAccessToken returns the stored access token if still valid.
func gogGetAccessToken(store *db.Store) (string, error) {
	accessToken, err := store.GetConfig("gog.access_token")
	if err != nil || accessToken == "" {
		return "", fmt.Errorf("GOG not configured — paste auth.json in Settings")
	}

	expiresStr, _ := store.GetConfig("gog.access_token_expires")
	if expiresStr != "" {
		if exp, err := strconv.ParseInt(expiresStr, 10, 64); err == nil {
			if time.Now().Unix() >= exp {
				return "", fmt.Errorf("GOG access token expired — re-paste auth.json in Settings")
			}
		}
	}

	return accessToken, nil
}

type gogWishlistResponse struct {
	Wishlist map[string]bool `json:"wishlist"`
}

func fetchGOGWishlist(client *http.Client, accessToken string) ([]string, error) {
	req, _ := http.NewRequest("GET", "https://embed.gog.com/user/wishlist.json", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("wishlist HTTP %d: %s", resp.StatusCode, b)
	}
	var parsed gogWishlistResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	var ids []string
	for id, wanted := range parsed.Wishlist {
		if wanted {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

type gogProductResponse struct {
	Title string `json:"title"`
}

func fetchGOGProductTitle(client *http.Client, gogID string) (string, error) {
	resp, err := client.Get("https://api.gog.com/products/" + gogID)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("product HTTP %d", resp.StatusCode)
	}
	var p gogProductResponse
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return "", err
	}
	return p.Title, nil
}
