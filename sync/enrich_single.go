package sync

import (
	"fmt"
	"strconv"
	"strings"

	"nisaba/db"
)

// ApplySingleMatch links one game to an IGDB entry and enriches it, in one call.
//
// This replaces the old `SetIGDBMatch` + `EnqueueEnrichment` pair, which was
// broken in two ways: it marked the game `manual`, which *removed* it from the
// pool `EnrichLibrary` selects from, and it enqueued into `enrichment_queue` — a
// table nothing anywhere drained. The result was a game that was linked and never
// enriched, with no indication anything had gone wrong.
//
// The write goes through enrichFromIGDB, the same function the enrichment
// pipeline and the review true-up use, so a hand-picked match is written exactly
// like a searched one.
func ApplySingleMatch(store *db.Store, client *IGDBClient, gameID, igdbID string) error {
	id, err := strconv.ParseInt(strings.TrimSpace(igdbID), 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("invalid igdb id %q", igdbID)
	}
	entry, err := client.FetchGame(id)
	if err != nil {
		return fmt.Errorf("fetch igdb %d: %w", id, err)
	}
	return enrichFromIGDB(store, gameID, *entry)
}

// RehydrateGame re-fetches metadata for one game and writes it.
//
// A game that already has an IGDB id is refreshed from that entry directly. One
// without an id is searched by title first, and only an **exact** normalised
// match is accepted (bestMatch), so a rehydrate cannot quietly attach a different
// game. Returns the outcome for the UI to report.
func RehydrateGame(store *db.Store, client *IGDBClient, gameID string) (string, error) {
	title, igdbID, err := store.GameEnrichTarget(gameID)
	if err != nil {
		return "", fmt.Errorf("load game: %w", err)
	}
	if title == "" && igdbID == "" {
		return "", fmt.Errorf("no such game %s", gameID)
	}

	if igdbID != "" {
		id, err := strconv.ParseInt(igdbID, 10, 64)
		if err == nil && id > 0 {
			entry, err := client.FetchGame(id)
			if err != nil {
				return "", fmt.Errorf("fetch igdb %d: %w", id, err)
			}
			if err := enrichFromIGDB(store, gameID, *entry); err != nil {
				return "", err
			}
			return fmt.Sprintf("Rehydrated from IGDB #%d.", id), nil
		}
	}

	results, err := client.SearchGame(title)
	if err != nil {
		return "", fmt.Errorf("search %q: %w", title, err)
	}
	match := bestMatch(title, results)
	if match == nil {
		// Not an error: IGDB has no entry whose name matches this title exactly,
		// which is a real outcome for store-decorated or obscure titles.
		return fmt.Sprintf("IGDB has no exact match for %q.", title), nil
	}
	if err := enrichFromIGDB(store, gameID, *match); err != nil {
		return "", err
	}
	return fmt.Sprintf("Matched to %q (IGDB #%d).", match.Name, match.ID), nil
}
