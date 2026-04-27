// Package db provides direct database access methods for nisaba.
// It mirrors the intent of the sqlc queries/ files but is implemented
// with database/sql directly so the binary compiles without running sqlc.
// Once sqlc is available, this package can be replaced with the generated output.
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Store wraps *sql.DB with typed query methods.
type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store { return &Store{db: db} }

// ── Library queries ───────────────────────────────────────────────────────────

type ListGamesParams struct {
	PlayStatus *string
	Store      *string
	SteamDeck  *string
	Platform   string  // "windows", "mac", "linux"
	Search     *string
	Genres     []string // AND filter
	Tags       []string // AND filter
	Installed  bool
	Favorites  bool
	Sort       string // "", "added", "playtime", "rating"
}

func (s *Store) ListGames(p ListGamesParams) ([]GameListRow, error) {
	// Build JOIN and WHERE args separately — JOINs appear before WHERE in
	// the SQL string so their ? placeholders must come first in the args slice.
	var joinArgs, whereArgs []any
	where := []string{"g.is_hidden = 0", "g.parent_id IS NULL"}

	if p.PlayStatus != nil && *p.PlayStatus != "" {
		where = append(where, "g.play_status = ?")
		whereArgs = append(whereArgs, *p.PlayStatus)
	}
	if p.SteamDeck != nil && *p.SteamDeck != "" {
		where = append(where, "g.steam_deck_verified = ?")
		whereArgs = append(whereArgs, *p.SteamDeck)
	}
	if p.Search != nil && *p.Search != "" {
		where = append(where, "g.title LIKE ?")
		whereArgs = append(whereArgs, "%"+*p.Search+"%")
	}
	switch p.Platform {
	case "windows":
		where = append(where, "g.windows = 1")
	case "mac":
		where = append(where, "g.mac = 1")
	case "linux":
		where = append(where, "g.linux = 1")
	}
	if p.Installed {
		where = append(where, "g.is_installed = 1")
	}
	if p.Favorites {
		where = append(where, "g.is_favorite = 1")
	}

	joins := ""
	if p.Store != nil && *p.Store != "" {
		joins = "JOIN game_stores _sf ON _sf.game_id = g.id AND _sf.store = ? AND _sf.owned = 1"
		joinArgs = append(joinArgs, *p.Store)
	}
	for i, genre := range p.Genres {
		alias := fmt.Sprintf("_g%d", i)
		joins += fmt.Sprintf(" JOIN game_genres %s ON %s.game_id = g.id AND %s.genre = ?", alias, alias, alias)
		joinArgs = append(joinArgs, genre)
	}
	for i, tag := range p.Tags {
		alias := fmt.Sprintf("_t%d", i)
		joins += fmt.Sprintf(" JOIN game_tags %s ON %s.game_id = g.id AND %s.tag = ?", alias, alias, alias)
		joinArgs = append(joinArgs, tag)
	}

	orderBy := "g.sort_title ASC"
	switch p.Sort {
	case "added":
		orderBy = "g.date_added DESC"
	case "playtime":
		orderBy = "CASE WHEN g.play_time_minutes IS NULL THEN 1 ELSE 0 END, g.play_time_minutes DESC"
	case "rating":
		orderBy = "CASE WHEN g.rating IS NULL THEN 1 ELSE 0 END, g.rating DESC, g.sort_title ASC"
	}

	args := append(joinArgs, whereArgs...)

	q := fmt.Sprintf(`
SELECT
    g.id, g.title, g.sort_title, g.enrichment_status,
    g.is_complete_edition, g.parent_id, g.artwork,
    g.steam_deck_verified, g.proton_rating, g.is_installed,
    g.play_status, g.rating, g.is_favorite, g.is_hidden, g.play_time_minutes,
    (SELECT GROUP_CONCAT(genre) FROM game_genres WHERE game_id = g.id) AS genres,
    (SELECT GROUP_CONCAT(tag)   FROM game_tags   WHERE game_id = g.id) AS tags,
    (SELECT GROUP_CONCAT(store) FROM game_stores WHERE game_id = g.id AND owned = 1) AS owned_stores,
    CASE WHEN g.igdb_id IS NOT NULL AND EXISTS (
        SELECT 1 FROM games g2
        JOIN game_stores gs2 ON gs2.game_id = g2.id AND gs2.owned = 1
        WHERE g2.igdb_id = g.igdb_id AND g2.id != g.id
    ) THEN 1 ELSE 0 END AS multi_store_owned
FROM games g %s
WHERE %s
ORDER BY %s`, joins, strings.Join(where, " AND "), orderBy)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []GameListRow
	for rows.Next() {
		var r GameListRow
		err := rows.Scan(
			&r.ID, &r.Title, &r.SortTitle, &r.EnrichmentStatus,
			&r.IsCompleteEdition, &r.ParentID, &r.ArtworkRaw,
			&r.SteamDeckVerifiedRaw, &r.ProtonRatingRaw, &r.IsInstalled,
			&r.PlayStatusRaw, &r.Rating, &r.IsFavorite, &r.IsHidden, &r.PlayTimeMinutes,
			&r.GenresRaw, &r.TagsRaw, &r.OwnedStoresRaw, &r.MultiStoreOwned,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) GetGame(id string) (*GameDetail, error) {
	q := `
SELECT
    g.id, g.igdb_id, g.rawg_id, g.title, g.sort_title, g.developer, g.publisher,
    g.description, g.short_description, g.release_date,
    g.enrichment_status, g.last_enriched, g.is_complete_edition, g.parent_id,
    g.artwork, g.windows, g.mac, g.linux, g.steam_deck_verified, g.proton_rating,
    g.is_installed, g.play_status, g.rating, g.short_review, g.notes,
    g.is_favorite, g.is_hidden, g.play_time_minutes, g.last_played, g.date_added,
    (SELECT GROUP_CONCAT(genre) FROM game_genres WHERE game_id = g.id) AS genres,
    (SELECT GROUP_CONCAT(tag)   FROM game_tags   WHERE game_id = g.id) AS tags
FROM games g WHERE g.id = ?`

	var g GameDetail
	err := s.db.QueryRow(q, id).Scan(
		&g.ID, &g.IGDBId, &g.RAWGId, &g.Title, &g.SortTitle, &g.Developer, &g.Publisher,
		&g.Description, &g.ShortDescription, &g.ReleaseDate,
		&g.EnrichmentStatus, &g.LastEnriched, &g.IsCompleteEdition, &g.ParentID,
		&g.ArtworkRaw, &g.Windows, &g.Mac, &g.Linux, &g.SteamDeckVerified, &g.ProtonRating,
		&g.IsInstalled, &g.PlayStatus, &g.Rating, &g.ShortReview, &g.Notes,
		&g.IsFavorite, &g.IsHidden, &g.PlayTimeMinutes, &g.LastPlayed, &g.DateAdded,
		&g.GenresRaw, &g.TagsRaw,
	)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) GetGameStores(gameID string) ([]GameStore, error) {
	rows, err := s.db.Query(
		`SELECT game_id, store, store_id, store_url, owned, owned_since FROM game_stores WHERE game_id = ? AND owned = 1`,
		gameID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []GameStore
	for rows.Next() {
		var r GameStore
		if err := rows.Scan(&r.GameID, &r.Store, &r.StoreID, &r.StoreURL, &r.Owned, &r.OwnedSince); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) GetGameInstallSources(gameID string) ([]InstallSource, error) {
	rows, err := s.db.Query(`
SELECT gi.game_id, gi.device_id, gi.volume_id, gi.install_path, gi.install_size_bytes,
       gi.runner, gi.last_seen, sv.label, sv.path
FROM game_install_sources gi
LEFT JOIN storage_volumes sv ON sv.id = gi.volume_id
WHERE gi.game_id = ?`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []InstallSource
	for rows.Next() {
		var r InstallSource
		if err := rows.Scan(
			&r.GameID, &r.DeviceID, &r.VolumeID, &r.InstallPath, &r.InstallSizeBytes,
			&r.Runner, &r.LastSeen, &r.VolumeLabel, &r.VolumePath,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) GetGameContents(gameID string) ([]GameContent, error) {
	rows, err := s.db.Query(
		`SELECT game_id, content_type, title, store_id, is_installed, installation_type, sort_order FROM game_contents WHERE game_id = ? ORDER BY sort_order, title`,
		gameID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []GameContent
	for rows.Next() {
		var r GameContent
		if err := rows.Scan(&r.GameID, &r.ContentType, &r.Title, &r.StoreID, &r.IsInstalled, &r.InstallationType, &r.SortOrder); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) GetGameDLCs(parentID string) ([]GameListRow, error) {
	rows, err := s.db.Query(`
SELECT g.id, g.title, g.sort_title, g.enrichment_status,
       g.is_complete_edition, g.parent_id, g.artwork,
       g.steam_deck_verified, g.proton_rating, g.is_installed,
       g.play_status, g.rating, g.is_favorite, g.is_hidden, g.play_time_minutes,
       NULL, NULL, NULL, 0
FROM games g WHERE g.parent_id = ? ORDER BY g.sort_title`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []GameListRow
	for rows.Next() {
		var r GameListRow
		if err := rows.Scan(
			&r.ID, &r.Title, &r.SortTitle, &r.EnrichmentStatus,
			&r.IsCompleteEdition, &r.ParentID, &r.ArtworkRaw,
			&r.SteamDeckVerifiedRaw, &r.ProtonRatingRaw, &r.IsInstalled,
			&r.PlayStatusRaw, &r.Rating, &r.IsFavorite, &r.IsHidden, &r.PlayTimeMinutes,
			&r.GenresRaw, &r.TagsRaw, &r.OwnedStoresRaw, &r.MultiStoreOwned,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) ExportGames() ([]ExportRow, error) {
	rows, err := s.db.Query(`
SELECT
    g.title,
    COALESCE(g.developer, '')           AS developer,
    COALESCE(g.publisher, '')           AS publisher,
    COALESCE(strftime('%Y', g.release_date), '') AS release_year,
    g.windows, g.mac, g.linux,
    COALESCE(g.steam_deck_verified, '') AS steam_deck_verified,
    COALESCE(g.proton_rating, '')       AS proton_rating,
    COALESCE(g.play_status, '')         AS play_status,
    COALESCE(g.rating, 0)               AS rating,
    g.is_installed,
    g.enrichment_status,
    COALESCE((SELECT GROUP_CONCAT(genre ORDER BY genre)
              FROM game_genres WHERE game_id = g.id), '') AS genres,
    COALESCE((SELECT GROUP_CONCAT(store ORDER BY store)
              FROM game_stores WHERE game_id = g.id AND owned = 1), '') AS owned_stores
FROM games g
WHERE g.is_hidden = 0 AND g.parent_id IS NULL
ORDER BY g.sort_title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ExportRow
	for rows.Next() {
		var r ExportRow
		var genresRaw, storesRaw string
		if err := rows.Scan(
			&r.Title, &r.Developer, &r.Publisher, &r.ReleaseYear,
			&r.Windows, &r.Mac, &r.Linux,
			&r.SteamDeckVerified, &r.ProtonRating,
			&r.PlayStatus, &r.Rating, &r.IsInstalled, &r.EnrichmentStatus,
			&genresRaw, &storesRaw,
		); err != nil {
			return nil, err
		}
		if genresRaw != "" {
			r.Genres = strings.Split(genresRaw, ",")
		}
		if storesRaw != "" {
			r.OwnedStores = strings.Split(storesRaw, ",")
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) CountGames() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM games WHERE is_hidden = 0 AND parent_id IS NULL`).Scan(&n)
	return n, err
}

func (s *Store) StatusCounts() ([]StatusCount, error) {
	rows, err := s.db.Query(`
SELECT play_status, COUNT(*) FROM games
WHERE is_hidden = 0 AND parent_id IS NULL
GROUP BY play_status ORDER BY play_status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	labels := map[string]string{
		"":          "Not Set",
		"unplayed":  "Unplayed",
		"playing":   "Playing",
		"completed": "Completed",
		"dropped":   "Dropped",
		"on_hold":   "On Hold",
	}

	var result []StatusCount
	for rows.Next() {
		var value sql.NullString
		var count int
		if err := rows.Scan(&value, &count); err != nil {
			return nil, err
		}
		v := value.String
		label := labels[v]
		if label == "" {
			label = v
		}
		result = append(result, StatusCount{Value: v, Label: label, Count: count})
	}
	return result, rows.Err()
}

// StoreCount holds a store name and its owned game count.
type StoreCount struct {
	Store string
	Count int
}

func (s *Store) StoreCounts() ([]StoreCount, error) {
	rows, err := s.db.Query(`
SELECT gs.store, COUNT(DISTINCT gs.game_id) AS cnt
FROM game_stores gs
JOIN games g ON g.id = gs.game_id
WHERE gs.owned = 1 AND g.is_hidden = 0 AND g.parent_id IS NULL
GROUP BY gs.store ORDER BY cnt DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []StoreCount
	for rows.Next() {
		var r StoreCount
		if err := rows.Scan(&r.Store, &r.Count); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) TopGenres(limit int) ([]GenreCount, error) {
	rows, err := s.db.Query(`
SELECT gg.genre, COUNT(*) AS cnt
FROM game_genres gg
JOIN games g ON g.id = gg.game_id
WHERE g.is_hidden = 0 AND g.parent_id IS NULL
GROUP BY gg.genre ORDER BY cnt DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []GenreCount
	for rows.Next() {
		var r GenreCount
		if err := rows.Scan(&r.Genre, &r.Count); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// SteamDeckCounts returns per-status counts for Steam Deck verification.
func (s *Store) SteamDeckCounts() ([]StatusCount, error) {
	rows, err := s.db.Query(`
SELECT steam_deck_verified, COUNT(*) FROM games
WHERE is_hidden = 0 AND parent_id IS NULL AND steam_deck_verified IS NOT NULL
GROUP BY steam_deck_verified ORDER BY CASE steam_deck_verified
    WHEN 'verified'    THEN 1
    WHEN 'playable'    THEN 2
    WHEN 'unsupported' THEN 3
    ELSE 4 END`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	labels := map[string]string{
		"verified":    "Verified",
		"playable":    "Playable",
		"unsupported": "Unsupported",
		"unknown":     "Unknown",
	}
	var result []StatusCount
	for rows.Next() {
		var value string
		var count int
		if err := rows.Scan(&value, &count); err != nil {
			return nil, err
		}
		result = append(result, StatusCount{Value: value, Label: labels[value], Count: count})
	}
	return result, rows.Err()
}

// ListGamesNeedingDeckStatus returns Steam games without a deck status.
func (s *Store) ListGamesNeedingDeckStatus() ([]struct{ ID, SteamAppID string }, error) {
	rows, err := s.db.Query(`
SELECT g.id, gs.store_id
FROM games g
JOIN game_stores gs ON gs.game_id = g.id AND gs.store = 'steam'
WHERE (g.steam_deck_verified IS NULL OR g.steam_deck_verified = 'unknown')
AND gs.store_id != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []struct{ ID, SteamAppID string }
	for rows.Next() {
		var r struct{ ID, SteamAppID string }
		if err := rows.Scan(&r.ID, &r.SteamAppID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// SetSteamDeckVerified updates the steam_deck_verified status for a game.
func (s *Store) SetSteamDeckVerified(gameID, status string) error {
	_, err := s.db.Exec(`UPDATE games SET steam_deck_verified = ? WHERE id = ?`, status, gameID)
	return err
}

// ListWishlistNeedingDeckStatus returns wishlist entries with a Steam store link
// that don't yet have a deck status (or are "unknown").
func (s *Store) ListWishlistNeedingDeckStatus() ([]struct{ ID, SteamAppID string }, error) {
	rows, err := s.db.Query(`
SELECT w.id, ws.store_id
FROM wishlist_entries w
JOIN wishlist_stores ws ON ws.wishlist_id = w.id AND ws.store = 'steam'
WHERE (w.steam_deck_verified IS NULL OR w.steam_deck_verified = 'unknown')
AND ws.store_id != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []struct{ ID, SteamAppID string }
	for rows.Next() {
		var r struct{ ID, SteamAppID string }
		if err := rows.Scan(&r.ID, &r.SteamAppID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// SetWishlistDeckVerified updates the steam_deck_verified status for a wishlist entry.
func (s *Store) SetWishlistDeckVerified(wishlistID, status string) error {
	_, err := s.db.Exec(`UPDATE wishlist_entries SET steam_deck_verified = ? WHERE id = ?`, status, wishlistID)
	return err
}

// SteamCrossRefRow is a non-Steam game candidate for Steam cross-referencing.
type SteamCrossRefRow struct {
	ID     string
	IGDBId int64
	Title  string
}

// ListGamesNeedingSteamCrossRef returns non-Steam games with an IGDB ID that
// don't yet have a Steam cross-reference row in game_stores.
func (s *Store) ListGamesNeedingSteamCrossRef() ([]SteamCrossRefRow, error) {
	rows, err := s.db.Query(`
SELECT g.id, CAST(g.igdb_id AS INTEGER), g.title
FROM games g
WHERE g.igdb_id IS NOT NULL
AND NOT EXISTS (SELECT 1 FROM game_stores gs WHERE gs.game_id = g.id AND gs.store = 'steam')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SteamCrossRefRow
	for rows.Next() {
		var r SteamCrossRefRow
		if err := rows.Scan(&r.ID, &r.IGDBId, &r.Title); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// UpsertSteamCrossRef inserts a non-owned Steam reference row for a game,
// so that it will be picked up by the deck status sync.
// Does nothing if a row already exists for this game+steam combination.
func (s *Store) UpsertSteamCrossRef(gameID, steamAppID string) error {
	_, err := s.db.Exec(`
INSERT INTO game_stores (game_id, store, store_id, store_url, owned)
VALUES (?, 'steam', ?, 'https://store.steampowered.com/app/' || ?, 0)
ON CONFLICT (game_id, store) DO NOTHING`, gameID, steamAppID, steamAppID)
	return err
}

func (s *Store) AllTags() ([]string, error) {
	rows, err := s.db.Query(`
SELECT DISTINCT gt.tag FROM game_tags gt
JOIN games g ON g.id = gt.game_id
WHERE g.is_hidden = 0 ORDER BY gt.tag ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

type UpdateUserDataParams struct {
	ID              string
	PlayStatus      *string
	Rating          *int
	ShortReview     *string
	Notes           *string
	IsFavorite      bool
	IsHidden        bool
	PlayTimeMinutes *int
	LastPlayed      *time.Time
}

func (s *Store) UpdateUserData(p UpdateUserDataParams) error {
	_, err := s.db.Exec(`
UPDATE games SET
    play_status       = ?,
    rating            = ?,
    short_review      = ?,
    notes             = ?,
    is_favorite       = ?,
    is_hidden         = ?,
    play_time_minutes = ?,
    last_played       = ?
WHERE id = ?`,
		p.PlayStatus, p.Rating, p.ShortReview, p.Notes,
		p.IsFavorite, p.IsHidden, p.PlayTimeMinutes, p.LastPlayed,
		p.ID,
	)
	return err
}

func (s *Store) CountNeedsReview() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM games WHERE enrichment_status = 'needs_review' AND parent_id IS NULL`).Scan(&n)
	return n, err
}

// ── Wishlist queries ──────────────────────────────────────────────────────────

type ListWishlistParams struct {
	Search    *string
	Sort      string   // priority, price_asc, price_desc, alpha, added, store_group
	Priority  *int     // filter: priority >= value (1=low, 2=med, 3=high)
	SteamDeck *string  // filter: steam_deck_verified
	Store     string   // filter: must have a link in wishlist_stores for this store
	BestStore  string   // filter: best_current_store = value
	MaxPrice   *float64 // filter: best_current_price <= value
	Owned      bool     // filter: only entries already linked to library
	Duplicates bool     // filter: only entries that share an IGDB ID or title with another entry
}

func (s *Store) ListWishlist(p ListWishlistParams) ([]WishlistRow, error) {
	where := []string{"1=1"}
	args := []any{}

	if p.Search != nil && *p.Search != "" {
		where = append(where, "w.title LIKE ?")
		args = append(args, "%"+*p.Search+"%")
	}
	if p.Priority != nil {
		where = append(where, "w.priority >= ?")
		args = append(args, *p.Priority)
	}
	if p.SteamDeck != nil && *p.SteamDeck != "" {
		where = append(where, "w.steam_deck_verified = ?")
		args = append(args, *p.SteamDeck)
	}
	if p.Store != "" {
		where = append(where, "EXISTS (SELECT 1 FROM wishlist_stores WHERE wishlist_id = w.id AND store = ?)")
		args = append(args, p.Store)
	}
	if p.BestStore != "" {
		where = append(where, "w.best_current_store = ?")
		args = append(args, p.BestStore)
	}
	if p.MaxPrice != nil {
		where = append(where, "w.best_current_price IS NOT NULL AND w.best_current_price <= ?")
		args = append(args, *p.MaxPrice)
	}
	if p.Owned {
		where = append(where, "w.library_id IS NOT NULL")
	}
	if p.Duplicates {
		where = append(where, `(
			(w.igdb_id IS NOT NULL AND EXISTS (SELECT 1 FROM wishlist_entries d WHERE d.igdb_id = w.igdb_id AND d.id != w.id))
			OR EXISTS (SELECT 1 FROM wishlist_entries d WHERE LOWER(d.title) = LOWER(w.title) AND d.id != w.id)
		)`)
	}

	orderBy := "w.priority DESC, w.sort_title ASC"
	switch p.Sort {
	case "price_asc":
		// SQLite: NULLs sort last in ASC when using CASE
		orderBy = "CASE WHEN w.best_current_price IS NULL THEN 1 ELSE 0 END, w.best_current_price ASC"
	case "price_desc":
		orderBy = "CASE WHEN w.best_current_price IS NULL THEN 1 ELSE 0 END, w.best_current_price DESC"
	case "alpha":
		orderBy = "w.sort_title ASC"
	case "added":
		orderBy = "w.date_added DESC"
	case "store_group":
		orderBy = "COALESCE(w.best_current_store, '') ASC, CASE WHEN w.best_current_price IS NULL THEN 1 ELSE 0 END, w.best_current_price ASC"
	}

	q := fmt.Sprintf(`
SELECT w.id, w.library_id, w.title, w.sort_title, w.enrichment_status,
       w.artwork, w.steam_deck_verified, w.currency,
       w.best_current_price, w.best_current_store,
       w.historical_low_price, w.historical_low_store,
       w.target_price, w.priority, w.date_added,
       (SELECT GROUP_CONCAT(tag)   FROM wishlist_tags   WHERE wishlist_id = w.id) AS tags,
       (SELECT GROUP_CONCAT(store) FROM wishlist_stores WHERE wishlist_id = w.id) AS stores,
       w.flag_remove,
       CASE WHEN
           (w.igdb_id IS NOT NULL AND EXISTS (SELECT 1 FROM wishlist_entries d WHERE d.igdb_id = w.igdb_id AND d.id != w.id))
           OR EXISTS (SELECT 1 FROM wishlist_entries d WHERE LOWER(d.title) = LOWER(w.title) AND d.id != w.id)
       THEN 1 ELSE 0 END AS is_duplicate
FROM wishlist_entries w
WHERE %s ORDER BY %s`, strings.Join(where, " AND "), orderBy)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []WishlistRow
	for rows.Next() {
		var r WishlistRow
		if err := rows.Scan(
			&r.ID, &r.LibraryID, &r.Title, &r.SortTitle, &r.EnrichmentStatus,
			&r.ArtworkRaw, &r.SteamDeckVerifiedRaw, &r.Currency,
			&r.BestCurrentPrice, &r.BestCurrentStore,
			&r.HistoricalLowPrice, &r.HistoricalLowStore,
			&r.TargetPrice, &r.Priority, &r.DateAdded,
			&r.TagsRaw, &r.StoresRaw,
			&r.FlagRemove, &r.IsDuplicate,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) CountWishlist() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM wishlist_entries`).Scan(&n)
	return n, err
}

// WishlistPriorityCounts returns counts per priority level (>0 only).
func (s *Store) WishlistPriorityCounts() ([]StatusCount, error) {
	rows, err := s.db.Query(`
SELECT priority, COUNT(*) FROM wishlist_entries
WHERE priority > 0 GROUP BY priority ORDER BY priority DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	labels := map[int]string{3: "High", 2: "Medium", 1: "Low"}
	var out []StatusCount
	for rows.Next() {
		var pri, cnt int
		if err := rows.Scan(&pri, &cnt); err != nil {
			return nil, err
		}
		out = append(out, StatusCount{Value: strconv.Itoa(pri), Label: labels[pri], Count: cnt})
	}
	return out, rows.Err()
}

// WishlistDeckCounts returns verified/playable counts for the sidebar.
func (s *Store) WishlistDeckCounts() ([]StatusCount, error) {
	rows, err := s.db.Query(`
SELECT steam_deck_verified, COUNT(*) FROM wishlist_entries
WHERE steam_deck_verified IN ('verified', 'playable')
GROUP BY steam_deck_verified
ORDER BY CASE steam_deck_verified WHEN 'verified' THEN 0 WHEN 'playable' THEN 1 END`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	labels := map[string]string{"verified": "Deck Verified", "playable": "Deck Playable"}
	var out []StatusCount
	for rows.Next() {
		var val string
		var cnt int
		if err := rows.Scan(&val, &cnt); err != nil {
			return nil, err
		}
		out = append(out, StatusCount{Value: val, Label: labels[val], Count: cnt})
	}
	return out, rows.Err()
}

// WishlistDuplicateCount returns the number of wishlist entries that share an
// IGDB ID or title with at least one other entry.
func (s *Store) WishlistDuplicateCount() (int, error) {
	var n int
	err := s.db.QueryRow(`
SELECT COUNT(*) FROM wishlist_entries w
WHERE (w.igdb_id IS NOT NULL AND EXISTS (SELECT 1 FROM wishlist_entries d WHERE d.igdb_id = w.igdb_id AND d.id != w.id))
   OR EXISTS (SELECT 1 FROM wishlist_entries d WHERE LOWER(d.title) = LOWER(w.title) AND d.id != w.id)`).Scan(&n)
	return n, err
}

// WishlistStoreCounts returns per-store entry counts for the sidebar filter.
func (s *Store) WishlistStoreCounts() ([]StatusCount, error) {
	rows, err := s.db.Query(`
SELECT ws.store, COUNT(DISTINCT ws.wishlist_id)
FROM wishlist_stores ws
GROUP BY ws.store ORDER BY ws.store ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []StatusCount
	for rows.Next() {
		var sc StatusCount
		if err := rows.Scan(&sc.Value, &sc.Count); err != nil {
			return nil, err
		}
		sc.Label = sc.Value
		result = append(result, sc)
	}
	return result, rows.Err()
}

// WishlistBestStoreCounts returns per-store entry counts based on best_current_store,
// sorted by count descending so the store with the most deals appears first.
func (s *Store) WishlistBestStoreCounts() ([]StatusCount, error) {
	rows, err := s.db.Query(`
SELECT best_current_store, COUNT(*) AS cnt
FROM wishlist_entries
WHERE best_current_store IS NOT NULL AND best_current_store != ''
GROUP BY best_current_store
ORDER BY cnt DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []StatusCount
	for rows.Next() {
		var sc StatusCount
		if err := rows.Scan(&sc.Value, &sc.Count); err != nil {
			return nil, err
		}
		sc.Label = sc.Value
		result = append(result, sc)
	}
	return result, rows.Err()
}

// ClearStalePrices nulls out best_current_price for any store source that is
// no longer active. Call once on startup after removing a price source.
func (s *Store) ClearStalePrices(storePrefix string) error {
	_, err := s.db.Exec(`
UPDATE wishlist_entries
SET best_current_price = NULL, best_current_store = NULL
WHERE best_current_store LIKE ?`, storePrefix+"%")
	return err
}

// ── Price thresholds ──────────────────────────────────────────────────────────

// PriceThreshold is a named price ceiling used to filter the wishlist.
type PriceThreshold struct {
	ID       int64
	Label    string
	MaxPrice float64
}

func (s *Store) ListPriceThresholds() ([]PriceThreshold, error) {
	rows, err := s.db.Query(`SELECT id, label, max_price FROM price_thresholds ORDER BY max_price ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PriceThreshold
	for rows.Next() {
		var t PriceThreshold
		if err := rows.Scan(&t.ID, &t.Label, &t.MaxPrice); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func (s *Store) AddPriceThreshold(label string, maxPrice float64) error {
	_, err := s.db.Exec(`INSERT INTO price_thresholds (label, max_price) VALUES (?, ?)`, label, maxPrice)
	return err
}

func (s *Store) DeletePriceThreshold(id int64) error {
	_, err := s.db.Exec(`DELETE FROM price_thresholds WHERE id = ?`, id)
	return err
}

// ── Stale wishlist cleanup ────────────────────────────────────────────────────

// DeleteStaleWishlistEntries removes entries whose ID starts with prefix but
// whose ID is not in the keepIDs set. Used after store wishlist syncs to prune
// games the user has since removed from their wishlist.
func (s *Store) DeleteStaleWishlistEntries(prefix string, keepIDs []string) (int, error) {
	keep := make(map[string]bool, len(keepIDs))
	for _, id := range keepIDs {
		keep[id] = true
	}

	rows, err := s.db.Query(`SELECT id FROM wishlist_entries WHERE id LIKE ?`, prefix+"%")
	if err != nil {
		return 0, err
	}
	var toDelete []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		if !keep[id] {
			toDelete = append(toDelete, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, id := range toDelete {
		if _, err := s.db.Exec(`DELETE FROM wishlist_entries WHERE id = ?`, id); err != nil {
			return len(toDelete), err
		}
	}
	return len(toDelete), nil
}

// InsertManualWishlistEntry adds a game manually (from IGDB search). Uses ON CONFLICT DO NOTHING
// so clicking "add" twice is safe.
func (s *Store) InsertManualWishlistEntry(id string, igdbID int64, title, sortTitle, coverURL string) error {
	artwork := "{}"
	if coverURL != "" {
		b, _ := json.Marshal(map[string]any{
			"square": map[string]string{"url": coverURL, "source": "igdb"},
		})
		artwork = string(b)
	}
	_, err := s.db.Exec(`
INSERT INTO wishlist_entries (id, igdb_id, title, sort_title, artwork, enrichment_status)
VALUES (?, ?, ?, ?, ?, 'matched')
ON CONFLICT(id) DO NOTHING`,
		id, igdbID, title, sortTitle, artwork)
	return err
}

// ListWishlistPriceHistory returns price history for a single wishlist entry, oldest first.
func (s *Store) ListWishlistPriceHistory(id string) ([]PriceHistoryRow, error) {
	rows, err := s.db.Query(`
SELECT price, store, recorded_at FROM wishlist_price_history
WHERE wishlist_id = ? ORDER BY recorded_at ASC LIMIT 90`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PriceHistoryRow
	for rows.Next() {
		var r PriceHistoryRow
		var recAt string
		if err := rows.Scan(&r.Price, &r.Store, &recAt); err != nil {
			return nil, err
		}
		r.RecordedAt, _ = time.Parse("2006-01-02 15:04:05", recAt)
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetWishlistDetail fetches a single wishlist entry with all fields for the detail view.
func (s *Store) GetWishlistDetail(id string) (*WishlistDetailRow, error) {
	var r WishlistDetailRow
	err := s.db.QueryRow(`
SELECT w.id, w.library_id, w.igdb_id, w.title, w.sort_title, w.enrichment_status,
       w.artwork, w.steam_deck_verified, w.currency,
       w.best_current_price, w.best_current_store, w.best_price_url,
       w.historical_low_price, w.historical_low_store,
       w.last_price_sync, w.target_price, w.preferred_store,
       w.priority, w.notes, w.date_added,
       (SELECT GROUP_CONCAT(tag) FROM wishlist_tags WHERE wishlist_id = w.id) AS tags
FROM wishlist_entries w
WHERE w.id = ?`, id).Scan(
		&r.ID, &r.LibraryID, &r.IGDBId, &r.Title, &r.SortTitle, &r.EnrichmentStatus,
		&r.ArtworkRaw, &r.SteamDeckVerifiedRaw, &r.Currency,
		&r.BestCurrentPrice, &r.BestCurrentStore, &r.BestPriceURL,
		&r.HistoricalLowPrice, &r.HistoricalLowStore,
		&r.LastPriceSync, &r.TargetPrice, &r.PreferredStore,
		&r.Priority, &r.Notes, &r.DateAdded,
		&r.TagsRaw,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &r, err
}

// ListWishlistStores returns all store availability entries for a wishlist item.
func (s *Store) ListWishlistStores(wishlistID string) ([]WishlistStoreLink, error) {
	rows, err := s.db.Query(
		`SELECT store, store_id, store_url FROM wishlist_stores WHERE wishlist_id = ?`, wishlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []WishlistStoreLink
	for rows.Next() {
		var r WishlistStoreLink
		if err := rows.Scan(&r.Store, &r.StoreID, &r.StoreURL); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// UpdateWishlistUserData saves user-editable fields on a wishlist entry.
func (s *Store) UpdateWishlistUserData(id string, priority int, notes string, targetPrice *float64, preferredStore string) error {
	var ns *string
	if notes != "" {
		ns = &notes
	}
	var ps *string
	if preferredStore != "" {
		ps = &preferredStore
	}
	_, err := s.db.Exec(`
UPDATE wishlist_entries SET
    priority        = ?,
    notes           = ?,
    target_price    = ?,
    preferred_store = ?
WHERE id = ?`,
		priority, ns, targetPrice, ps, id,
	)
	return err
}

// LinkWishlistToLibrary sets library_id on wishlist entries whose igdb_id matches
// a game in the library. Safe to call repeatedly — idempotent.
func (s *Store) LinkWishlistToLibrary() (int, error) {
	res, err := s.db.Exec(`
UPDATE wishlist_entries
SET library_id = (
    SELECT id FROM games WHERE igdb_id = wishlist_entries.igdb_id LIMIT 1
)
WHERE igdb_id IS NOT NULL
  AND EXISTS (SELECT 1 FROM games WHERE igdb_id = wishlist_entries.igdb_id)`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// GameTitlesByStoreID returns a map of store_id → game title for a given store.
// Used to resolve names for wishlist items already in the library.
func (s *Store) GameTitlesByStoreID(storeName string, storeIDs []string) (map[string]string, error) {
	if len(storeIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(storeIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := []any{storeName}
	for _, id := range storeIDs {
		args = append(args, id)
	}
	rows, err := s.db.Query(fmt.Sprintf(`
SELECT gs.store_id, g.title
FROM game_stores gs JOIN games g ON g.id = gs.game_id
WHERE gs.store = ? AND gs.store_id IN (%s)`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]string, len(storeIDs))
	for rows.Next() {
		var sid, title string
		if err := rows.Scan(&sid, &title); err != nil {
			return nil, err
		}
		result[sid] = title
	}
	return result, rows.Err()
}

// WishlistPricingRow is used by the GG.deals pricing sync.
type WishlistPricingRow struct {
	ID         string
	Title      string
	ITADId     string // retained for legacy compatibility
	SteamAppID string // from wishlist_stores, may be empty
}

// ListWishlistForPricing returns all wishlist entries with their Steam app IDs.
func (s *Store) ListWishlistForPricing() ([]WishlistPricingRow, error) {
	rows, err := s.db.Query(`
SELECT w.id, w.title, COALESCE(w.itad_id, ''), COALESCE(ws.store_id, '')
FROM wishlist_entries w
LEFT JOIN wishlist_stores ws ON ws.wishlist_id = w.id AND ws.store = 'steam'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []WishlistPricingRow
	for rows.Next() {
		var r WishlistPricingRow
		if err := rows.Scan(&r.ID, &r.Title, &r.ITADId, &r.SteamAppID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// WishlistResellerRow is used by the reseller scraper.
type WishlistResellerRow struct {
	ID               string
	Title            string
	BestCurrentPrice *float64
}

// ListWishlistForResellerPricing returns wishlist entries that have no Steam
// App ID — these are not covered by GG.deals and need reseller scraping.
func (s *Store) ListWishlistForResellerPricing() ([]WishlistResellerRow, error) {
	rows, err := s.db.Query(`
SELECT w.id, w.title, w.best_current_price
FROM wishlist_entries w
WHERE NOT EXISTS (
    SELECT 1 FROM wishlist_stores ws
    WHERE ws.wishlist_id = w.id AND ws.store = 'steam' AND ws.store_id != ''
)
ORDER BY w.sort_title ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []WishlistResellerRow
	for rows.Next() {
		var r WishlistResellerRow
		if err := rows.Scan(&r.ID, &r.Title, &r.BestCurrentPrice); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// UpdateWishlistBestPriceIfLower updates best_current_price and store only when
// the new price is strictly lower than the currently stored value (or there is none).
// Returns true if the row was actually updated.
func (s *Store) UpdateWishlistBestPriceIfLower(id string, price float64, store string) (bool, error) {
	res, err := s.db.Exec(`
UPDATE wishlist_entries
SET best_current_price = ?,
    best_current_store = ?,
    last_price_sync    = datetime('now')
WHERE id = ?
  AND (best_current_price IS NULL OR best_current_price > ?)`,
		price, store, id, price,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		_ = s.InsertWishlistPriceHistory(id, price, store)
	}
	return n > 0, nil
}

// SetWishlistITADId stores the ITAD game UUID for a wishlist entry.
func (s *Store) SetWishlistITADId(wishlistID, itadID string) error {
	_, err := s.db.Exec(`UPDATE wishlist_entries SET itad_id = ? WHERE id = ?`, itadID, wishlistID)
	return err
}

// WishlistPricingUpdate holds price data to write back to a wishlist entry.
type WishlistPricingUpdate struct {
	ID                  string
	BestCurrentPrice    *float64
	BestCurrentStore    *string
	BestPriceURL        *string
	HistoricalLowPrice  *float64
	HistoricalLowStore  *string
}

// UpdateWishlistPricing writes current and historical-low prices to a wishlist entry
// and appends a row to the price history log when a current price is present.
func (s *Store) UpdateWishlistPricing(p WishlistPricingUpdate) error {
	_, err := s.db.Exec(`
UPDATE wishlist_entries SET
    best_current_price   = ?,
    best_current_store   = ?,
    best_price_url       = ?,
    historical_low_price = ?,
    historical_low_store = ?,
    last_price_sync      = datetime('now')
WHERE id = ?`,
		p.BestCurrentPrice, p.BestCurrentStore, p.BestPriceURL,
		p.HistoricalLowPrice, p.HistoricalLowStore,
		p.ID,
	)
	if err != nil {
		return err
	}
	if p.BestCurrentPrice != nil && p.BestCurrentStore != nil {
		_ = s.InsertWishlistPriceHistory(p.ID, *p.BestCurrentPrice, *p.BestCurrentStore)
	}
	return nil
}

// ToggleWishlistFlagRemove flips the flag_remove boolean for a wishlist entry.
func (s *Store) ToggleWishlistFlagRemove(id string) error {
	_, err := s.db.Exec(`UPDATE wishlist_entries SET flag_remove = 1 - flag_remove WHERE id = ?`, id)
	return err
}

// ListFlaggedForRemoval returns all wishlist entries marked for removal.
func (s *Store) ListFlaggedForRemoval() ([]WishlistRow, error) {
	rows, err := s.db.Query(fmt.Sprintf(`
SELECT %s
FROM wishlist_entries w
WHERE w.flag_remove = 1
ORDER BY w.sort_title ASC`, wishlistRowColumns))
	if err != nil {
		return nil, err
	}
	return scanWishlistRows(rows)
}

// DeleteWishlistEntry removes a wishlist entry and all its related rows
// (cascaded via foreign keys).
func (s *Store) DeleteWishlistEntry(id string) error {
	_, err := s.db.Exec(`DELETE FROM wishlist_entries WHERE id = ?`, id)
	return err
}

// UpsertWishlistEntry inserts a wishlist entry. On conflict it updates title/sort_title
// only when the stored title is still a "Steam App XXXXXXX" placeholder.
func (s *Store) UpsertWishlistEntry(id, title, sortTitle string) error {
	_, err := s.db.Exec(`
INSERT INTO wishlist_entries (id, title, sort_title, enrichment_status)
VALUES (?, ?, ?, 'needs_review')
ON CONFLICT(id) DO UPDATE SET
    title      = CASE WHEN wishlist_entries.title LIKE 'Steam App %' THEN excluded.title ELSE wishlist_entries.title END,
    sort_title = CASE WHEN wishlist_entries.title LIKE 'Steam App %' THEN excluded.sort_title ELSE wishlist_entries.sort_title END`,
		id, title, sortTitle,
	)
	return err
}

// UpsertWishlistStoreLink inserts or updates a wishlist_stores row.
func (s *Store) UpsertWishlistStoreLink(wishlistID, store, storeID, storeURL string) error {
	_, err := s.db.Exec(`
INSERT INTO wishlist_stores (wishlist_id, store, store_id, store_url)
VALUES (?, ?, ?, ?)
ON CONFLICT(wishlist_id, store) DO UPDATE SET
    store_id  = excluded.store_id,
    store_url = excluded.store_url`,
		wishlistID, store, storeID, storeURL,
	)
	return err
}

// ── Device + install-state queries ───────────────────────────────────────────

// UpsertDevice creates or refreshes a device record.
func (s *Store) UpsertDevice(id, name, platform string) error {
	if platform == "" {
		platform = "other"
	}
	_, err := s.db.Exec(`
INSERT INTO devices (id, name, label, platform, last_seen)
VALUES (?, ?, ?, ?, datetime('now'))
ON CONFLICT(id) DO UPDATE SET
    name      = excluded.name,
    label     = excluded.label,
    platform  = excluded.platform,
    last_seen = excluded.last_seen`,
		id, name, name, platform,
	)
	return err
}

// SyncInstallSources replaces all install records for a device with the
// supplied list, then recomputes games.is_installed across the whole library.
// Returns (installed, notFound, err).
func (s *Store) SyncInstallSources(deviceID, deviceName, platform string, inputs []InstallInput) (installed, notFound int, err error) {
	if err = s.UpsertDevice(deviceID, deviceName, platform); err != nil {
		return
	}

	// Full refresh: clear then re-insert.
	if _, err = s.db.Exec(`DELETE FROM game_install_sources WHERE device_id = ?`, deviceID); err != nil {
		return
	}

	for _, inp := range inputs {
		var gameID string
		qErr := s.db.QueryRow(
			`SELECT game_id FROM game_stores WHERE store = ? AND store_id = ? LIMIT 1`,
			inp.Store, inp.StoreID,
		).Scan(&gameID)
		if qErr == sql.ErrNoRows {
			notFound++
			continue
		}
		if qErr != nil {
			err = qErr
			return
		}

		_, iErr := s.db.Exec(`
INSERT INTO game_install_sources (game_id, device_id, install_path, install_size_bytes, runner, last_seen)
VALUES (?, ?, ?, ?, ?, datetime('now'))`,
			gameID, deviceID, inp.InstallDir, inp.SizeBytes, inp.Store,
		)
		if iErr != nil {
			err = iErr
			return
		}
		installed++
	}

	// Recompute is_installed on every game.
	_, err = s.db.Exec(`
UPDATE games SET is_installed = CASE
    WHEN EXISTS (SELECT 1 FROM game_install_sources WHERE game_id = games.id) THEN 1
    ELSE 0
END`)
	return
}

// ── Sync error log ────────────────────────────────────────────────────────────

// AppendSyncErrors bulk-inserts per-item error lines for one sync run.
// runID should be the run's start time formatted as time.RFC3339.
func (s *Store) AppendSyncErrors(syncType, runID string, errors []string) error {
	if len(errors) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO sync_errors (sync_type, run_id, message) VALUES (?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, msg := range errors {
		if _, err := stmt.Exec(syncType, runID, msg); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// RecentSyncErrors returns the most recent error lines, optionally filtered
// by sync type (pass "" for all types). Returns up to limit rows.
func (s *Store) RecentSyncErrors(syncType string, limit int) ([]SyncError, error) {
	var rows *sql.Rows
	var err error
	if syncType == "" {
		rows, err = s.db.Query(
			`SELECT id, sync_type, run_id, message, created_at FROM sync_errors ORDER BY id DESC LIMIT ?`,
			limit,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, sync_type, run_id, message, created_at FROM sync_errors WHERE sync_type = ? ORDER BY id DESC LIMIT ?`,
			syncType, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SyncError
	for rows.Next() {
		var e SyncError
		if err := rows.Scan(&e.ID, &e.SyncType, &e.RunID, &e.Message, &e.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// SyncErrorTypes returns the distinct sync types that have logged errors.
func (s *Store) SyncErrorTypes() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT sync_type FROM sync_errors ORDER BY sync_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		types = append(types, t)
	}
	return types, rows.Err()
}

// ── Sync log queries ──────────────────────────────────────────────────────────

func (s *Store) RecentSyncs(limit int) ([]SyncLog, error) {
	rows, err := s.db.Query(
		`SELECT id, type, status, started_at, finished_at, games_added, games_updated, error_message FROM sync_log ORDER BY started_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SyncLog
	for rows.Next() {
		var r SyncLog
		if err := rows.Scan(&r.ID, &r.Type, &r.Status, &r.StartedAt, &r.FinishedAt, &r.GamesAdded, &r.GamesUpdated, &r.ErrorMessage); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) LatestSyncByType(syncType string) (*SyncLog, error) {
	var r SyncLog
	err := s.db.QueryRow(
		`SELECT id, type, status, started_at, finished_at, games_added, games_updated, error_message FROM sync_log WHERE type = ? ORDER BY started_at DESC LIMIT 1`,
		syncType,
	).Scan(&r.ID, &r.Type, &r.Status, &r.StartedAt, &r.FinishedAt, &r.GamesAdded, &r.GamesUpdated, &r.ErrorMessage)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &r, err
}

func (s *Store) StartSync(syncType string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO sync_log (type, status, started_at) VALUES (?, 'running', CURRENT_TIMESTAMP)`,
		syncType,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinishSync(id int64, status string, added, updated int, errMsg string) error {
	var errVal any
	if errMsg != "" {
		errVal = errMsg
	}
	_, err := s.db.Exec(`
UPDATE sync_log SET status = ?, finished_at = CURRENT_TIMESTAMP,
    games_added = ?, games_updated = ?, error_message = ?
WHERE id = ?`, status, added, updated, errVal, id)
	return err
}

// ── Dashboard queries ─────────────────────────────────────────────────────────

func (s *Store) GetDashboardStats() (DashboardStats, error) {
	var d DashboardStats
	err := s.db.QueryRow(`
SELECT
    (SELECT COUNT(*) FROM games              WHERE parent_id IS NULL)                             AS library_count,
    (SELECT COUNT(*) FROM wishlist_entries)                                                       AS wishlist_count,
    (SELECT COUNT(*) FROM games              WHERE is_installed = 1 AND parent_id IS NULL)        AS installed_count,
    (SELECT COUNT(*) FROM games              WHERE enrichment_status = 'needs_review'
                                               AND parent_id IS NULL)                             AS unmatched_count
`).Scan(&d.LibraryCount, &d.WishlistCount, &d.InstalledCount, &d.UnmatchedCount)
	return d, err
}

// wishlistRowColumns is the SELECT column list shared by dashboard wishlist queries.
const wishlistRowColumns = `
w.id, w.library_id, w.title, w.sort_title, w.enrichment_status,
w.artwork, w.steam_deck_verified, w.currency,
w.best_current_price, w.best_current_store,
w.historical_low_price, w.historical_low_store,
w.target_price, w.priority, w.date_added,
(SELECT GROUP_CONCAT(tag)   FROM wishlist_tags   WHERE wishlist_id = w.id) AS tags,
(SELECT GROUP_CONCAT(store) FROM wishlist_stores WHERE wishlist_id = w.id) AS stores,
w.flag_remove`

func scanWishlistRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}) ([]WishlistRow, error) {
	defer rows.Close()
	var result []WishlistRow
	for rows.Next() {
		var r WishlistRow
		if err := rows.Scan(
			&r.ID, &r.LibraryID, &r.Title, &r.SortTitle, &r.EnrichmentStatus,
			&r.ArtworkRaw, &r.SteamDeckVerifiedRaw, &r.Currency,
			&r.BestCurrentPrice, &r.BestCurrentStore,
			&r.HistoricalLowPrice, &r.HistoricalLowStore,
			&r.TargetPrice, &r.Priority, &r.DateAdded,
			&r.TagsRaw, &r.StoresRaw,
			&r.FlagRemove,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ListPriceAlerts returns wishlist entries whose current price is at or below
// the user's target price or the $2.00 instant-buy threshold.
func (s *Store) ListPriceAlerts() ([]WishlistRow, error) {
	rows, err := s.db.Query(fmt.Sprintf(`
SELECT %s
FROM wishlist_entries w
WHERE w.best_current_price IS NOT NULL
  AND (
      (w.target_price IS NOT NULL AND w.best_current_price <= w.target_price)
      OR w.best_current_price <= 2.00
  )
ORDER BY w.best_current_price ASC
LIMIT 10`, wishlistRowColumns))
	if err != nil {
		return nil, err
	}
	return scanWishlistRows(rows)
}

// ListNearHistoricalLow returns wishlist entries whose current price is within
// 10% above their all-time low, excluding items already in price alerts.
func (s *Store) ListNearHistoricalLow() ([]WishlistRow, error) {
	rows, err := s.db.Query(fmt.Sprintf(`
SELECT %s
FROM wishlist_entries w
WHERE w.best_current_price IS NOT NULL
  AND w.historical_low_price IS NOT NULL
  AND w.best_current_price <= w.historical_low_price * 1.10
  AND w.best_current_price > 2.00
  AND (w.target_price IS NULL OR w.best_current_price > w.target_price)
ORDER BY w.best_current_price / w.historical_low_price ASC
LIMIT 8`, wishlistRowColumns))
	if err != nil {
		return nil, err
	}
	return scanWishlistRows(rows)
}

// ListHighPriorityWishlist returns priority-3 wishlist entries sorted by price.
func (s *Store) ListHighPriorityWishlist() ([]WishlistRow, error) {
	rows, err := s.db.Query(fmt.Sprintf(`
SELECT %s
FROM wishlist_entries w
WHERE w.priority >= 3
ORDER BY CASE WHEN w.best_current_price IS NULL THEN 1 ELSE 0 END,
         w.best_current_price ASC
LIMIT 10`, wishlistRowColumns))
	if err != nil {
		return nil, err
	}
	return scanWishlistRows(rows)
}

// ── Config queries ────────────────────────────────────────────────────────────

func (s *Store) GetConfig(key string) (string, error) {
	var val string
	err := s.db.QueryRow(`SELECT value FROM app_config WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func (s *Store) SetConfig(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO app_config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

func (s *Store) AllConfig() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM app_config`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = v
	}
	return result, rows.Err()
}

// ── Enrichment queue ──────────────────────────────────────────────────────────

func (s *Store) EnqueueEnrichment(entityType, entityID string) error {
	_, err := s.db.Exec(`
INSERT INTO enrichment_queue (entity_type, entity_id, status)
VALUES (?, ?, 'pending')
ON CONFLICT(entity_type, entity_id) DO UPDATE SET status = 'pending', attempts = 0, last_error = NULL`,
		entityType, entityID,
	)
	return err
}

func (s *Store) QueueCounts() (QueueCounts, error) {
	var q QueueCounts
	err := s.db.QueryRow(`
SELECT
    COUNT(*) FILTER (WHERE status = 'pending'),
    COUNT(*) FILTER (WHERE status = 'running'),
    COUNT(*) FILTER (WHERE status = 'failed')
FROM enrichment_queue`).Scan(&q.Pending, &q.Running, &q.Failed)
	return q, err
}

// ── Review queue ──────────────────────────────────────────────────────────────

type ReviewEntry struct {
	ID        string
	Title     string
	ArtworkRaw sql.NullString
	StoreInfo string
}

func (r ReviewEntry) Artwork() Artwork { return ParseArtwork(r.ArtworkRaw) }

func (s *Store) ListNeedsReview(offset int) ([]ReviewEntry, error) {
	rows, err := s.db.Query(`
SELECT
    g.id, g.title, g.artwork,
    COALESCE((SELECT GROUP_CONCAT(store || ':' || COALESCE(store_id, ''))
              FROM game_stores WHERE game_id = g.id AND owned = 1), '') AS store_info
FROM games g
WHERE g.enrichment_status = 'needs_review' AND g.parent_id IS NULL
ORDER BY g.sort_title ASC
LIMIT 30 OFFSET ?`, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ReviewEntry
	for rows.Next() {
		var r ReviewEntry
		if err := rows.Scan(&r.ID, &r.Title, &r.ArtworkRaw, &r.StoreInfo); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) SetEnrichmentStatus(id, status string) error {
	_, err := s.db.Exec(
		`UPDATE games SET enrichment_status = ?, last_enriched = CURRENT_TIMESTAMP WHERE id = ?`,
		status, id,
	)
	return err
}

func (s *Store) SetIGDBMatch(gameID, igdbID string) error {
	_, err := s.db.Exec(
		`UPDATE games SET igdb_id = ?, enrichment_status = 'manual', last_enriched = CURRENT_TIMESTAMP WHERE id = ?`,
		igdbID, gameID,
	)
	return err
}

// GameNeedsReviewRow is a minimal row used by the enrichment pipeline.
type GameNeedsReviewRow struct {
	ID    string
	Title string
}

// ListNeedsReviewTitles returns id+title for every game awaiting enrichment.
func (s *Store) ListNeedsReviewTitles() ([]GameNeedsReviewRow, error) {
	rows, err := s.db.Query(`
SELECT id, title FROM games
WHERE enrichment_status = 'needs_review' AND parent_id IS NULL
ORDER BY sort_title ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []GameNeedsReviewRow
	for rows.Next() {
		var r GameNeedsReviewRow
		if err := rows.Scan(&r.ID, &r.Title); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// EnrichGameParams carries IGDB metadata for a matched game.
type EnrichGameParams struct {
	ID          string
	IGDBId      int64
	ArtworkJSON string
	Description *string
	ReleaseDate *string
}

// EnrichGame saves IGDB metadata and marks the game as 'matched'.
// Description and ReleaseDate use COALESCE so they don't overwrite existing values.
func (s *Store) EnrichGame(p EnrichGameParams) error {
	_, err := s.db.Exec(`
UPDATE games SET
    igdb_id           = ?,
    artwork           = ?,
    description       = COALESCE(?, description),
    release_date      = COALESCE(?, release_date),
    enrichment_status = 'matched',
    last_enriched     = CURRENT_TIMESTAMP
WHERE id = ?`,
		p.IGDBId, p.ArtworkJSON, p.Description, p.ReleaseDate, p.ID,
	)
	return err
}

// ListWishlistNeedsEnrichment returns id+title for wishlist entries awaiting enrichment.
func (s *Store) ListWishlistNeedsEnrichment() ([]GameNeedsReviewRow, error) {
	rows, err := s.db.Query(`
SELECT id, title FROM wishlist_entries
WHERE enrichment_status = 'needs_review'
ORDER BY sort_title ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []GameNeedsReviewRow
	for rows.Next() {
		var r GameNeedsReviewRow
		if err := rows.Scan(&r.ID, &r.Title); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// EnrichWishlistEntry saves IGDB metadata to a wishlist entry and marks it 'matched'.
func (s *Store) EnrichWishlistEntry(p EnrichGameParams) error {
	_, err := s.db.Exec(`
UPDATE wishlist_entries SET
    igdb_id           = ?,
    artwork           = ?,
    enrichment_status = 'matched',
    last_enriched     = CURRENT_TIMESTAMP
WHERE id = ?`,
		p.IGDBId, p.ArtworkJSON, p.ID,
	)
	return err
}

// ── Import helpers ────────────────────────────────────────────────────────────

// FindGameByStoreID returns the game_id for an existing store link, or "" if none.
func (s *Store) FindGameByStoreID(store, storeID string) (string, error) {
	var gameID string
	err := s.db.QueryRow(
		`SELECT game_id FROM game_stores WHERE store = ? AND store_id = ?`,
		store, storeID,
	).Scan(&gameID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return gameID, err
}

func (s *Store) FindGameByTitle(title string) (string, error) {
	var id string
	err := s.db.QueryRow(`SELECT id FROM games WHERE title = ? OR sort_title = ? LIMIT 1`, title, makeSortTitle(title)).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

// InsertGameParams holds fields for a new game record.
type InsertGameParams struct {
	ID               string
	Title            string
	SortTitle        string
	Developer        *string
	Description      *string
	ShortDescription *string
	ReleaseDate      *string
	ArtworkJSON      string
	Windows          bool
	Mac              bool
	Linux            bool
}

// InsertGame inserts a new game row; silently skips if the ID already exists.
func (s *Store) InsertGame(p InsertGameParams) error {
	_, err := s.db.Exec(`
INSERT OR IGNORE INTO games
    (id, title, sort_title, developer, description, short_description,
     release_date, enrichment_status, artwork, windows, mac, linux)
VALUES (?, ?, ?, ?, ?, ?, ?, 'needs_review', ?, ?, ?, ?)`,
		p.ID, p.Title, p.SortTitle, p.Developer,
		p.Description, p.ShortDescription, p.ReleaseDate,
		p.ArtworkJSON, p.Windows, p.Mac, p.Linux,
	)
	return err
}

// UpsertGameStoreLink inserts or updates a game_stores row.
func (s *Store) UpsertGameStoreLink(gameID, store, storeID, storeURL string) error {
	var sidVal, surlVal any
	if storeID != "" {
		sidVal = storeID
	}
	if storeURL != "" {
		surlVal = storeURL
	}
	_, err := s.db.Exec(`
INSERT INTO game_stores (game_id, store, store_id, store_url, owned)
VALUES (?, ?, ?, ?, 1)
ON CONFLICT(game_id, store) DO UPDATE SET
    store_id  = excluded.store_id,
    store_url = excluded.store_url,
    owned     = 1`,
		gameID, store, sidVal, surlVal,
	)
	return err
}

// UpsertGenre inserts a genre tag for a game; silently skips duplicates.
func (s *Store) UpsertGenre(gameID, genre string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO game_genres (game_id, genre) VALUES (?, ?)`,
		gameID, genre,
	)
	return err
}

// UpdatePlayTimeIfGreater sets play_time_minutes only when the new value exceeds the stored one.
func (s *Store) UpdatePlayTimeIfGreater(gameID string, minutes int) error {
	_, err := s.db.Exec(`
UPDATE games SET play_time_minutes = ?
WHERE id = ? AND (play_time_minutes IS NULL OR play_time_minutes < ?)`,
		minutes, gameID, minutes,
	)
	return err
}

// UpdateLastPlayedIfLater sets last_played only when t is more recent than the stored value.
func (s *Store) UpdateLastPlayedIfLater(gameID string, t time.Time) error {
	_, err := s.db.Exec(`
UPDATE games SET last_played = ?
WHERE id = ? AND (last_played IS NULL OR last_played < ?)`,
		t, gameID, t,
	)
	return err
}

// SetProtonRating stores a ProtonDB tier string for a game.
func (s *Store) SetProtonRating(gameID, rating string) error {
	_, err := s.db.Exec(`UPDATE games SET proton_rating = ? WHERE id = ?`, rating, gameID)
	return err
}

// ListGamesNeedingProtonRating returns Steam games that have no proton_rating yet.
func (s *Store) ListGamesNeedingProtonRating() ([]struct{ ID, SteamAppID string }, error) {
	rows, err := s.db.Query(`
SELECT g.id, gs.store_id
FROM games g
JOIN game_stores gs ON gs.game_id = g.id AND gs.store = 'steam' AND gs.owned = 1
WHERE g.proton_rating IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []struct{ ID, SteamAppID string }
	for rows.Next() {
		var r struct{ ID, SteamAppID string }
		if err := rows.Scan(&r.ID, &r.SteamAppID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// InsertWishlistPriceHistory logs a price observation for a wishlist entry.
func (s *Store) InsertWishlistPriceHistory(wishlistID string, price float64, store string) error {
	_, err := s.db.Exec(`
INSERT INTO wishlist_price_history (wishlist_id, price, store, recorded_at)
VALUES (?, ?, ?, datetime('now'))`,
		wishlistID, price, store,
	)
	return err
}

// UpsertContent inserts a bundled DLC/content entry; skips if title already exists.
func (s *Store) UpsertContent(gameID, contentType, title, storeID string) error {
	var sidVal any
	if storeID != "" {
		sidVal = storeID
	}
	_, err := s.db.Exec(`
INSERT INTO game_contents (game_id, content_type, title, store_id)
VALUES (?, ?, ?, ?)
ON CONFLICT(game_id, title) DO NOTHING`,
		gameID, contentType, title, sidVal,
	)
	return err
}

// ── Mystery packs ────────────────────────────────────────────────────────────

type MysteryPackSite struct {
	ID      string
	Name    string
	BaseURL string
	Enabled bool
}

type MysteryPackRow struct {
	ID               string
	SiteID           string
	SiteName         string
	Name             string
	URL              string
	PackType         string
	PriceUSD         *float64
	KeyCount         int
	Enabled          bool
	LastPriced       *string
	LastAnalyzed     *string
	RecommendationBadge *string
	ROIKeyshop       *float64
	OverlapCount     *int
}

type MysteryPackGame struct {
	Title            string
	SteamAppID       *string
	RetailPriceUSD   *float64
	KeyshopPriceUSD  *float64
	PriceUpdatedAt   *string
}

type MysteryPackAnalysis struct {
	PackID            string
	AnalyzedAt        string
	PackPriceUSD      *float64
	PoolSize          *int
	OverlapCount      *int
	NewGamesCount     *int
	KeyshopValueTotal *float64
	KeyshopValueNew   *float64
	ROIKeyshop        *float64
	ROIPerKey         *float64
	VarianceScore     *int
	Recommendation    *string
	OverlapTitles     []string
	NotableGames      []string
}

type MysteryPackDetail struct {
	ID               string
	SiteID           string
	SiteName         string
	Name             string
	URL              string
	PackType         string
	PriceUSD         *float64
	KeyCount         int
	ValueSpec        string
	Notes            *string
	Enabled          bool
	LastPriced       *string
	Games            []MysteryPackGame
	Analysis         *MysteryPackAnalysis
}

type MysteryPackParams struct {
	ID        string
	SiteID    string
	Name      string
	URL       string
	PackType  string
	PriceUSD  *float64
	KeyCount  int
	ValueSpec string
	Notes     *string
	Enabled   bool
}

type MysteryPackAnalysisParams struct {
	PackID            string
	AnalyzedAt        string
	PackPriceUSD      *float64
	PoolSize          *int
	OverlapCount      *int
	NewGamesCount     *int
	KeyshopValueTotal *float64
	KeyshopValueNew   *float64
	ROIKeyshop        *float64
	ROIPerKey         *float64
	VarianceScore     *int
	Recommendation    *string
	OverlapTitles     []string
	NotableGames      []string
}

type ScrapeQueue struct {
	ID        string
	ScrapedAt string
	CreatedAt string
	PagesJSON string
	AppliedAt *string
}

type MysteryPackOffer struct {
	PackID     string
	SellerID   string
	PriceUSD   float64
	URL        *string
	ValidUntil *string
	UpdatedAt  string
}

type MysteryPackPriceHistoryRow struct {
	ID         int64
	PackID     string
	SellerID   string
	PriceUSD   float64
	URL        *string
	RecordedAt string
}

func (s *Store) ListMysteryPackSites() ([]MysteryPackSite, error) {
	rows, err := s.db.Query(`
SELECT id, name, base_url, enabled FROM mystery_pack_sites
ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MysteryPackSite
	for rows.Next() {
		var site MysteryPackSite
		if err := rows.Scan(&site.ID, &site.Name, &site.BaseURL, &site.Enabled); err != nil {
			return nil, err
		}
		result = append(result, site)
	}
	return result, rows.Err()
}

func (s *Store) UpsertMysteryPackSite(id, name, baseURL string) error {
	_, err := s.db.Exec(`
INSERT INTO mystery_pack_sites (id, name, base_url, enabled)
VALUES (?, ?, ?, 1)
ON CONFLICT(id) DO UPDATE SET name = excluded.name, base_url = excluded.base_url`,
		id, name, baseURL,
	)
	return err
}

func (s *Store) DeleteMysteryPackSite(id string) error {
	_, err := s.db.Exec(`DELETE FROM mystery_pack_sites WHERE id = ?`, id)
	return err
}

func (s *Store) ListMysteryPacks() ([]MysteryPackRow, error) {
	rows, err := s.db.Query(`
SELECT
    mp.id, mp.site_id, mps.name, mp.name, mp.url, mp.pack_type,
    mp.price_usd, mp.key_count, mp.enabled, mp.last_priced,
    mpa.analyzed_at, mpa.recommendation, mpa.roi_keyshop, mpa.overlap_count
FROM mystery_packs mp
JOIN mystery_pack_sites mps ON mps.id = mp.site_id
LEFT JOIN mystery_pack_analysis mpa ON mpa.pack_id = mp.id
ORDER BY mps.name ASC, mp.name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MysteryPackRow
	for rows.Next() {
		var row MysteryPackRow
		if err := rows.Scan(
			&row.ID, &row.SiteID, &row.SiteName, &row.Name, &row.URL, &row.PackType,
			&row.PriceUSD, &row.KeyCount, &row.Enabled, &row.LastPriced,
			&row.LastAnalyzed, &row.RecommendationBadge, &row.ROIKeyshop, &row.OverlapCount,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *Store) ListMysteryPacksBySite(siteID string) ([]MysteryPackRow, error) {
	rows, err := s.db.Query(`
SELECT
    mp.id, mp.site_id, mps.name, mp.name, mp.url, mp.pack_type,
    mp.price_usd, mp.key_count, mp.enabled, mp.last_priced,
    mpa.analyzed_at, mpa.recommendation, mpa.roi_keyshop, mpa.overlap_count
FROM mystery_packs mp
JOIN mystery_pack_sites mps ON mps.id = mp.site_id
LEFT JOIN mystery_pack_analysis mpa ON mpa.pack_id = mp.id
WHERE mp.site_id = ?
ORDER BY mp.name ASC`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MysteryPackRow
	for rows.Next() {
		var row MysteryPackRow
		if err := rows.Scan(
			&row.ID, &row.SiteID, &row.SiteName, &row.Name, &row.URL, &row.PackType,
			&row.PriceUSD, &row.KeyCount, &row.Enabled, &row.LastPriced,
			&row.LastAnalyzed, &row.RecommendationBadge, &row.ROIKeyshop, &row.OverlapCount,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *Store) GetMysteryPack(id string) (*MysteryPackDetail, error) {
	var pack MysteryPackDetail
	err := s.db.QueryRow(`
SELECT
    mp.id, mp.site_id, mps.name, mp.name, mp.url, mp.pack_type,
    mp.price_usd, mp.key_count, mp.value_spec, mp.notes, mp.enabled, mp.last_priced
FROM mystery_packs mp
JOIN mystery_pack_sites mps ON mps.id = mp.site_id
WHERE mp.id = ?`, id).Scan(
		&pack.ID, &pack.SiteID, &pack.SiteName, &pack.Name, &pack.URL, &pack.PackType,
		&pack.PriceUSD, &pack.KeyCount, &pack.ValueSpec, &pack.Notes, &pack.Enabled, &pack.LastPriced,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	games, err := s.ListMysteryPackGames(id)
	if err != nil {
		return nil, err
	}
	pack.Games = games

	analysis, err := s.GetMysteryPackAnalysis(id)
	if err != nil {
		return nil, err
	}
	pack.Analysis = analysis

	return &pack, nil
}

func (s *Store) UpsertMysteryPack(p MysteryPackParams) error {
	var priceVal any
	if p.PriceUSD != nil {
		priceVal = *p.PriceUSD
	}
	var notesVal any
	if p.Notes != nil {
		notesVal = *p.Notes
	}
	_, err := s.db.Exec(`
INSERT INTO mystery_packs (id, site_id, name, url, pack_type, price_usd, key_count, value_spec, notes, enabled, last_priced)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CASE WHEN ? IS NOT NULL THEN datetime('now') ELSE NULL END)
ON CONFLICT(id) DO UPDATE SET
    site_id = excluded.site_id,
    name = excluded.name,
    url = excluded.url,
    pack_type = excluded.pack_type,
    price_usd = excluded.price_usd,
    key_count = excluded.key_count,
    value_spec = excluded.value_spec,
    notes = excluded.notes,
    enabled = excluded.enabled,
    last_priced = CASE WHEN excluded.price_usd IS NOT NULL THEN datetime('now') ELSE last_priced END`,
		p.ID, p.SiteID, p.Name, p.URL, p.PackType, priceVal, p.KeyCount, p.ValueSpec, notesVal, p.Enabled, priceVal,
	)
	return err
}

func (s *Store) UpdateMysteryPackPrice(id string, priceUSD float64) error {
	_, err := s.db.Exec(`
UPDATE mystery_packs
SET price_usd = ?, last_priced = datetime('now')
WHERE id = ?`, priceUSD, id)
	return err
}

func (s *Store) DeleteMysteryPack(id string) error {
	_, err := s.db.Exec(`DELETE FROM mystery_packs WHERE id = ?`, id)
	return err
}

func (s *Store) ListMysteryPackGames(packID string) ([]MysteryPackGame, error) {
	rows, err := s.db.Query(`
SELECT title, steam_app_id, retail_price_usd, keyshop_price_usd, price_updated_at
FROM mystery_pack_games
WHERE pack_id = ?
ORDER BY title ASC`, packID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MysteryPackGame
	for rows.Next() {
		var game MysteryPackGame
		if err := rows.Scan(&game.Title, &game.SteamAppID, &game.RetailPriceUSD, &game.KeyshopPriceUSD, &game.PriceUpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, game)
	}
	return result, rows.Err()
}

func (s *Store) UpsertMysteryPackGame(packID, title, steamAppID string, retailPrice float64) error {
	var appIDVal any
	if steamAppID != "" {
		appIDVal = steamAppID
	}
	var retailVal any
	if retailPrice > 0 {
		retailVal = retailPrice
	}
	_, err := s.db.Exec(`
INSERT INTO mystery_pack_games (pack_id, title, steam_app_id, retail_price_usd)
VALUES (?, ?, ?, ?)
ON CONFLICT(pack_id, title) DO UPDATE SET
    steam_app_id = excluded.steam_app_id,
    retail_price_usd = excluded.retail_price_usd`,
		packID, title, appIDVal, retailVal,
	)
	return err
}

func (s *Store) UpdateMysteryPackGameKeyshopPrice(packID, title string, keyshopPrice float64, updatedAt string) error {
	_, err := s.db.Exec(`
UPDATE mystery_pack_games
SET keyshop_price_usd = ?, price_updated_at = ?
WHERE pack_id = ? AND title = ?`, keyshopPrice, updatedAt, packID, title)
	return err
}

func (s *Store) DeleteMysteryPackGame(packID, title string) error {
	_, err := s.db.Exec(`DELETE FROM mystery_pack_games WHERE pack_id = ? AND title = ?`, packID, title)
	return err
}

func (s *Store) SaveMysteryPackAnalysis(a MysteryPackAnalysisParams) error {
	overlapTitles, _ := json.Marshal(a.OverlapTitles)
	notableGames, _ := json.Marshal(a.NotableGames)

	_, err := s.db.Exec(`
INSERT INTO mystery_pack_analysis (
    pack_id, analyzed_at, pack_price_usd, pool_size, overlap_count, new_games_count,
    keyshop_value_total, keyshop_value_new, roi_keyshop, roi_per_key, variance_score,
    recommendation, overlap_titles, notable_games
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(pack_id) DO UPDATE SET
    analyzed_at = excluded.analyzed_at,
    pack_price_usd = excluded.pack_price_usd,
    pool_size = excluded.pool_size,
    overlap_count = excluded.overlap_count,
    new_games_count = excluded.new_games_count,
    keyshop_value_total = excluded.keyshop_value_total,
    keyshop_value_new = excluded.keyshop_value_new,
    roi_keyshop = excluded.roi_keyshop,
    roi_per_key = excluded.roi_per_key,
    variance_score = excluded.variance_score,
    recommendation = excluded.recommendation,
    overlap_titles = excluded.overlap_titles,
    notable_games = excluded.notable_games`,
		a.PackID, a.AnalyzedAt, a.PackPriceUSD, a.PoolSize, a.OverlapCount, a.NewGamesCount,
		a.KeyshopValueTotal, a.KeyshopValueNew, a.ROIKeyshop, a.ROIPerKey, a.VarianceScore,
		a.Recommendation, string(overlapTitles), string(notableGames),
	)
	return err
}

func (s *Store) GetMysteryPackAnalysis(packID string) (*MysteryPackAnalysis, error) {
	var a MysteryPackAnalysis
	var overlapTitlesJSON, notableGamesJSON []byte

	err := s.db.QueryRow(`
SELECT
    pack_id, analyzed_at, pack_price_usd, pool_size, overlap_count, new_games_count,
    keyshop_value_total, keyshop_value_new, roi_keyshop, roi_per_key, variance_score,
    recommendation, COALESCE(overlap_titles, '[]'), COALESCE(notable_games, '[]')
FROM mystery_pack_analysis
WHERE pack_id = ?`, packID).Scan(
		&a.PackID, &a.AnalyzedAt, &a.PackPriceUSD, &a.PoolSize, &a.OverlapCount, &a.NewGamesCount,
		&a.KeyshopValueTotal, &a.KeyshopValueNew, &a.ROIKeyshop, &a.ROIPerKey, &a.VarianceScore,
		&a.Recommendation, &overlapTitlesJSON, &notableGamesJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal(overlapTitlesJSON, &a.OverlapTitles)
	_ = json.Unmarshal(notableGamesJSON, &a.NotableGames)

	return &a, nil
}

func (s *Store) ListOwnedTitleIndex() (map[string]struct{}, error) {
	rows, err := s.db.Query(`
SELECT DISTINCT LOWER(TRIM(g.title))
FROM games g
JOIN game_stores gs ON gs.game_id = g.id
WHERE gs.owned = 1
ORDER BY g.title ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]struct{})
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			return nil, err
		}
		result[title] = struct{}{}
	}
	return result, rows.Err()
}

// ListWishlistTitleIndex returns a map of normalized wishlist entry titles for local game matching.
func (s *Store) ListWishlistTitleIndex() (map[string]struct{}, error) {
	rows, err := s.db.Query(`
SELECT DISTINCT LOWER(TRIM(title))
FROM wishlist_entries`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]struct{})
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			return nil, err
		}
		result[title] = struct{}{}
	}
	return result, rows.Err()
}

// Scrape queue management

func (s *Store) CreateScrapeQueue(id, scrapedAt, createdAt, pagesJSON string) error {
	_, err := s.db.Exec(`
INSERT INTO mystery_pack_scrape_queues (id, scraped_at, created_at, pages_json, applied_at)
VALUES (?, ?, ?, ?, NULL)`,
		id, scrapedAt, createdAt, pagesJSON)
	return err
}

func (s *Store) GetScrapeQueue(id string) (*ScrapeQueue, error) {
	var q ScrapeQueue
	var appliedAt sql.NullString
	err := s.db.QueryRow(`
SELECT id, scraped_at, created_at, pages_json, applied_at
FROM mystery_pack_scrape_queues
WHERE id = ?`,
		id).Scan(&q.ID, &q.ScrapedAt, &q.CreatedAt, &q.PagesJSON, &appliedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if appliedAt.Valid {
		q.AppliedAt = &appliedAt.String
	}
	return &q, nil
}

func (s *Store) MarkQueueApplied(id, appliedAt string) error {
	_, err := s.db.Exec(`
UPDATE mystery_pack_scrape_queues
SET applied_at = ?
WHERE id = ?`,
		appliedAt, id)
	return err
}

func (s *Store) DeleteExpiredScrapeQueues(olderThan string) error {
	_, err := s.db.Exec(`
DELETE FROM mystery_pack_scrape_queues
WHERE created_at < ?`,
		olderThan)
	return err
}

// Pack lookup

func (s *Store) GetMysteryPackBySiteAndTitle(siteID, normalizedTitle string) (*MysteryPackRow, error) {
	rows, err := s.db.Query(`
SELECT id, site_id, name, url, pack_type, price_usd, key_count, enabled
FROM mystery_packs
WHERE site_id = ? AND LOWER(TRIM(name)) = LOWER(TRIM(?))
LIMIT 1`,
		siteID, normalizedTitle)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	var p MysteryPackRow
	if err := rows.Scan(&p.ID, &p.SiteID, &p.Name, &p.URL, &p.PackType, &p.PriceUSD, &p.KeyCount, &p.Enabled); err != nil {
		return nil, err
	}
	return &p, rows.Err()
}

// Offers (current per-seller prices)

func (s *Store) UpsertMysteryPackOffer(packID, sellerID string, priceUSD float64, url, validUntil, updatedAt string) error {
	_, err := s.db.Exec(`
INSERT INTO mystery_pack_offers (pack_id, seller_id, price_usd, url, valid_until, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(pack_id, seller_id) DO UPDATE SET
	price_usd = excluded.price_usd,
	url = excluded.url,
	valid_until = excluded.valid_until,
	updated_at = excluded.updated_at`,
		packID, sellerID, priceUSD, url, validUntil, updatedAt)
	return err
}

func (s *Store) ListMysteryPackOffers(packID string) ([]MysteryPackOffer, error) {
	rows, err := s.db.Query(`
SELECT pack_id, seller_id, price_usd, url, valid_until, updated_at
FROM mystery_pack_offers
WHERE pack_id = ?
ORDER BY price_usd ASC`,
		packID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MysteryPackOffer
	for rows.Next() {
		var o MysteryPackOffer
		var url, validUntil sql.NullString
		if err := rows.Scan(&o.PackID, &o.SellerID, &o.PriceUSD, &url, &validUntil, &o.UpdatedAt); err != nil {
			return nil, err
		}
		if url.Valid {
			o.URL = &url.String
		}
		if validUntil.Valid {
			o.ValidUntil = &validUntil.String
		}
		result = append(result, o)
	}
	return result, rows.Err()
}

func (s *Store) DeleteMysteryPackOffer(packID, sellerID string) error {
	_, err := s.db.Exec(`
DELETE FROM mystery_pack_offers
WHERE pack_id = ? AND seller_id = ?`,
		packID, sellerID)
	return err
}

// Price history

func (s *Store) InsertPriceHistory(packID, sellerID string, priceUSD float64, url, recordedAt string) error {
	_, err := s.db.Exec(`
INSERT INTO mystery_pack_price_history (pack_id, seller_id, price_usd, url, recorded_at)
VALUES (?, ?, ?, ?, ?)`,
		packID, sellerID, priceUSD, url, recordedAt)
	return err
}

func (s *Store) ListPriceHistory(packID string) ([]MysteryPackPriceHistoryRow, error) {
	rows, err := s.db.Query(`
SELECT id, pack_id, seller_id, price_usd, url, recorded_at
FROM mystery_pack_price_history
WHERE pack_id = ?
ORDER BY recorded_at DESC`,
		packID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MysteryPackPriceHistoryRow
	for rows.Next() {
		var h MysteryPackPriceHistoryRow
		var url sql.NullString
		if err := rows.Scan(&h.ID, &h.PackID, &h.SellerID, &h.PriceUSD, &url, &h.RecordedAt); err != nil {
			return nil, err
		}
		if url.Valid {
			h.URL = &url.String
		}
		result = append(result, h)
	}
	return result, rows.Err()
}

// SyncPackLowestPrice updates the pack's price_usd to the MIN of current offers.
func (s *Store) SyncPackLowestPrice(packID string) error {
	_, err := s.db.Exec(`
UPDATE mystery_packs
SET price_usd = (
	SELECT MIN(price_usd)
	FROM mystery_pack_offers
	WHERE pack_id = ?
)
WHERE id = ?`,
		packID, packID)
	return err
}
