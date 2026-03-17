// Package sync contains store sync and import logic for NISABA.
package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"nisaba/db"
)

// ImportResult summarizes one store import run.
type ImportResult struct {
	Store   string
	Added   int
	Skipped int
	Errors  []string
}

// ImportHeroicLibraries reads all three Heroic JSON files from dir and upserts
// them into the database. Returns a combined result.
func ImportHeroicLibraries(store *db.Store, dir string) ([]ImportResult, error) {
	var results []ImportResult

	epic, err := importEpic(store, filepath.Join(dir, "legendary_library.json"))
	if err != nil {
		return nil, fmt.Errorf("epic: %w", err)
	}
	results = append(results, epic)

	gog, err := importGOG(store, filepath.Join(dir, "gog_library.json"))
	if err != nil {
		return nil, fmt.Errorf("gog: %w", err)
	}
	results = append(results, gog)

	amazon, err := importAmazon(store, filepath.Join(dir, "nile_library.json"))
	if err != nil {
		return nil, fmt.Errorf("amazon: %w", err)
	}
	results = append(results, amazon)

	return results, nil
}

// ── Epic (Legendary) ─────────────────────────────────────────────────────────

type epicLibrary struct {
	Library []epicGame `json:"library"`
}

type epicGame struct {
	AppName       string      `json:"app_name"`
	Title         string      `json:"title"`
	Developer     string      `json:"developer"`
	ArtCover      string      `json:"art_cover"`
	ArtSquare     string      `json:"art_square"`
	StoreURL      string      `json:"store_url"`
	Namespace     string      `json:"namespace"`
	IsInstalled   bool        `json:"is_installed"`
	IsMacNative   bool        `json:"is_mac_native"`
	IsLinuxNative bool        `json:"is_linux_native"`
	Install       epicInstall `json:"install"`
	DLCList       []epicDLC   `json:"dlcList"`
	Extra         epicExtra   `json:"extra"`
}

type epicInstall struct {
	IsDLC bool `json:"is_dlc"`
}

type epicDLC struct {
	Title          string `json:"title"`
	ID             string `json:"id"`
	EntitlementName string `json:"entitlementName"`
}

type epicExtra struct {
	About    epicAbout `json:"about"`
	StoreURL string    `json:"storeUrl"`
}

type epicAbout struct {
	Description      string `json:"description"`
	ShortDescription string `json:"shortDescription"`
}

func importEpic(store *db.Store, path string) (ImportResult, error) {
	result := ImportResult{Store: "epic"}

	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	var lib epicLibrary
	if err := json.Unmarshal(data, &lib); err != nil {
		return result, err
	}

	for _, g := range lib.Library {
		if g.Install.IsDLC || g.Title == "" || g.AppName == "" {
			result.Skipped++
			continue
		}

		storeURL := g.StoreURL
		if storeURL == "" {
			storeURL = g.Extra.StoreURL
		}

		gameID, err := store.FindGameByStoreID("epic", g.AppName)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", g.Title, err))
			continue
		}

		artwork := buildArtwork(g.ArtCover, g.ArtSquare, "", "", "", "epic")
		artJSON, _ := json.Marshal(artwork)

		var desc, shortDesc *string
		// Epic's `extra.about.description` sometimes just contains the title; skip those.
		if d := g.Extra.About.Description; d != "" && d != g.Title {
			desc = &d
		}
		if s := g.Extra.About.ShortDescription; s != "" {
			shortDesc = &s
		}
		dev := nullableString(g.Developer)

		if gameID == "" {
			gameID = uuid.New().String()
			if err := store.InsertGame(db.InsertGameParams{
				ID:               gameID,
				Title:            g.Title,
				SortTitle:        makeSortTitle(g.Title),
				Developer:        dev,
				Description:      desc,
				ShortDescription: shortDesc,
				ArtworkJSON:      string(artJSON),
				Windows:          true,
				Mac:              g.IsMacNative,
				Linux:            g.IsLinuxNative,
			}); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: insert: %v", g.Title, err))
				continue
			}
			result.Added++
		}
		// Always upsert the store link (handles re-imports cleanly).
		if err := store.UpsertGameStoreLink(gameID, "epic", g.AppName, storeURL); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: store link: %v", g.Title, err))
		}

		// Store bundled DLC as game_contents.
		for _, dlc := range g.DLCList {
			if dlc.Title == "" {
				continue
			}
			dlcID := dlc.EntitlementName
			if dlcID == "" {
				dlcID = dlc.ID
			}
			_ = store.UpsertContent(gameID, "dlc", dlc.Title, dlcID)
		}
	}

	return result, nil
}

// ── GOG ──────────────────────────────────────────────────────────────────────

type gogLibrary struct {
	Games []gogGame `json:"games"`
}

type gogGame struct {
	AppName       string    `json:"app_name"`
	Title         string    `json:"title"`
	Developer     string    `json:"developer"`
	ArtCover      string    `json:"art_cover"`
	ArtSquare     string    `json:"art_square"`
	ArtBackground string    `json:"art_background"`
	ArtIcon       string    `json:"art_icon"`
	IsInstalled   bool      `json:"is_installed"`
	IsMacNative   bool      `json:"is_mac_native"`
	IsLinuxNative bool      `json:"is_linux_native"`
	Install       gogInstall `json:"install"`
	Extra         gogExtra  `json:"extra"`
}

type gogInstall struct {
	IsDLC bool `json:"is_dlc"`
}

type gogExtra struct {
	About  gogAbout `json:"about"`
	Genres []string `json:"genres"`
}

type gogAbout struct {
	Description      string `json:"description"`
	ShortDescription string `json:"shortDescription"`
}

func importGOG(store *db.Store, path string) (ImportResult, error) {
	result := ImportResult{Store: "gog"}

	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	var lib gogLibrary
	if err := json.Unmarshal(data, &lib); err != nil {
		return result, err
	}

	for _, g := range lib.Games {
		if g.Install.IsDLC || g.Title == "" || g.AppName == "" {
			result.Skipped++
			continue
		}

		gameID, err := store.FindGameByStoreID("gog", g.AppName)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", g.Title, err))
			continue
		}

		artwork := buildArtwork(g.ArtCover, g.ArtSquare, g.ArtBackground, "", g.ArtIcon, "gog")
		artJSON, _ := json.Marshal(artwork)

		var desc, shortDesc *string
		if d := g.Extra.About.Description; d != "" {
			desc = &d
		}
		if s := g.Extra.About.ShortDescription; s != "" {
			shortDesc = &s
		}
		dev := nullableString(g.Developer)

		if gameID == "" {
			gameID = uuid.New().String()
			if err := store.InsertGame(db.InsertGameParams{
				ID:               gameID,
				Title:            g.Title,
				SortTitle:        makeSortTitle(g.Title),
				Developer:        dev,
				Description:      desc,
				ShortDescription: shortDesc,
				ArtworkJSON:      string(artJSON),
				Windows:          true,
				Mac:              g.IsMacNative,
				Linux:            g.IsLinuxNative,
			}); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: insert: %v", g.Title, err))
				continue
			}
			result.Added++
		}

		if err := store.UpsertGameStoreLink(gameID, "gog", g.AppName, ""); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: store link: %v", g.Title, err))
		}

		for _, genre := range g.Extra.Genres {
			if genre != "" {
				_ = store.UpsertGenre(gameID, genre)
			}
		}
	}

	return result, nil
}

// ── Amazon (Nile) ─────────────────────────────────────────────────────────────

type nileLibrary struct {
	Library []nileGame `json:"library"`
}

type nileGame struct {
	AppName       string    `json:"app_name"`
	Title         string    `json:"title"`
	Developer     string    `json:"developer"`
	ArtCover      string    `json:"art_cover"`
	ArtSquare     string    `json:"art_square"`
	ArtBackground string    `json:"art_background"`
	ArtLogo       string    `json:"art_logo"`
	Description   string    `json:"description"`
	IsInstalled   bool      `json:"is_installed"`
	IsMacNative   bool      `json:"is_mac_native"`
	IsLinuxNative bool      `json:"is_linux_native"`
	Extra         nileExtra `json:"extra"`
}

type nileExtra struct {
	Genres      []string `json:"genres"`
	ReleaseDate string   `json:"releaseDate"`
}

func importAmazon(store *db.Store, path string) (ImportResult, error) {
	result := ImportResult{Store: "amazon"}

	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	var lib nileLibrary
	if err := json.Unmarshal(data, &lib); err != nil {
		return result, err
	}

	for _, g := range lib.Library {
		if g.Title == "" || g.AppName == "" {
			result.Skipped++
			continue
		}

		gameID, err := store.FindGameByStoreID("amazon", g.AppName)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", g.Title, err))
			continue
		}

		artwork := buildArtwork(g.ArtCover, g.ArtSquare, g.ArtBackground, g.ArtLogo, "", "amazon")
		artJSON, _ := json.Marshal(artwork)

		// Parse release date to just the date portion.
		releaseDate := parseDate(g.Extra.ReleaseDate)

		var desc *string
		if g.Description != "" {
			desc = &g.Description
		}
		dev := nullableString(g.Developer)

		if gameID == "" {
			gameID = uuid.New().String()
			if err := store.InsertGame(db.InsertGameParams{
				ID:          gameID,
				Title:       g.Title,
				SortTitle:   makeSortTitle(g.Title),
				Developer:   dev,
				Description: desc,
				ReleaseDate: releaseDate,
				ArtworkJSON: string(artJSON),
				Windows:     true,
				Mac:         g.IsMacNative,
				Linux:       g.IsLinuxNative,
			}); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: insert: %v", g.Title, err))
				continue
			}
			result.Added++
		}

		if err := store.UpsertGameStoreLink(gameID, "amazon", g.AppName, ""); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: store link: %v", g.Title, err))
		}

		for _, genre := range g.Extra.Genres {
			if genre != "" {
				_ = store.UpsertGenre(gameID, genre)
			}
		}
	}

	return result, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

type artworkJSON struct {
	Cover      artRef `json:"cover"`
	Square     artRef `json:"square"`
	Background artRef `json:"background"`
	Logo       artRef `json:"logo"`
	Icon       artRef `json:"icon"`
}

type artRef struct {
	URL    string `json:"url"`
	Source string `json:"source"`
}

func buildArtwork(cover, square, background, logo, icon, source string) artworkJSON {
	ref := func(url string) artRef {
		if url == "" {
			return artRef{}
		}
		return artRef{URL: url, Source: source}
	}
	return artworkJSON{
		Cover:      ref(cover),
		Square:     ref(square),
		Background: ref(background),
		Logo:       ref(logo),
		Icon:       ref(icon),
	}
}

// makeSortTitle strips leading articles for alphabetical sorting.
func makeSortTitle(title string) string {
	lower := strings.ToLower(title)
	for _, prefix := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(title[len(prefix):])
		}
	}
	return title
}

// nullableString returns nil if s is empty, otherwise &s.
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// parseDate trims an ISO 8601 datetime to just the date portion.
func parseDate(s string) *string {
	if s == "" {
		return nil
	}
	if len(s) >= 10 {
		d := s[:10]
		return &d
	}
	return &s
}
