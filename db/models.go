package db

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// ── Artwork ──────────────────────────────────────────────────────────────────

type ArtworkRef struct {
	URL    string `json:"url"`
	Source string `json:"source"`
}

type Artwork struct {
	Cover      ArtworkRef `json:"cover"`
	Square     ArtworkRef `json:"square"`
	Background ArtworkRef `json:"background"`
	Logo       ArtworkRef `json:"logo"`
	Icon       ArtworkRef `json:"icon"`
}

func ParseArtwork(raw sql.NullString) Artwork {
	if !raw.Valid || raw.String == "" {
		return Artwork{}
	}
	var a Artwork
	_ = json.Unmarshal([]byte(raw.String), &a)
	return a
}

// ── Game (list view) ─────────────────────────────────────────────────────────

type GameListRow struct {
	ID                    string
	Title                 string
	SortTitle             string
	EnrichmentStatus      string
	IsCompleteEdition     bool
	ParentID              sql.NullString
	ArtworkRaw            sql.NullString
	SteamDeckVerifiedRaw  sql.NullString
	ProtonRatingRaw       sql.NullString
	IsInstalled           bool
	PlayStatusRaw         sql.NullString
	Rating                sql.NullInt64
	IsFavorite            bool
	IsHidden              bool
	PlayTimeMinutes       sql.NullInt64
	GenresRaw             sql.NullString // comma-separated
	TagsRaw               sql.NullString // comma-separated
	OwnedStoresRaw        sql.NullString // comma-separated
	MultiStoreOwned       bool           // true if another game with the same IGDB ID is owned in a different store
}

func (g GameListRow) Artwork() Artwork        { return ParseArtwork(g.ArtworkRaw) }
func (g GameListRow) OwnedStores() []string   { return splitCSV(g.OwnedStoresRaw) }
func (g GameListRow) Genres() []string        { return splitCSV(g.GenresRaw) }
func (g GameListRow) Tags() []string          { return splitCSV(g.TagsRaw) }
func (g GameListRow) HasSeparateDLC() bool    { return false }
// SteamDeckVerified returns a plain string for template eq comparisons.
func (g GameListRow) SteamDeckVerified() string { return g.SteamDeckVerifiedRaw.String }
func (g GameListRow) ProtonRating() string      { return g.ProtonRatingRaw.String }
func (g GameListRow) PlayStatus() string        { return g.PlayStatusRaw.String }

// ── Game (full detail) ───────────────────────────────────────────────────────

type GameDetail struct {
	ID                string
	IGDBId            sql.NullString
	RAWGId            sql.NullString
	Title             string
	SortTitle         string
	Developer         sql.NullString
	Publisher         sql.NullString
	Description       sql.NullString
	ShortDescription  sql.NullString
	ReleaseDate       sql.NullString
	EnrichmentStatus  string
	LastEnriched      sql.NullString
	IsCompleteEdition bool
	ParentID          sql.NullString
	ArtworkRaw        sql.NullString
	Windows           bool
	Mac               bool
	Linux             bool
	SteamDeckVerified sql.NullString
	ProtonRating      sql.NullString
	IsInstalled       bool
	PlayStatus        sql.NullString
	Rating            sql.NullInt64
	ShortReview       sql.NullString
	Notes             sql.NullString
	IsFavorite        bool
	IsHidden          bool
	PlayTimeMinutes   sql.NullInt64
	LastPlayed        sql.NullString
	DateAdded         string
	GenresRaw         sql.NullString
	TagsRaw           sql.NullString
}

func (g GameDetail) Artwork() Artwork  { return ParseArtwork(g.ArtworkRaw) }
func (g GameDetail) Genres() []string  { return splitCSV(g.GenresRaw) }
func (g GameDetail) Tags() []string    { return splitCSV(g.TagsRaw) }

// ── Game store link ──────────────────────────────────────────────────────────

type GameStore struct {
	GameID     string
	Store      string
	StoreID    sql.NullString
	StoreURL   sql.NullString
	Owned      bool
	OwnedSince sql.NullTime
}

// ── Install source ───────────────────────────────────────────────────────────

// InstallInput is one game's install record as parsed by the browser client.
type InstallInput struct {
	Store      string `json:"store"`
	StoreID    string `json:"store_id"`
	InstallDir string `json:"install_dir"`
	SizeBytes  int64  `json:"size_bytes"`
}

type InstallSource struct {
	GameID            string
	DeviceID          string
	VolumeID          sql.NullString
	InstallPath       sql.NullString
	InstallSizeBytes  sql.NullInt64
	Runner            sql.NullString
	LastSeen          time.Time
	VolumeLabel       sql.NullString
	VolumePath        sql.NullString
}

// ── Game content (bundled DLC) ───────────────────────────────────────────────

type GameContent struct {
	GameID           string
	ContentType      string
	Title            string
	StoreID          sql.NullString
	IsInstalled      bool
	InstallationType string
	SortOrder        int
}

// ── Wishlist entry ───────────────────────────────────────────────────────────

type WishlistRow struct {
	ID                      string
	LibraryID               sql.NullString
	Title                   string
	SortTitle               string
	EnrichmentStatus        string
	ArtworkRaw              sql.NullString
	SteamDeckVerifiedRaw    sql.NullString
	Currency                string
	BestCurrentPrice        sql.NullFloat64
	BestCurrentStore        sql.NullString
	HistoricalLowPrice      sql.NullFloat64
	HistoricalLowStore      sql.NullString
	TargetPrice             sql.NullFloat64
	Priority                int
	DateAdded               string
	TagsRaw                 sql.NullString
	StoresRaw               sql.NullString
	FlagRemove              bool
	IsDuplicate             bool // only populated by ListWishlist
}

func (w WishlistRow) Artwork() Artwork        { return ParseArtwork(w.ArtworkRaw) }
func (w WishlistRow) Stores() []string        { return splitCSV(w.StoresRaw) }
func (w WishlistRow) Tags() []string          { return splitCSV(w.TagsRaw) }
func (w WishlistRow) SteamDeckVerified() string { return w.SteamDeckVerifiedRaw.String }

// ── Wishlist detail ───────────────────────────────────────────────────────────

type WishlistDetailRow struct {
	ID                   string
	LibraryID            sql.NullString
	IGDBId               sql.NullInt64
	Title                string
	SortTitle            string
	EnrichmentStatus     string
	ArtworkRaw           sql.NullString
	SteamDeckVerifiedRaw sql.NullString
	Currency             string
	BestCurrentPrice     sql.NullFloat64
	BestCurrentStore     sql.NullString
	BestPriceURL         sql.NullString
	HistoricalLowPrice   sql.NullFloat64
	HistoricalLowStore   sql.NullString
	LastPriceSync        sql.NullString
	TargetPrice          sql.NullFloat64
	PreferredStore       sql.NullString
	Priority             int
	Notes                sql.NullString
	DateAdded            string
	TagsRaw              sql.NullString
}

func (w WishlistDetailRow) Artwork() Artwork          { return ParseArtwork(w.ArtworkRaw) }
func (w WishlistDetailRow) Tags() []string            { return splitCSV(w.TagsRaw) }
func (w WishlistDetailRow) SteamDeckVerified() string { return w.SteamDeckVerifiedRaw.String }

// ── Wishlist store link ───────────────────────────────────────────────────────

type WishlistStoreLink struct {
	Store    string
	StoreID  sql.NullString
	StoreURL sql.NullString
}

// ── Sync log ─────────────────────────────────────────────────────────────────

// SyncError is one per-item error line captured during a sync run.
type SyncError struct {
	ID        int64
	SyncType  string
	RunID     string // ISO timestamp of the run, groups errors from the same batch
	Message   string
	CreatedAt string
}

type SyncLog struct {
	ID           int64
	Type         string
	Status       string
	StartedAt    string
	FinishedAt   sql.NullString
	GamesAdded   sql.NullInt64
	GamesUpdated sql.NullInt64
	ErrorMessage sql.NullString
}

// ── Status count ─────────────────────────────────────────────────────────────

type StatusCount struct {
	Value string
	Label string
	Count int
}

// ── Genre count ──────────────────────────────────────────────────────────────

type GenreCount struct {
	Genre string
	Count int
}

// ── Enrichment queue count ───────────────────────────────────────────────────

type QueueCounts struct {
	Pending int
	Running int
	Failed  int
}

// ── Dashboard stats ───────────────────────────────────────────────────────────

type DashboardStats struct {
	LibraryCount   int
	WishlistCount  int
	InstalledCount int
	UnmatchedCount int
}

// ── Price history ─────────────────────────────────────────────────────────────

type PriceHistoryRow struct {
	Price      float64
	Store      string
	RecordedAt time.Time
}

// ── Export ───────────────────────────────────────────────────────────────────

// ExportRow is returned by ExportGames for the library export endpoints.
type ExportRow struct {
	Title             string
	Developer         string
	Publisher         string
	ReleaseYear       string
	Windows           bool
	Mac               bool
	Linux             bool
	SteamDeckVerified string
	ProtonRating      string
	Genres            []string
	OwnedStores       []string
	PlayStatus        string
	Rating            int
	IsInstalled       bool
	EnrichmentStatus  string
}

// ── helpers ──────────────────────────────────────────────────────────────────

func splitCSV(s sql.NullString) []string {
	if !s.Valid || s.String == "" {
		return nil
	}
	return strings.Split(s.String, ",")
}
