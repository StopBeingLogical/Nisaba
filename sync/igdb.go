package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	stdsync "sync"
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

	// The pacing below lives in the client rather than in each caller because one
	// search is no longer one request: it is a name-anchored lookup merged with a
	// ranked search, plus a head-only fallback form. Every caller paced itself at
	// 250ms per *game*, which after this change puts two to four times that rate on
	// the wire and collects 429s.
	throttleMu  stdsync.Mutex
	lastRequest time.Time
}

// minRequestInterval paces outgoing IGDB calls at the API's 4 requests per second
// budget, with a little headroom.
const minRequestInterval = 270 * time.Millisecond

// throttle blocks until the next request may be sent.
func (c *IGDBClient) throttle() {
	c.throttleMu.Lock()
	defer c.throttleMu.Unlock()
	if wait := minRequestInterval - time.Since(c.lastRequest); wait > 0 {
		time.Sleep(wait)
	}
	c.lastRequest = time.Now()
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
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"platforms"`
	InvolvedCompanies []struct {
		Developer bool `json:"developer"`
		Publisher bool `json:"publisher"`
		Company   *struct {
			Name string `json:"name"`
		} `json:"company"`
	} `json:"involved_companies"`
}

// DeveloperName returns the companies IGDB credits as developer, comma-joined.
// Empty when IGDB lists none, so callers can skip writing the column.
func (g IGDBGame) DeveloperName() string {
	var names []string
	for _, ic := range g.InvolvedCompanies {
		if ic.Developer && ic.Company != nil && ic.Company.Name != "" {
			names = append(names, ic.Company.Name)
		}
	}
	return strings.Join(names, ", ")
}

// PublisherName returns the companies IGDB credits as publisher, comma-joined.
// Empty when IGDB lists none.
func (g IGDBGame) PublisherName() string {
	var names []string
	for _, ic := range g.InvolvedCompanies {
		if ic.Publisher && ic.Company != nil && ic.Company.Name != "" {
			names = append(names, ic.Company.Name)
		}
	}
	return strings.Join(names, ", ")
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

// PlatformNames returns the platform names IGDB lists for the game. The review
// page shows these so a candidate can be judged without a second tab.
func (g IGDBGame) PlatformNames() []string {
	names := make([]string, 0, len(g.Platforms))
	for _, p := range g.Platforms {
		if p.Name != "" {
			names = append(names, p.Name)
		}
	}
	return names
}

func (c *IGDBClient) query(body string) ([]IGDBGame, error) {
	if err := c.ensureToken(); err != nil {
		return nil, err
	}
	c.throttle()
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

const igdbFields = "id,name,cover.url,genres.name,summary,first_release_date,url,platforms.id,platforms.name," +
	"involved_companies.company.name,involved_companies.developer,involved_companies.publisher"

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
//  1. Batch-fetch websites for all stored igdb_ids; extract Steam App IDs.
//  2. For games whose stored igdb_id had no Steam website (likely wrong platform
//     variant matched during enrichment), search by title and re-extract from
//     that result. The search no longer filters to PC (id=6): that filter
//     reshaped IGDB's ranking and hid exact entries, while bestMatch already
//     prefers a PC entry among exact names.
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
	// website — the enrichment likely matched a mobile/console variant. bestMatch
	// already prefers a PC entry among exact names, so the query no longer
	// filters by platform: doing that reshaped the ranking and hid exact entries.
	for _, g := range needsFallback {
		results, err := igdbClient.SearchGame(g.Title)
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

// Limits on the two search forms. Both were measured against the live API rather
// than chosen: the correct entry for "Against the Storm" sits at position 10 of
// the platform-filtered ranked search and position 14 unfiltered, so the limit of
// 5 this used to ask for truncated the answer away before the scorer saw it.
const (
	maxRankedResults   = 25
	maxAnchoredResults = 25
)

// SearchGame returns IGDB rows that may correspond to a store title.
//
// IGDB's `search` is not a substring matcher and not a relevance engine, and two
// measured properties of it shape this function:
//
//  1. It is a conjunction over *every* term, matched against the summary as well
//     as the name. So a title carrying a subtitle matches nothing at all
//     ("Fallout 2: A Post Nuclear Role Playing Game" returns an empty set), and a
//     title whose words occur in other games' blurbs returns that noise instead
//     ("Against the Storm" returns Life is Strange: Before the Storm, whose
//     summary says "against").
//  2. Its ranking is poor enough that the exact entry for a plain title can sit
//     well past the first few results.
//
// So a search is built from two sources rather than one. A name-anchored wildcard
// lookup cannot return summary noise and cannot be outranked, so its hits are
// merged ahead of the ranked ones; a title carrying a subtitle is retried on its
// head alone when the full form reaches nothing usable.
func (c *IGDBClient) SearchGame(title string) ([]IGDBGame, error) {
	forms := searchForms(title)
	var merged []IGDBGame
	seen := make(map[int64]bool)
	for i, q := range forms {
		anchored, err := c.searchByName(q)
		if err != nil {
			return nil, err
		}
		ranked, err := c.searchRanked(q)
		if err != nil {
			return nil, err
		}
		merged = appendUnique(merged, seen, anchored)
		merged = appendUnique(merged, seen, ranked)
		// The full title is the primary form and the heads are fallbacks, so they
		// cost requests only when the forms tried so far reached nothing confident.
		// The test is a confident score rather than "any score": a weak hit is
		// exactly what a title with a real match underneath it scores, and stopping
		// there would keep the wrong candidate and skip the query that finds the
		// right one.
		if i < len(forms)-1 && bestScore(title, merged) >= confidentScore {
			break
		}
	}
	return merged, nil
}

// searchForms lists the query forms to try, best first: the undecorated title,
// then its head cut at the *last* separator, then the same cut at the first.
//
// The two heads exist because store titles decorate front and back, and the two
// ends need opposite treatment. Cutting at the last separator drops a trailing
// decoration while keeping the game's own name, which is what "Warhammer 40,000:
// Dawn of War II - Anniversary Edition" needs — cutting at the first separator
// there yields "Warhammer 40,000", a franchise prefix, and the best it can then
// score is Dawn of War, the wrong game. Cutting at the first separator is what
// "Fallout 2: A Post Nuclear Role Playing Game" needs, where the tail is a
// description rather than a product name. The order matters only for the cost:
// the loop stops as soon as a form scores confidently, so the narrower head is
// asked for first.
func searchForms(title string) []string {
	cleaned := searchTitle(title)
	forms := []string{cleaned}
	add := func(f string) {
		if f == "" || f == cleaned {
			return
		}
		for _, existing := range forms {
			if existing == f {
				return
			}
		}
		forms = append(forms, f)
	}
	add(headSearchTitle(cleaned))
	add(broadHeadSearchTitle(cleaned))
	return forms
}

// headSearchTitle cuts at the last separator, keeping as much of the title as
// still looks like a product name. Two words are required, so "Batman: Arkham
// Asylum" is not shortened to the far too broad "Batman".
func headSearchTitle(s string) string {
	cut := -1
	for _, sep := range titleSeparators {
		if i := strings.LastIndex(s, sep); i > 0 && i > cut {
			cut = i
		}
	}
	return validHead(s, cut)
}

// broadHeadSearchTitle cuts at the first separator, dropping whatever subtitle or
// descriptive tail the store appended.
func broadHeadSearchTitle(s string) string {
	cut := -1
	for _, sep := range titleSeparators {
		if i := strings.Index(s, sep); i > 0 && (cut < 0 || i < cut) {
			cut = i
		}
	}
	return validHead(s, cut)
}

var titleSeparators = []string{" - ", " – ", ": "}

func validHead(s string, cut int) string {
	if cut < 0 {
		return ""
	}
	head := strings.TrimSpace(s[:cut])
	if len(strings.Fields(head)) < 2 {
		return ""
	}
	return head
}

// searchByName looks a title up by name rather than by relevance. The wildcard is
// case-insensitive, so unlike `where name = "..."` — which is case-*sensitive* and
// returns an empty set for the same name in different case — it cannot fail on
// capitalisation. Sorting by name keeps a title that *is* the query ahead of the
// entries that merely contain it, which is what holds the exact entry inside the
// limit for a query that matches many names.
func (c *IGDBClient) searchByName(q string) ([]IGDBGame, error) {
	body := anchoredBody(q)
	if body == "" {
		return nil, nil
	}
	return c.query(body)
}

func anchoredBody(q string) string {
	literal := wildcardLiteral(q)
	if literal == "" {
		return ""
	}
	return fmt.Sprintf(
		`where name ~ *"%s"*; fields %s; sort name asc; limit %d;`,
		literal, igdbFields, maxAnchoredResults,
	)
}

// searchRanked asks IGDB's own relevance search, which reaches variants a name
// lookup cannot (a store's "Battle Isle 2" to IGDB's "Battle Isle 2200").
func (c *IGDBClient) searchRanked(q string) ([]IGDBGame, error) {
	return c.query(rankedBody(q))
}

func rankedBody(q string) string {
	escaped := strings.ReplaceAll(q, `"`, `\"`)
	return fmt.Sprintf(`search "%s"; fields %s; limit %d;`, escaped, igdbFields, maxRankedResults)
}

// wildcardLiteral strips the characters that would end or unbalance the quoted
// wildcard literal. A removed character makes the anchor slightly looser, never
// wrong: scoring still runs against the untouched title afterwards.
func wildcardLiteral(s string) string {
	s = strings.NewReplacer(`\`, "", `"`, "", `*`, "", `%`, "").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

func appendUnique(dst []IGDBGame, seen map[int64]bool, src []IGDBGame) []IGDBGame {
	for _, g := range src {
		if seen[g.ID] {
			continue
		}
		seen[g.ID] = true
		dst = append(dst, g)
	}
	return dst
}

// RankSearchResults orders search hits so the closest name match to the stored
// title comes first, regardless of the order IGDB returned them in. The review
// page's search box uses it, so the entry that *is* the title being searched is
// the first thing on screen rather than whatever IGDB happened to rank first.
func RankSearchResults(title string, games []IGDBGame) []IGDBGame {
	type scored struct {
		game  IGDBGame
		score float64
	}
	ranked := make([]scored, 0, len(games))
	for _, g := range games {
		s, _ := scoreMatch(title, g.Name)
		ranked = append(ranked, scored{g, s})
	}
	sort.SliceStable(ranked, func(a, b int) bool { return ranked[a].score > ranked[b].score })
	out := make([]IGDBGame, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, r.game)
	}
	return out
}

// confidentScore is the tier at which a result is good enough to stop searching:
// the "contains" tier and above. A weak hit or a zero-scoring one still leaves the
// fallback forms worth their requests.
const confidentScore = 0.7

// bestScore reports the highest score any result reaches against the stored title.
func bestScore(title string, games []IGDBGame) float64 {
	best := 0.0
	for _, g := range games {
		if s, _ := scoreMatch(title, g.Name); s > best {
			best = s
		}
	}
	return best
}

// FetchGame loads a single IGDB entry by its id. The review page uses it after
// a manual search result is picked, so the stored candidate carries the full
// evidence rather than whatever the search response happened to include.
func (c *IGDBClient) FetchGame(igdbID int64) (*IGDBGame, error) {
	results, err := c.query(fmt.Sprintf(`fields %s; where id = %d; limit 1;`, igdbFields, igdbID))
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("IGDB has no game with id %d", igdbID)
	}
	return &results[0], nil
}

// CandidateFromGame is the one place an IGDB entry is turned into a review-queue
// candidate, so a searched candidate and a hand-picked one are stored identically.
func CandidateFromGame(gameID string, g IGDBGame, confidence string, score float64, inLibrary bool) db.MatchCandidate {
	return db.MatchCandidate{
		GameID:      gameID,
		IGDBID:      g.ID,
		IGDBName:    g.Name,
		CoverURL:    g.CoverURL(),
		ReleaseYear: g.ReleaseYear(),
		Confidence:  confidence,
		Score:       score,
		InLibrary:   inLibrary,
		Summary:     g.Summary,
		Genres:      strings.Join(g.GenreNames(), ", "),
		Platforms:   strings.Join(g.PlatformNames(), ", "),
		IGDBURL:     g.URL,
	}
}

// enrichFromIGDB writes one IGDB entry onto a game: the IGDB id, cover artwork,
// summary, release date, developer, publisher and genres, then marks the game
// 'matched' (which is what takes it out of the review queue).
//
// Both the enrichment pipeline and the review page's true-up go through this, so
// a match applied by hand is written identically to one found by a search and the
// two cannot drift apart.
func enrichFromIGDB(store *db.Store, gameID string, match IGDBGame) error {
	coverURL := match.CoverURL()
	artJSON, _ := json.Marshal(buildArtwork(coverURL, coverURL, "", "", "", "igdb"))

	var summary, releaseDate, developer, publisher *string
	if match.Summary != "" {
		s := match.Summary
		summary = &s
	}
	if rd := match.ReleaseDate(); rd != "" {
		releaseDate = &rd
	}
	if d := match.DeveloperName(); d != "" {
		developer = &d
	}
	if name := match.PublisherName(); name != "" {
		publisher = &name
	}

	if err := store.EnrichGame(db.EnrichGameParams{
		ID:          gameID,
		IGDBId:      match.ID,
		ArtworkJSON: string(artJSON),
		Description: summary,
		ReleaseDate: releaseDate,
		Developer:   developer,
		Publisher:   publisher,
	}); err != nil {
		return err
	}
	for _, genre := range match.GenreNames() {
		_ = store.UpsertGenre(gameID, genre)
	}
	return nil
}

// maxStoredCandidates bounds the shortlist kept per game. Three are shown (the
// primary plus two alternates); a couple spare leaves room for rejections without
// needing another search.
const maxStoredCandidates = 5

// rankCandidates scores every IGDB result for a title and returns the usable ones
// best-first, skipping entries the owner has already rejected for this game.
//
// The exclusion is what makes "try again" meaningful: the search is deterministic,
// so without it a re-match would offer back the exact candidate it was just told to
// discard.
//
// Returning the whole ranking, not just the winner, is what lets the review page
// offer a shortlist: measured against live, re-searching after a rejection never
// once produced a better-tier match, so the scorer's ranking past the first entry
// is not trustworthy enough to pick from on the owner's behalf.
func rankCandidates(title, gameID string, results []IGDBGame, known, rejected map[int64]bool) []db.MatchCandidate {
	type scored struct {
		candidate db.MatchCandidate
		score     float64
	}
	ranked := make([]scored, 0, len(results))
	for i := range results {
		if rejected[results[i].ID] {
			continue
		}
		score, label := scoreMatch(title, results[i].Name)
		if score <= 0 {
			continue
		}
		ranked = append(ranked, scored{
			candidate: CandidateFromGame(gameID, results[i], label, score, known[results[i].ID]),
			score:     score,
		})
	}
	sort.SliceStable(ranked, func(a, b int) bool { return ranked[a].score > ranked[b].score })

	if len(ranked) > maxStoredCandidates {
		ranked = ranked[:maxStoredCandidates]
	}
	out := make([]db.MatchCandidate, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, r.candidate)
	}
	return out
}

// bestCandidate is the single winner of a ranking, for callers that want one.
func bestCandidate(title, gameID string, results []IGDBGame, known, rejected map[int64]bool) (db.MatchCandidate, float64) {
	ranked := rankCandidates(title, gameID, results, known, rejected)
	if len(ranked) == 0 {
		return db.MatchCandidate{GameID: gameID, Confidence: "none"}, 0
	}
	return ranked[0], ranked[0].Score
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
	// Expand ampersands before punctuation is stripped, so "Orcs & Humans"
	// and "Orcs and Humans" compare equal.
	s = strings.ReplaceAll(s, "&", " and ")
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
	fields := strings.Fields(s)
	// Rewrite roman-numeral tokens as arabic so "Might and Magic VI" and
	// "Might and Magic 6" compare equal. Only tokens whose canonical roman
	// spelling round-trips are converted, so words like "mix" are left alone.
	for i, f := range fields {
		if n, ok := romanToArabic(f); ok {
			fields[i] = strconv.Itoa(n)
		}
	}
	return strings.Join(fields, " ")
}

// romanToArabic parses a lowercase roman numeral, rejecting anything that is
// not in canonical form (so "mix" and "did" are not numerals).
func romanToArabic(s string) (int, bool) {
	if len(s) < 2 || len(s) > 7 {
		return 0, false
	}
	vals := map[byte]int{'i': 1, 'v': 5, 'x': 10, 'l': 50, 'c': 100, 'd': 500, 'm': 1000}
	total, prev := 0, 0
	for i := len(s) - 1; i >= 0; i-- {
		v, ok := vals[s[i]]
		if !ok {
			return 0, false
		}
		if v < prev {
			total -= v
		} else {
			total += v
			prev = v
		}
	}
	if total <= 0 {
		return 0, false
	}
	if strings.ToLower(toRoman(total)) != s {
		return 0, false
	}
	return total, true
}

// toRoman renders 1..3999 in canonical lowercase roman numerals.
func toRoman(n int) string {
	if n <= 0 || n > 3999 {
		return ""
	}
	table := []struct {
		v int
		s string
	}{
		{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"},
		{100, "C"}, {90, "XC"}, {50, "L"}, {40, "XL"},
		{10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"},
	}
	var b strings.Builder
	for _, e := range table {
		for n >= e.v {
			b.WriteString(e.s)
			n -= e.v
		}
	}
	return b.String()
}

// searchTitle prepares a store-supplied title for the IGDB search endpoint.
// Trademark symbols are absent from IGDB names and make the search return
// nothing for titles that carry them.
// searchTitle renders a stored title as a query for IGDB's search endpoint.
//
// IGDB's `search` is literal: a decorated title returns an empty set rather than
// partial matches. Measured against the live API, `Batman: Arkham Asylum GOTY
// Edition`, `Baldur's Gate: The Original Saga`, `Astebreed: Definitive Edition`,
// `BloodNet (FDD version)` and `Amerzone: The Explorer's Legacy (1999)` all
// returned **zero** results, while the undecorated title returned the game every
// time. So store decorations are stripped here, before the query, and nowhere
// else: scoring still runs against the raw title, so a wider search can only
// reach a match — it cannot re-rank one.
func searchTitle(s string) string {
	s = strings.NewReplacer("™", "", "®", "", "©", "", "\u00a0", " ").Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	s = tightenSeparators(s)

	cleaned := cleanSearchTitle(s)
	// Never let cleaning empty a title the search could still use.
	if cleaned == "" {
		return s
	}
	// Cleaning removed something from the middle of a word run, so the same space
	// can appear again: "Tomb Raider (VI): The Angel of Darkness" loses its "(VI)"
	// and is left with "Tomb Raider : The Angel of Darkness".
	return tightenSeparators(cleaned)
}

// tightenSeparators closes the gap a removal can leave before punctuation, as in
// "Batman :" after a trademark goes, or "Tomb Raider :" after a bracketed note.
func tightenSeparators(s string) string {
	return strings.NewReplacer(" :", ":", " ,", ",", " .", ".", " ;", ";").Replace(s)
}

// searchEditionSuffixes are trailing store decorations that carry no weight with
// IGDB's search. Each includes its leading space so it only matches on a word
// boundary, and the separator before it (" - ", ": ", ", ") is trimmed after the
// fact, so one entry covers "… GOTY Edition", "…: GOTY Edition" and "… - GOTY
// Edition". Longer phrases come first so "… Complete Edition" is not left as "…
// Complete".
//
// Deliberately *not* here: variant products that are genuinely different games,
// such as "Christmas Edition" or "Scenery CD" — stripping those would send the
// search after the wrong entry entirely.
var searchEditionSuffixes = []string{
	" game of the year edition", " goty edition",
	" definitive edition", " deluxe edition",
	" complete edition", " enhanced edition",
	" special edition", " collector's edition",
	" ultimate edition", " gold edition",
	" premium edition", " the original saga",
	" goty", " deluxe",
}

// searchSeparators are trimmed from the end of a title after a decoration is
// removed, so "Baldur's Gate:" becomes "Baldur's Gate".
const searchSeparators = " -:,|"

func trimSearchSeparators(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), searchSeparators+" ")
}

// stripBracketSegments removes every "(…)" or "[…]" group, wherever it sits —
// including one that never closes, whose tail is dropped with it. Brackets hold
// store notes, years, format tags and edition labels, never part of a game's own
// name, so removing them can only widen the search; and because scoring still runs
// against the title exactly as stored, a wrongly dropped tail cannot make a match
// stricter than it was. An input that is nothing but bracket groups returns empty,
// which cleanSearchTitle treats as "do not shorten this" rather than as a query.
func stripBracketSegments(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '(' || r == '[':
			depth++
		case r == ')' || r == ']':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// cleanSearchTitle removes trailing decoration until the title stops shrinking:
// a parenthesised note or year, an edition suffix, and a trailing bundle part.
func cleanSearchTitle(s string) string {
	cur := strings.TrimSpace(s)
	for {
		prev := cur

		// A bracketed note anywhere in the title, not only at the end. Stores
		// number series in the middle: "Heroes Chronicles [Chapter 1] - Warlords
		// of the Wasteland" matched nothing at all until the marker came out of
		// the query, and there are eight rows of exactly that shape.
		cur = stripBracketSegments(cur)

		// A trailing "(...)" or "[...]" note, e.g. "BloodNet (FDD version)".
		// Still needed for an unbalanced bracket, which the pass above leaves
		// alone rather than guessing where the group ended.
		if strings.HasSuffix(cur, ")") {
			if i := strings.LastIndex(cur, "("); i > 0 {
				cur = strings.TrimSpace(cur[:i])
			}
		} else if strings.HasSuffix(cur, "]") {
			if i := strings.LastIndex(cur, "["); i > 0 {
				cur = strings.TrimSpace(cur[:i])
			}
		}

		// A trailing bundle, e.g. "Besiege + The Splintered Sea DLC".
		if i := strings.Index(cur, " + "); i > 0 {
			cur = strings.TrimSpace(cur[:i])
		}

		// An edition/collection suffix.
		lower := strings.ToLower(cur)
		for _, suffix := range searchEditionSuffixes {
			if strings.HasSuffix(lower, suffix) {
				cur = trimSearchSeparators(cur[:len(cur)-len(suffix)])
				break
			}
		}

		if cur == prev {
			break
		}
		if len(cur) < 2 {
			return s
		}
	}
	return cur
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

		if err := enrichFromIGDB(store, g.ID, *match); err != nil {
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

// SearchIGDBSteamAppID searches IGDB for a game by title and returns its Steam App ID.
// Returns ("", nil) if not found or if the game has no Steam website.
func (c *IGDBClient) SearchIGDBSteamAppID(title string) (string, error) {
	results, err := c.SearchGame(title)
	if err != nil || len(results) == 0 {
		return "", err
	}

	match := bestMatch(title, results)
	if match == nil {
		return "", nil
	}

	// Fetch websites for the matched game to extract Steam App ID
	steamMap, err := c.FetchSteamAppIDs([]int64{match.ID})
	if err != nil {
		return "", err
	}

	appID, ok := steamMap[match.ID]
	if !ok {
		return "", nil
	}

	return appID, nil
}

// ── Match candidates for the review page ─────────────────────────────────────

// scoreMatch ranks how closely an IGDB name relates to a stored title, using
// progressively looser tests than bestMatch's exact equality, and returns a
// 0..1 score with a label for the UI.
//
// This is deliberately looser than the enrichment path. Nothing here is ever
// *accepted*: every candidate is put in front of the owner to rule on, so a
// loose ranking spends review attention rather than corrupting data.
func scoreMatch(stored, igdbName string) (float64, string) {
	a := normalizeTitle(stored)
	b := normalizeTitle(igdbName)
	if a == "" || b == "" {
		return 0, "none"
	}
	if a == b {
		return 1, "exact"
	}
	// Stores concatenate words IGDB spaces out ("Dragonview" for IGDB's "Dragon
	// View"), and no token test can see through that: the tokens are disjoint, so
	// the correct entry scored zero and was thrown away.
	if strings.ReplaceAll(a, " ", "") == strings.ReplaceAll(b, " ", "") {
		return 1, "exact"
	}
	// Partial-title tests need at least two words on the shorter side, or a
	// one-word title swallows everything that starts with it — "Diablo" would
	// rank "Diablo IV: Season of Divine Intervention" as a strong match.
	short, long := a, b
	if len(b) < len(a) {
		short, long = b, a
	}
	if len(strings.Fields(short)) >= 2 {
		// One title may be a leading run of whole words of the other:
		// "halcyon 6" vs "halcyon 6 starbase commander".
		if strings.HasPrefix(long, short+" ") {
			return 0.85, "prefix"
		}
		if strings.Contains(a, b) || strings.Contains(b, a) {
			return 0.7, "contains"
		}
	}
	inter, union := tokenOverlap(a, b)
	if union == 0 {
		return 0, "none"
	}
	j := float64(inter) / float64(union)
	if j >= 0.6 {
		return 0.5 + 0.2*j, "tokens"
	}
	return 0.2 * j, "weak"
}

// tokenOverlap returns the shared and combined distinct-word counts of two
// already-normalised titles.
func tokenOverlap(a, b string) (inter, union int) {
	as := make(map[string]bool)
	for _, t := range strings.Fields(a) {
		as[t] = true
	}
	bs := make(map[string]bool)
	for _, t := range strings.Fields(b) {
		bs[t] = true
	}
	for t := range as {
		if bs[t] {
			inter++
		}
	}
	return inter, len(as) + len(bs) - inter
}

// FindMatchCandidates searches IGDB for every needs_review game and stores the
// best-ranked result for the owner to approve or reject on the review page.
//
// It never writes a match to a game: it only fills the review queue, so a wrong
// candidate costs nothing but a click. Games already ruled on are skipped by
// ListGamesNeedingCandidate, and UpsertMatchCandidate refuses to overwrite a
// decided row, so the pass is safe to re-run.
func FindMatchCandidates(store *db.Store, client *IGDBClient, progressFn func(EnrichProgress)) error {
	games, err := store.ListGamesNeedingCandidate()
	if err != nil {
		return fmt.Errorf("list games: %w", err)
	}
	known, err := store.MatchedIGDBIDs()
	if err != nil {
		return fmt.Errorf("matched ids: %w", err)
	}
	// Entries the owner already rejected are never offered again, so a re-run
	// cannot resurrect the candidate that was just voted down.
	rejected, err := store.RejectedIGDBIDs()
	if err != nil {
		return fmt.Errorf("rejected ids: %w", err)
	}

	p := EnrichProgress{Total: len(games)}
	ticker := time.NewTicker(250 * time.Millisecond) // 4 req/sec, as elsewhere
	defer ticker.Stop()

	for _, g := range games {
		<-ticker.C

		candidate := db.MatchCandidate{GameID: g.ID, Confidence: "none"}
		results, err := client.SearchGame(g.Title)
		if err != nil {
			p.Errors++
		} else {
			// Keep the whole ranking so the row can offer alternatives, and make the
			// best of it the row's current candidate.
			ranked := rankCandidates(g.Title, g.ID, results, known, rejected[g.ID])
			if len(ranked) > 0 {
				candidate = ranked[0]
				p.Matched++
			}
			if err := store.ReplaceMatchCandidates(g.ID, ranked); err != nil {
				p.Errors++
			}
		}

		if err := store.UpsertMatchCandidate(candidate); err != nil {
			p.Errors++
		}
		p.Done++
		if progressFn != nil {
			progressFn(p)
		}
	}
	return nil
}
