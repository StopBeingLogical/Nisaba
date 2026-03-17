package sync

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"nisaba/db"
)

// steamOwnedGamesResp is the IPlayerService/GetOwnedGames response envelope.
type steamOwnedGamesResp struct {
	Response struct {
		GameCount int         `json:"game_count"`
		Games     []steamGame `json:"games"`
	} `json:"response"`
}

type steamGame struct {
	AppID                  int    `json:"appid"`
	Name                   string `json:"name"`
	PlaytimeForever        int    `json:"playtime_forever"`
	PlaytimeWindowsForever int    `json:"playtime_windows_forever"`
	PlaytimeMacForever     int    `json:"playtime_mac_forever"`
	PlaytimeLinuxForever   int    `json:"playtime_linux_forever"`
	RtimeLastPlayed        int64  `json:"rtime_last_played"`
	ImgIconURL             string `json:"img_icon_url"`
}

// SyncSteamOwnership fetches the Steam library for the configured user and
// upserts ownership records. New games are inserted with enrichment_status
// 'needs_review' and artwork pre-populated from Steam CDN URLs.
func SyncSteamOwnership(store *db.Store) (ImportResult, error) {
	result := ImportResult{Store: "steam"}

	apiKey, err := store.GetConfig("steam.api_key")
	if err != nil || apiKey == "" {
		return result, fmt.Errorf("steam.api_key not configured — set it in Settings")
	}
	steamID, err := store.GetConfig("steam.user_id")
	if err != nil || steamID == "" {
		return result, fmt.Errorf("steam.user_id not configured — set it in Settings")
	}

	url := fmt.Sprintf(
		"https://api.steampowered.com/IPlayerService/GetOwnedGames/v1/?key=%s&steamid=%s&include_appinfo=1&include_played_free_games=1&format=json",
		apiKey, steamID,
	)

	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return result, fmt.Errorf("steam API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("steam API returned HTTP %d", resp.StatusCode)
	}

	var data steamOwnedGamesResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return result, fmt.Errorf("decode steam response: %w", err)
	}

	if len(data.Response.Games) == 0 {
		return result, fmt.Errorf("no games returned — check API key, Steam ID, and that the profile is public")
	}

	for _, g := range data.Response.Games {
		if g.Name == "" || g.AppID == 0 {
			result.Skipped++
			continue
		}

		appIDStr := strconv.Itoa(g.AppID)
		storeURL := fmt.Sprintf("https://store.steampowered.com/app/%d", g.AppID)

		// Steam CDN portrait cover (600×900) — best fit for the library grid.
		coverURL := fmt.Sprintf(
			"https://cdn.cloudflare.steamstatic.com/steam/apps/%d/library_600x900.jpg",
			g.AppID,
		)
		artwork := buildArtwork(coverURL, coverURL, "", "", "", "steam")
		artJSON, _ := json.Marshal(artwork)

		gameID, err := store.FindGameByStoreID("steam", appIDStr)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", g.Name, err))
			continue
		}

		if gameID == "" {
			gameID = uuid.New().String()
			if err := store.InsertGame(db.InsertGameParams{
				ID:          gameID,
				Title:       g.Name,
				SortTitle:   makeSortTitle(g.Name),
				ArtworkJSON: string(artJSON),
				Windows:     true,
			}); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: insert: %v", g.Name, err))
				continue
			}
			result.Added++
		}

		if err := store.UpsertGameStoreLink(gameID, "steam", appIDStr, storeURL); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: store link: %v", g.Name, err))
		}

		// Persist playtime — overwrite only if Steam reports more time played.
		if g.PlaytimeForever > 0 {
			if err := store.UpdatePlayTimeIfGreater(gameID, g.PlaytimeForever); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: playtime: %v", g.Name, err))
			}
		}
		// Persist last_played — overwrite only if Steam reports a later timestamp.
		if g.RtimeLastPlayed > 0 {
			t := time.Unix(g.RtimeLastPlayed, 0)
			if err := store.UpdateLastPlayedIfLater(gameID, t); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: last_played: %v", g.Name, err))
			}
		}
	}

	return result, nil
}
