package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"nisaba/db"
)

// IGDBClient handles authenticated requests to the IGDB API.
// Access tokens are cached in the struct — reuse the same instance.
type IGDBClient struct {
	clientID     string
	clientSecret string
	accessToken  string
	tokenExpiry  time.Time
	http         *http.Client
}

func NewIGDBClient(clientID, clientSecret string) *IGDBClient {
	return &IGDBClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		http:         &http.Client{Timeout: 15 * time.Second},
	}
}

type twitchTokenResp struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

func (c *IGDBClient) ensureToken() error {
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return nil
	}
	url := fmt.Sprintf(
		"https://id.twitch.tv/oauth2/token?client_id=%s&client_secret=%s&grant_type=client_credentials",
		c.clientID, c.clientSecret,
	)
	resp, err := c.http.Post(url, "application/x-www-form-urlencoded", nil)
	if err != nil {
		return fmt.Errorf("twitch token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("twitch token HTTP %d: %s", resp.StatusCode, b)
	}
	var t twitchTokenResp
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return fmt.Errorf("twitch token decode: %w", err)
	}
	c.accessToken = t.AccessToken
	// Subtract 5 minutes as a safety buffer
	c.tokenExpiry = time.Now().Add(time.Duration(t.ExpiresIn-300) * time.Second)
	return nil
}

// IGDBGame is a single result from the /games endpoint.
type IGDBGame struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Summary          string `json:"summary"`
	FirstReleaseDate int64  `json:"first_release_date"`
	URL              string `json:"url"`
	Cover            *struct {
		URL string `json:"url"`
	} `json:"cover"`
	Genres []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Platforms []struct {
		ID int `json:"id"`
	} `json:"platforms"`
}

// HasPCPlatform returns true if this game entry includes PC (Windows), platform ID 6.
func (g IGDBGame) HasPCPlatform() bool {
	for _, p := range g.Platforms {
		if p.ID == 6 {
			return true
		}
	}
	return false
}

func (g IGDBGame) CoverURL() string {
	if g.Cover == nil || g.Cover.URL == "" {
		return ""
	}
	url := strings.ReplaceAll(g.Cover.URL, "t_thumb", "t_cover_big_2x")
	if strings.HasPrefix(url, "//") {
		url = "https:" + url
	}
	return url
}

func (g IGDBGame) ReleaseYear() string {
	if g.FirstReleaseDate == 0 {
		return ""
	}
	return time.Unix(g.FirstReleaseDate, 0).UTC().Format("2006")
}

func (g IGDBGame) ReleaseDate() string {
	if g.FirstReleaseDate == 0 {
		return ""
	}
	return time.Unix(g.FirstReleaseDate, 0).UTC().Format("2006-01-02")
}

func (g IGDBGame) GenreNames() []string {
	names := make([]string, 0, len(g.Genres))
	for _, genre := range g.Genres {
		names = append(names, genre.Name)
	}
	return names
}

func (c *IGDBClient) query(body string) ([]IGDBGame, error) {
	if err := c.ensureToken(); err != nil {
		return nil, err
	}
	req, _ := http.NewRequest("POST", "https://api.igdb.com/v4/games",
		bytes.NewBufferString(body))
	req.Header.Set("Client-ID", c.clientID)
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("IGDB HTTP %d: %s", resp.StatusCode, b)
	}
	var results []IGDBGame
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}
	return results, nil
}

const igdbFields = "id,name,cover.url,genres.name,summary,first_release_date,url,platforms.id"

// FetchSteamAppIDs queries the IGDB /games endpoint for the given IGDB game
// IDs, inspects the websites field for Steam store URLs, and returns a map of
// igdb_id → Steam App ID string. IDs are chunked into batches of 500.
// Steam URLs follow the pattern: store.steampowered.com/app/NNNNN
func (c *IGDBClient) FetchSteamAppIDs(igdbIDs []int64) (map[int64]string, error) {
	if err := c.ensureToken(); err != nil {
		return nil, err
	}
	result := make(map[int64]string, len(igdbIDs))
	const batchSize = 500
	for i := 0; i < len(igdbIDs); i += batchSize {
		end := i + batchSize
		if end > len(igdbIDs) {
			end = len(igdbIDs)
		}
		batch := igdbIDs[i:end]

		idStrs := make([]string, len(batch))
		for j, id := range batch {
			idStrs[j] = fmt.Sprintf("%d", id)
		}
		body := fmt.Sprintf(
			"fields id,websites.url; where id = (%s); limit %d;",
			strings.Join(idStrs, ","), len(batch),
		)

		req, _ := http.NewRequest("POST", "https://api.igdb.com/v4/games",
			bytes.NewBufferString(body))
		req.Header.Set("Client-ID", c.clientID)
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
		req.Header.Set("Content-Type", "text/plain")

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("igdb games: %w", err)
		}
		body2, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("igdb games HTTP %d: %s", resp.StatusCode, body2)
		}

		var games []struct {
			ID       int64 `json:"id"`
			Websites []struct {
				URL string `json:"url"`
			} `json:"websites"`
		}
		if err := json.Unmarshal(body2, &games); err != nil {
			return nil, fmt.Errorf("igdb games decode: %w", err)
		}
		for _, g := range games {
			for _, w := range g.Websites {
				if appID := extractSteamAppID(w.URL); appID != "" {
					result[g.ID] = appID
					break
				}
			}
		}
	}
	return result, nil
}

// extractSteamAppID parses a Steam store URL and returns the App ID string,
// or "" if the URL is not a Steam store page.
// Handles: https://store.steampowered.com/app/1794680/...
func extractSteamAppID(u string) string {
	const prefix = "store.steampowered.com/app/"
	idx := strings.Index(u, prefix)
	if idx < 0 {
		return ""
	}
	rest := u[idx+len(prefix):]
	// rest is "1794680/Game_Title" or "1794680" — take up to first slash or end
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		rest = rest[:slash]
	}
	// Validate it's all digits
	for _, c := range rest {
		if c < '0' || c > '9' {
			return ""
		}
	}
	if rest == "" {
		return ""
	}
	return rest
}

// SyncSteamCrossRefs queries IGDB for Steam App IDs for all non-Steam library
// games that have an IGDB ID, and inserts cross-reference rows into game_stores
// so that the deck status sync can check them.
//
// Strategy:
// 1. Batch-fetch websites for all stored igdb_ids; extract Steam App IDs.
// 2. For games whose stored igdb_id had no Steam website (likely wrong platform
//    variant matched during enrichment), do a title search filtered to PC
//    platform (id=6) and re-extract from that result.
func SyncSteamCrossRefs(store *db.Store, igdbClient *IGDBClient, progress func(done, total int)) (int, error) {
	games, err := store.ListGamesNeedingSteamCrossRef()
	if err != nil {
		return 0, fmt.Errorf("list games: %w", err)
	}
	total := len(games)
	if total == 0 {
		return 0, nil
	}

	igdbIDs := make([]int64, len(games))
	idToRow := make(map[int64]db.SteamCrossRefRow, len(games))
	for i, g := range games {
		igdbIDs[i] = g.IGDBId
		idToRow[g.IGDBId] = g
	}

	// Pass 1: batch websites lookup for stored igdb_ids.
	steamMap, err := igdbClient.FetchSteamAppIDs(igdbIDs)
	if err != nil {
		return 0, fmt.Errorf("fetch steam ids: %w", err)
	}

	inserted := 0
	var needsFallback []db.SteamCrossRefRow

	for _, g := range games {
		if appID, ok := steamMap[g.IGDBId]; ok {
			if err := store.UpsertSteamCrossRef(g.ID, appID); err == nil {
				inserted++
			}
		} else {
			needsFallback = append(needsFallback, g)
		}
	}

	// Pass 2: title-based search for games whose stored igdb_id had no Steam
	// website — the enrichment likely matched a mobile/console variant.
	for _, g := range needsFallback {
		results, err := igdbClient.SearchGamePC(g.Title)
		if err != nil || len(results) == 0 {
			continue
		}
		match := bestMatch(g.Title, results)
		if match == nil {
			continue
		}
		// Fetch websites for this new igdb_id.
		newMap, err := igdbClient.FetchSteamAppIDs([]int64{match.ID})
		if err != nil {
			continue
		}
		if appID, ok := newMap[match.ID]; ok {
			if err := store.UpsertSteamCrossRef(g.ID, appID); err == nil {
				inserted++
			}
		}
		time.Sleep(250 * time.Millisecond) // be kind to IGDB rate limits
	}

	if progress != nil {
		progress(total, total)
	}
	return inserted, nil
}

// SearchGame returns up to 5 IGDB results for the given title.
func (c *IGDBClient) SearchGame(title string) ([]IGDBGame, error) {
	escaped := strings.ReplaceAll(title, `"`, `\"`)
	body := fmt.Sprintf(`search "%s"; fields %s; limit 5;`, escaped, igdbFields)
	return c.query(body)
}

// SearchGamePC returns up to 5 IGDB results for the given title, filtered to
// games that include PC (Windows) as a platform (platform id = 6).
func (c *IGDBClient) SearchGamePC(title string) ([]IGDBGame, error) {
	escaped := strings.ReplaceAll(title, `"`, `\"`)
	body := fmt.Sprintf(
		`search "%s"; fields %s; where platforms = (6); limit 5;`,
		escaped, igdbFields,
	)
	return c.query(body)
}

// bestMatch returns the best IGDB result for the given title. Among exact
// name matches, PC (Windows, platform 6) entries are preferred over other
// platforms to avoid matching mobile/console-only variants.
func bestMatch(title string, results []IGDBGame) *IGDBGame {
	norm := normalizeTitle(title)
	var firstMatch *IGDBGame
	for i := range results {
		if normalizeTitle(results[i].Name) != norm {
			continue
		}
		if results[i].HasPCPlatform() {
			return &results[i]
		}
		if firstMatch == nil {
			firstMatch = &results[i]
		}
	}
	return firstMatch
}

func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	// Strip leading articles
	for _, pfx := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(s, pfx) {
			s = s[len(pfx):]
		}
	}
	// Keep only letters, digits, and spaces
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			return r
		}
		return -1
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// ── Bulk enrichment ───────────────────────────────────────────────────────────

// EnrichProgress is updated by EnrichLibrary as it runs.
type EnrichProgress struct {
	Total   int
	Done    int
	Matched int
	Errors  int
}

// EnrichLibrary processes all needs_review games, searching IGDB for each.
// If rawg is non-nil, it is tried as a fallback when IGDB finds no match.
// progressFn is called after every game so callers can track progress.
// Rate-limited to ~4 req/sec to respect IGDB limits.
func EnrichLibrary(store *db.Store, client *IGDBClient, progressFn func(EnrichProgress), rawg ...*RAWGClient) error {
	var rawgClient *RAWGClient
	if len(rawg) > 0 {
		rawgClient = rawg[0]
	}
	games, err := store.ListNeedsReviewTitles()
	if err != nil {
		return fmt.Errorf("list games: %w", err)
	}

	p := EnrichProgress{Total: len(games)}
	ticker := time.NewTicker(250 * time.Millisecond) // 4 req/sec
	defer ticker.Stop()

	for _, g := range games {
		<-ticker.C

		results, err := client.SearchGame(g.Title)
		if err != nil {
			p.Errors++
			p.Done++
			if progressFn != nil {
				progressFn(p)
			}
			continue
		}

		match := bestMatch(g.Title, results)
		if match == nil {
			// IGDB miss — try RAWG fallback
			if rawgClient != nil {
				rawgMatch, _ := rawgClient.SearchGame(g.Title)
				if rawgMatch != nil {
					artJSON, _ := json.Marshal(buildArtwork(rawgMatch.BackgroundImage, rawgMatch.BackgroundImage, "", "", "", "rawg"))
					var rd *string
					if rawgMatch.Released != "" {
						rd = &rawgMatch.Released
					}
					if err := store.EnrichGame(db.EnrichGameParams{
						ID:          g.ID,
						ArtworkJSON: string(artJSON),
						ReleaseDate: rd,
					}); err != nil {
						p.Errors++
					} else {
						p.Matched++
						for _, genre := range rawgMatch.GenreNames() {
							_ = store.UpsertGenre(g.ID, genre)
						}
					}
					p.Done++
					if progressFn != nil {
						progressFn(p)
					}
					continue
				}
			}
			p.Done++
			if progressFn != nil {
				progressFn(p)
			}
			continue
		}

		// Build cover artwork from IGDB
		coverURL := match.CoverURL()
		artJSON, _ := json.Marshal(buildArtwork(coverURL, coverURL, "", "", "", "igdb"))

		var summary, releaseDate *string
		if match.Summary != "" {
			s := match.Summary
			summary = &s
		}
		if rd := match.ReleaseDate(); rd != "" {
			releaseDate = &rd
		}

		if err := store.EnrichGame(db.EnrichGameParams{
			ID:          g.ID,
			IGDBId:      match.ID,
			ArtworkJSON: string(artJSON),
			Description: summary,
			ReleaseDate: releaseDate,
		}); err != nil {
			p.Errors++
		} else {
			p.Matched++
			for _, genre := range match.GenreNames() {
				_ = store.UpsertGenre(g.ID, genre)
			}
		}

		p.Done++
		if progressFn != nil {
			progressFn(p)
		}
	}

	return nil
}

// EnrichWishlist processes all needs_review wishlist entries, searching IGDB for each.
// If rawg is non-nil, it is tried as a fallback when IGDB finds no match.
func EnrichWishlist(store *db.Store, client *IGDBClient, progressFn func(EnrichProgress), rawg ...*RAWGClient) error {
	var rawgClient *RAWGClient
	if len(rawg) > 0 {
		rawgClient = rawg[0]
	}
	entries, err := store.ListWishlistNeedsEnrichment()
	if err != nil {
		return fmt.Errorf("list wishlist: %w", err)
	}

	p := EnrichProgress{Total: len(entries)}
	ticker := time.NewTicker(250 * time.Millisecond) // 4 req/sec
	defer ticker.Stop()

	for _, g := range entries {
		<-ticker.C

		results, err := client.SearchGame(g.Title)
		if err != nil {
			p.Errors++
			p.Done++
			if progressFn != nil {
				progressFn(p)
			}
			continue
		}

		match := bestMatch(g.Title, results)
		if match == nil {
			if rawgClient != nil {
				rawgMatch, _ := rawgClient.SearchGame(g.Title)
				if rawgMatch != nil {
					artJSON, _ := json.Marshal(buildArtwork(rawgMatch.BackgroundImage, rawgMatch.BackgroundImage, "", "", "", "rawg"))
					if err := store.EnrichWishlistEntry(db.EnrichGameParams{
						ID:          g.ID,
						ArtworkJSON: string(artJSON),
					}); err != nil {
						p.Errors++
					} else {
						p.Matched++
					}
					p.Done++
					if progressFn != nil {
						progressFn(p)
					}
					continue
				}
			}
			p.Done++
			if progressFn != nil {
				progressFn(p)
			}
			continue
		}

		coverURL := match.CoverURL()
		artJSON, _ := json.Marshal(buildArtwork(coverURL, coverURL, "", "", "", "igdb"))

		if err := store.EnrichWishlistEntry(db.EnrichGameParams{
			ID:          g.ID,
			IGDBId:      match.ID,
			ArtworkJSON: string(artJSON),
		}); err != nil {
			p.Errors++
		} else {
			p.Matched++
		}

		p.Done++
		if progressFn != nil {
			progressFn(p)
		}
	}

	return nil
}
