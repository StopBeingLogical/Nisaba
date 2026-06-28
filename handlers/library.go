package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	storesync "nisaba/sync"
	"nisaba/db"
)

const perPage = 200

// gameGridData is passed to the grid partial so pagination controls can render.
type gameGridData struct {
	Games         []db.GameListRow
	CurrentPage   int
	TotalPages    int
	TotalMatching int
}

// Library renders the main game library grid view.
func (h *Handler) Library(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	q := r.URL.Query()

	p := db.ListGamesParams{
		Sort:      q.Get("sort"),
		Platform:  q.Get("platform"),
		Installed: q.Get("installed") == "1",
		Favorites: q.Get("favorites") == "1",
	}
	if v := q.Get("q"); v != "" {
		p.Search = &v
	}
	if v := q.Get("play_status"); v != "" {
		p.PlayStatus = &v
	}
	if v := q.Get("store"); v != "" {
		p.Store = &v
	}
	if v := q.Get("steam_deck"); v != "" {
		p.SteamDeck = &v
	}
	if genres := q["genre"]; len(genres) > 0 {
		p.Genres = genres
	}
	if tags := q["tag"]; len(tags) > 0 {
		p.Tags = tags
	}

	page := 1
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	p.Limit = perPage
	p.Offset = (page - 1) * perPage

	total, _ := h.store.CountMatchingGames(p)
	games, err := h.store.ListGames(p)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	totalPages := (total + perPage - 1) / perPage
	gridData := gameGridData{
		Games:         games,
		CurrentPage:   page,
		TotalPages:    totalPages,
		TotalMatching: total,
	}

	// HTMX partial swap — only return the grid + pagination
	if isHTMX(r) {
		h.renderPartial(w, "game_grid_partial.html", gridData)
		return
	}

	base, err := h.baseData("library")
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	totalAll, _ := h.store.CountGames()
	statuses, _ := h.store.StatusCounts()
	stores, _ := h.store.StoreCounts()
	genres, _ := h.store.TopGenres(15)
	tags, _ := h.store.AllTags()
	deckCounts, _ := h.store.SteamDeckCounts()

	base["TotalGames"] = totalAll
	base["StatusCounts"] = statuses
	base["StoreCounts"] = stores
	base["TopGenres"] = genres
	base["Tags"] = tags
	base["SteamDeckCounts"] = deckCounts
	base["Games"] = games
	base["Page"] = "library"
	base["CurrentPage"] = page
	base["TotalPages"] = totalPages
	base["TotalMatching"] = total

	h.render(w, "library.html", base)

	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		log.Printf("Library page loaded in %v (%d games, page %d/%d)", elapsed, total, page, totalPages)
	}
}

// GameDetail renders the full detail page for a single game.
func (h *Handler) GameDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	game, err := h.store.GetGame(id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	stores, _ := h.store.GetGameStores(id)
	sources, _ := h.store.GetGameInstallSources(id)
	contents, _ := h.store.GetGameContents(id)
	dlcs, _ := h.store.GetGameDLCs(id)

	base, err := h.baseData("library")
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Build install source view models with device labels
	type InstallSourceView struct {
		DeviceLabel      string
		VolumeLabel      string
		InstallPath      string
		InstallSizeBytes int64
	}
	var installViews []InstallSourceView
	for _, src := range sources {
		v := InstallSourceView{
			DeviceLabel: src.DeviceID,
		}
		if src.VolumeLabel.Valid {
			v.VolumeLabel = src.VolumeLabel.String
		}
		if src.InstallPath.Valid {
			v.InstallPath = src.InstallPath.String
		}
		if src.InstallSizeBytes.Valid {
			v.InstallSizeBytes = src.InstallSizeBytes.Int64
		}
		installViews = append(installViews, v)
	}

	base["Game"] = gameDetailView(game)
	base["Stores"] = stores
	base["InstallSources"] = installViews
	base["Contents"] = contents
	base["DLCs"] = dlcs

	h.render(w, "game_detail.html", base)
}

// gameDetailView builds a template-friendly struct from the db model.
type GameView struct {
	ID                string
	Title             string
	SortTitle         string
	Developer         string
	Publisher         string
	Description       string
	ShortDescription  string
	ReleaseDate       string
	EnrichmentStatus  string
	IsCompleteEdition bool
	Windows           bool
	Mac               bool
	Linux             bool
	SteamDeckVerified string
	ProtonRating      string
	IsInstalled       bool
	PlayStatus        string
	Rating            int
	ShortReview       string
	Notes             string
	IsFavorite        bool
	IsHidden          bool
	PlayTimeDisplay   string // e.g. "12h 34m" or "45m"
	Artwork           db.Artwork
	Genres            []string
	Tags              []string
}

func gameDetailView(g *db.GameDetail) GameView {
	v := GameView{
		ID:                g.ID,
		Title:             g.Title,
		SortTitle:         g.SortTitle,
		EnrichmentStatus:  g.EnrichmentStatus,
		IsCompleteEdition: g.IsCompleteEdition,
		Windows:           g.Windows,
		Mac:               g.Mac,
		Linux:             g.Linux,
		IsInstalled:       g.IsInstalled,
		IsFavorite:        g.IsFavorite,
		IsHidden:          g.IsHidden,
		Artwork:           g.Artwork(),
		Genres:            g.Genres(),
		Tags:              g.Tags(),
	}
	if g.Developer.Valid {
		v.Developer = g.Developer.String
	}
	if g.Publisher.Valid {
		v.Publisher = g.Publisher.String
	}
	if g.Description.Valid {
		v.Description = g.Description.String
	}
	if g.ShortDescription.Valid {
		v.ShortDescription = g.ShortDescription.String
	}
	if g.ReleaseDate.Valid {
		v.ReleaseDate = g.ReleaseDate.String
	}
	if g.SteamDeckVerified.Valid {
		v.SteamDeckVerified = g.SteamDeckVerified.String
	}
	if g.ProtonRating.Valid {
		v.ProtonRating = g.ProtonRating.String
	}
	if g.PlayStatus.Valid {
		v.PlayStatus = g.PlayStatus.String
	}
	if g.Rating.Valid {
		v.Rating = int(g.Rating.Int64)
	}
	if g.ShortReview.Valid {
		v.ShortReview = g.ShortReview.String
	}
	if g.Notes.Valid {
		v.Notes = g.Notes.String
	}
	if g.PlayTimeMinutes.Valid && g.PlayTimeMinutes.Int64 > 0 {
		mins := g.PlayTimeMinutes.Int64
		if mins >= 60 {
			v.PlayTimeDisplay = fmt.Sprintf("%dh %dm", mins/60, mins%60)
		} else {
			v.PlayTimeDisplay = fmt.Sprintf("%dm", mins)
		}
	}
	return v
}

// UpdateUserData handles HTMX form submissions for user-editable fields.
func (h *Handler) UpdateUserData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	p := db.UpdateUserDataParams{
		ID:         id,
		IsFavorite: r.FormValue("is_favorite") == "1",
		IsHidden:   r.FormValue("is_hidden") == "1",
	}

	if v := r.FormValue("play_status"); v != "" {
		p.PlayStatus = &v
	}
	if v := r.FormValue("rating"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			p.Rating = &n
		}
	}
	if v := r.FormValue("short_review"); v != "" {
		p.ShortReview = &v
	}
	if v := r.FormValue("notes"); v != "" {
		p.Notes = &v
	}
	if err := h.store.UpdateUserData(p); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<span class="text-green-400">Saved.</span>`))
}

// Rehydrate queues a single game for metadata rehydration.
func (h *Handler) Rehydrate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.EnqueueEnrichment("game", id); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<span class="text-green-400 text-xs">Queued.</span>`))
}

// AddGameForm renders the manual game entry form.
func (h *Handler) AddGameForm(w http.ResponseWriter, r *http.Request) {
	base, err := h.baseData("library")
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	igdbConfigured := false
	clientID, _ := h.store.GetConfig("igdb.client_id")
	if clientID != "" {
		igdbConfigured = true
	}
	base["IGDBConfigured"] = igdbConfigured
	h.render(w, "game_add.html", base)
}

// GameSearch searches IGDB for game metadata to pre-fill the add form.
// Returns a JSON array of up to 5 results.
func (h *Handler) GameSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}

	h.igdbMu.Lock()
	if h.igdb == nil {
		clientID, _ := h.store.GetConfig("igdb.client_id")
		clientSecret, _ := h.store.GetConfig("igdb.client_secret")
		if clientID != "" && clientSecret != "" {
			h.igdb = storesync.NewIGDBClient(clientID, clientSecret)
		}
	}
	client := h.igdb
	h.igdbMu.Unlock()

	if client == nil {
		http.Error(w, "IGDB not configured", http.StatusServiceUnavailable)
		return
	}

	results, err := client.SearchGame(q)
	if err != nil {
		http.Error(w, "IGDB search failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	type searchResult struct {
		IGDBID      int64  `json:"igdb_id"`
		Name        string `json:"name"`
		CoverURL    string `json:"cover_url"`
		ReleaseDate string `json:"release_date"`
		Description string `json:"description"`
	}
	out := make([]searchResult, 0, len(results))
	for _, g := range results {
		out = append(out, searchResult{
			IGDBID:      g.ID,
			Name:        g.Name,
			CoverURL:    g.CoverURL(),
			ReleaseDate: g.ReleaseDate(),
			Description: g.Summary,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// CreateGame processes the manual game entry form and redirects to the new game.
func (h *Handler) CreateGame(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}

	var developer, description, releaseDate *string
	if v := strings.TrimSpace(r.FormValue("developer")); v != "" {
		developer = &v
	}
	if v := strings.TrimSpace(r.FormValue("description")); v != "" {
		description = &v
	}
	if v := strings.TrimSpace(r.FormValue("release_date")); v != "" {
		releaseDate = &v
	}

	// Build artwork JSON safely.
	type artRef struct {
		URL    string `json:"url"`
		Source string `json:"source"`
	}
	type artworkObj struct {
		Cover  artRef `json:"cover"`
		Square artRef `json:"square"`
	}
	coverURL := strings.TrimSpace(r.FormValue("cover_url"))
	igdbID := strings.TrimSpace(r.FormValue("igdb_id"))
	artSource := "manual"
	if igdbID != "" {
		artSource = "igdb"
	}
	var artJSON string
	if coverURL != "" {
		ref := artRef{URL: coverURL, Source: artSource}
		b, _ := json.Marshal(artworkObj{Cover: ref, Square: ref})
		artJSON = string(b)
	} else {
		artJSON = "{}"
	}

	id := uuid.New().String()
	if err := h.store.InsertGame(db.InsertGameParams{
		ID:          id,
		Title:       title,
		SortTitle:   makeSortTitleH(title),
		Developer:   developer,
		Description: description,
		ReleaseDate: releaseDate,
		ArtworkJSON: artJSON,
		Windows:     r.FormValue("windows") == "1",
		Mac:         r.FormValue("mac") == "1",
		Linux:       r.FormValue("linux") == "1",
	}); err != nil {
		http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// If an IGDB result was selected, mark as matched (skip review queue).
	if igdbID != "" {
		_ = h.store.SetIGDBMatch(id, igdbID)
	}

	if store := r.FormValue("store"); store != "" {
		storeID := strings.TrimSpace(r.FormValue("store_id"))
		storeURL := strings.TrimSpace(r.FormValue("store_url"))
		_ = h.store.UpsertGameStoreLink(id, store, storeID, storeURL)
	}

	http.Redirect(w, r, "/library/"+id, http.StatusSeeOther)
}

// makeSortTitleH strips leading articles for alphabetical sorting.
func makeSortTitleH(title string) string {
	lower := strings.ToLower(title)
	for _, prefix := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(title[len(prefix):])
		}
	}
	return title
}

// ExportLibrary serves the full library as either a Markdown doc or JSON blob.
// Query param: format=markdown (default) or format=json.
func (h *Handler) ExportLibrary(w http.ResponseWriter, r *http.Request) {
	games, err := h.store.ExportGames()
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	format := r.URL.Query().Get("format")
	if format == "json" {
		h.exportJSON(w, games)
	} else {
		h.exportMarkdown(w, games)
	}
}

func (h *Handler) exportJSON(w http.ResponseWriter, games []db.ExportRow) {
	type gameJSON struct {
		Title     string   `json:"title"`
		Developer string   `json:"developer,omitempty"`
		Publisher string   `json:"publisher,omitempty"`
		Year      string   `json:"year,omitempty"`
		Platforms []string `json:"platforms"`
		Stores    []string `json:"stores"`
		Genres    []string `json:"genres,omitempty"`
		Status    string   `json:"play_status,omitempty"`
		Rating    int      `json:"rating,omitempty"`
		SteamDeck string   `json:"steam_deck,omitempty"`
		Proton    string   `json:"proton_rating,omitempty"`
		Installed bool     `json:"installed"`
		Enriched  string   `json:"enrichment_status"`
	}

	// Build summary counts.
	storeCounts := map[string]int{}
	statusCounts := map[string]int{}
	platformCounts := map[string]int{"windows": 0, "mac": 0, "linux": 0}
	installed := 0

	out := make([]gameJSON, 0, len(games))
	for _, g := range games {
		var platforms []string
		if g.Windows {
			platforms = append(platforms, "windows")
			platformCounts["windows"]++
		}
		if g.Mac {
			platforms = append(platforms, "mac")
			platformCounts["mac"]++
		}
		if g.Linux {
			platforms = append(platforms, "linux")
			platformCounts["linux"]++
		}
		for _, s := range g.OwnedStores {
			storeCounts[s]++
		}
		if g.PlayStatus != "" {
			statusCounts[g.PlayStatus]++
		}
		if g.IsInstalled {
			installed++
		}
		out = append(out, gameJSON{
			Title:     g.Title,
			Developer: g.Developer,
			Publisher: g.Publisher,
			Year:      g.ReleaseYear,
			Platforms: platforms,
			Stores:    g.OwnedStores,
			Genres:    g.Genres,
			Status:    g.PlayStatus,
			Rating:    g.Rating,
			SteamDeck: g.SteamDeckVerified,
			Proton:    g.ProtonRating,
			Installed: g.IsInstalled,
			Enriched:  g.EnrichmentStatus,
		})
	}

	payload := map[string]any{
		"generated_at": time.Now().Format("2006-01-02"),
		"total_games":  len(games),
		"summary": map[string]any{
			"by_store":    storeCounts,
			"by_status":   statusCounts,
			"by_platform": platformCounts,
			"installed":   installed,
		},
		"games": out,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="nisaba-library.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(payload)
}

func (h *Handler) exportMarkdown(w http.ResponseWriter, games []db.ExportRow) {
	var sb strings.Builder
	now := time.Now().Format("2006-01-02")

	// Summary counts.
	storeCounts := map[string]int{}
	statusCounts := map[string]int{}
	win, mac, linux, installed := 0, 0, 0, 0
	for _, g := range games {
		if g.Windows {
			win++
		}
		if g.Mac {
			mac++
		}
		if g.Linux {
			linux++
		}
		if g.IsInstalled {
			installed++
		}
		for _, s := range g.OwnedStores {
			storeCounts[s]++
		}
		if g.PlayStatus != "" {
			statusCounts[g.PlayStatus]++
		}
	}

	sb.WriteString("# NISABA Game Library Export\n\n")
	fmt.Fprintf(&sb, "Generated: %s | Total: %d games\n\n", now, len(games))

	sb.WriteString("## Platform Coverage\n\n")
	fmt.Fprintf(&sb, "- Windows: %d\n- Mac: %d\n- Linux: %d\n- Installed locally: %d\n\n", win, mac, linux, installed)

	sb.WriteString("## Store Breakdown\n\n")
	for store, count := range storeCounts {
		fmt.Fprintf(&sb, "- %s: %d\n", store, count)
	}
	sb.WriteString("\n")

	sb.WriteString("## Play Status\n\n")
	for status, count := range statusCounts {
		fmt.Fprintf(&sb, "- %s: %d\n", status, count)
	}
	sb.WriteString("\n")

	// Game table.
	sb.WriteString("## Game List\n\n")
	sb.WriteString("| Title | Year | Developer | Platforms | Stores | Genres | Status | ★ |\n")
	sb.WriteString("|-------|------|-----------|-----------|--------|--------|--------|---|\n")

	for _, g := range games {
		var plat []string
		if g.Windows {
			plat = append(plat, "W")
		}
		if g.Mac {
			plat = append(plat, "M")
		}
		if g.Linux {
			plat = append(plat, "L")
		}
		rating := ""
		if g.Rating > 0 {
			rating = fmt.Sprintf("%d", g.Rating)
		}
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			mdEscape(g.Title),
			g.ReleaseYear,
			mdEscape(g.Developer),
			strings.Join(plat, "/"),
			strings.Join(g.OwnedStores, ", "),
			strings.Join(g.Genres, ", "),
			g.PlayStatus,
			rating,
		)
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="nisaba-library.md"`)
	w.Write([]byte(sb.String()))
}

// mdEscape escapes pipe characters so they don't break Markdown tables.
func mdEscape(s string) string {
	return strings.ReplaceAll(s, "|", "&#124;")
}
