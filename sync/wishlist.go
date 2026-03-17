package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nisaba/db"
)

// WishlistResult summarises one store's wishlist import.
type WishlistResult struct {
	Store   string
	Added   int
	Removed int
	Errors  []string
}

// SyncSteamWishlist fetches the Steam wishlist via the authenticated Web API
// (works regardless of profile privacy setting).
func SyncSteamWishlist(store *db.Store) (WishlistResult, error) {
	result := WishlistResult{Store: "steam"}

	apiKey, err := store.GetConfig("steam.api_key")
	if err != nil || apiKey == "" {
		return result, fmt.Errorf("steam.api_key not configured — set it in Settings")
	}
	steamID, err := store.GetConfig("steam.user_id")
	if err != nil || steamID == "" {
		return result, fmt.Errorf("steam.user_id not configured — set it in Settings")
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// ── Step 1: fetch wishlist app IDs from official API ─────────────────────
	items, err := fetchWishlistItems(client, apiKey, steamID)
	if err != nil {
		return result, err
	}
	if len(items) == 0 {
		result.Added = 0
		return result, nil
	}

	// ── Step 2: collect all app IDs ──────────────────────────────────────────
	appIDs := make([]string, 0, len(items))
	for _, item := range items {
		appIDs = append(appIDs, strconv.FormatInt(item.AppID, 10))
	}

	// ── Step 3: resolve titles from our library first (free) ─────────────────
	knownTitles, _ := store.GameTitlesByStoreID("steam", appIDs)

	// ── Step 4: batch-fetch names for games not already in library ───────────
	var unknown []string
	for _, id := range appIDs {
		if _, ok := knownTitles[id]; !ok {
			unknown = append(unknown, id)
		}
	}
	fetchedTitles := batchFetchSteamNames(client, unknown)

	// ── Step 5: upsert ───────────────────────────────────────────────────────
	keepIDs := make([]string, 0, len(items))
	for _, item := range items {
		appIDStr := strconv.FormatInt(item.AppID, 10)
		id := "steam-wish-" + appIDStr
		keepIDs = append(keepIDs, id)

		title := knownTitles[appIDStr]
		if title == "" {
			title = fetchedTitles[appIDStr]
		}
		if title == "" {
			title = "Steam App " + appIDStr
		}

		sortTitle := makeSortTitle(title)
		if err := store.UpsertWishlistEntry(id, title, sortTitle); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", appIDStr, err))
			continue
		}
		storeURL := "https://store.steampowered.com/app/" + appIDStr
		if err := store.UpsertWishlistStoreLink(id, "steam", appIDStr, storeURL); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s store link: %v", appIDStr, err))
		}
		result.Added++
	}

	// ── Step 6: remove stale entries ─────────────────────────────────────────
	removed, err := store.DeleteStaleWishlistEntries("steam-wish-", keepIDs)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("cleanup: %v", err))
	}
	result.Removed = removed

	return result, nil
}

// steamWishlistAPIResponse mirrors IWishlistService/GetWishlist/v1.
type steamWishlistAPIResponse struct {
	Response struct {
		Items []struct {
			AppID    int64 `json:"appid"`
			Priority int   `json:"priority"`
		} `json:"items"`
	} `json:"response"`
}

func fetchWishlistItems(client *http.Client, apiKey, steamID string) ([]struct {
	AppID    int64
	Priority int
}, error) {
	url := fmt.Sprintf(
		"https://api.steampowered.com/IWishlistService/GetWishlist/v1/?key=%s&steamid=%s",
		apiKey, steamID,
	)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("wishlist fetch: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("wishlist read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wishlist API returned HTTP %d", resp.StatusCode)
	}
	var parsed steamWishlistAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("wishlist parse: %w", err)
	}
	items := make([]struct {
		AppID    int64
		Priority int
	}, len(parsed.Response.Items))
	for i, it := range parsed.Response.Items {
		items[i].AppID = it.AppID
		items[i].Priority = it.Priority
	}
	return items, nil
}

// steamAppDetail is the per-app shape returned by store.steampowered.com/api/appdetails.
type steamAppDetail struct {
	Success bool `json:"success"`
	Data    struct {
		Name string `json:"name"`
	} `json:"data"`
}

// fetchSteamAppName fetches a single app name from the Steam store API.
func fetchSteamAppName(client *http.Client, appID string) string {
	url := "https://store.steampowered.com/api/appdetails?filters=basic&appids=" + appID
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	var raw map[string]steamAppDetail
	if err := json.Unmarshal(body, &raw); err != nil {
		return ""
	}
	if entry, ok := raw[appID]; ok && entry.Success {
		return entry.Data.Name
	}
	return ""
}

// batchFetchSteamNames tries a batch appdetails request first; if it returns
// nothing (Steam sometimes blocks batches), falls back to individual requests.
func batchFetchSteamNames(client *http.Client, appIDs []string) map[string]string {
	result := make(map[string]string, len(appIDs))
	if len(appIDs) == 0 {
		return result
	}

	// Try batch (undocumented but fast when it works).
	batchURL := "https://store.steampowered.com/api/appdetails?filters=basic&appids=" +
		strings.Join(appIDs, ",")
	req, _ := http.NewRequest("GET", batchURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if resp, err := client.Do(req); err == nil {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK && len(body) > 2 && body[0] == '{' {
			var raw map[string]steamAppDetail
			if json.Unmarshal(body, &raw) == nil {
				for id, entry := range raw {
					if entry.Success && entry.Data.Name != "" {
						result[id] = entry.Data.Name
					}
				}
			}
		}
	}

	// Fall back to individual requests for any still-missing IDs.
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for _, id := range appIDs {
		if result[id] != "" {
			continue
		}
		<-ticker.C
		if name := fetchSteamAppName(client, id); name != "" {
			result[id] = name
		}
	}

	// Final fallback: scrape the store page for games the API won't name
	// (unreleased / coming-soon / restricted apps where appdetails returns false).
	pageTicker := time.NewTicker(500 * time.Millisecond)
	defer pageTicker.Stop()
	for _, id := range appIDs {
		if result[id] != "" {
			continue
		}
		<-pageTicker.C
		if name := fetchSteamAppNameFromPage(client, id); name != "" {
			result[id] = name
		}
	}
	return result
}

// fetchSteamAppNameFromPage scrapes the Steam store page as a last resort.
// The appdetails API returns success:false for unreleased / coming-soon games,
// but the store page og:title meta tag still carries the game name.
// Forces US/English to avoid region pages, and bypasses age gates via cookies.
func fetchSteamAppNameFromPage(client *http.Client, appID string) string {
	// cc=US and l=english avoid region-specific error pages.
	req, err := http.NewRequest("GET", "https://store.steampowered.com/app/"+appID+"/?cc=US&l=english", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	// Bypass age-gate without browser interaction.
	req.Header.Set("Cookie", "birthtime=631152001; lastagecheckage=1-January-1990; wants_mature_content=1")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	defer resp.Body.Close()

	// Read the first 32 KB — og:title is always in <head> but Steam inlines
	// some scripts before it; 32 KB gives plenty of headroom.
	buf := make([]byte, 32768)
	n, _ := io.ReadFull(resp.Body, buf)
	return extractOGTitle(string(buf[:n]))
}

// extractOGTitle pulls the game name from a Steam og:title meta tag.
// Handles both attribute orderings:
//   <meta property="og:title" content="Name on Steam">
//   <meta content="Name on Steam" property="og:title">
// Steam title formats:
//   "Game Title on Steam"
//   "Save 75% on Game Title on Steam"
//   "Play Game Title"  (free-to-play)
func extractOGTitle(html string) string {
	title := ogTitleByProperty(html)
	if title == "" {
		title = ogTitleByContent(html)
	}
	if title == "" {
		return ""
	}

	// Strip " on Steam" suffix.
	title = strings.TrimSuffix(title, " on Steam")

	// Strip sale prefix "Save X% on ".
	if i := strings.Index(title, "% on "); i >= 0 {
		title = title[i+5:]
	}

	// Strip free-to-play prefix "Play ".
	title = strings.TrimPrefix(title, "Play ")

	return strings.TrimSpace(title)
}

// ogTitleByProperty finds og:title via property= attribute first.
func ogTitleByProperty(html string) string {
	idx := strings.Index(html, `property="og:title"`)
	if idx < 0 {
		return ""
	}
	rest := html[idx:]
	ci := strings.Index(rest, `content="`)
	if ci < 0 || ci > 200 { // content= must be on the same tag
		return ""
	}
	rest = rest[ci+len(`content="`):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// ogTitleByContent finds og:title via content= attribute first (reverse order).
func ogTitleByContent(html string) string {
	search := html
	for {
		ci := strings.Index(search, `content="`)
		if ci < 0 {
			return ""
		}
		search = search[ci+len(`content="`):]
		end := strings.Index(search, `"`)
		if end < 0 {
			return ""
		}
		val := search[:end]
		search = search[end:]
		// Check if property="og:title" follows within the same tag.
		tagEnd := strings.IndexByte(search, '>')
		if tagEnd < 0 {
			continue
		}
		if strings.Contains(search[:tagEnd], `property="og:title"`) {
			return val
		}
	}
}

