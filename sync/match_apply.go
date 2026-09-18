package sync

import (
	"fmt"
	"log"
	"time"

	"nisaba/db"
)

// ApplyProgress reports a true-up run. Total counts the verdicts being applied;
// Done advances once per verdict so the page can show a progress bar.
type ApplyProgress struct {
	Total    int
	Done     int
	Phase    string
	Applied  int // yes: linked to IGDB and re-enriched
	Rematch  int // no: the rejected entry was replaced with a different one
	Stranded int // no: rejected, and no other candidate existed to offer
	Errors   int
	Message  string
}

// ApplyMatchVerdicts turns the verdicts saved on the match review page into
// changes to the library. This is the only path in the review flow that writes to
// `games`, and it only ever runs when the owner asks for it.
//
// A **yes** links the game to the candidate it was shown and re-enriches it, using
// the same write the enrichment pipeline uses (enrichFromIGDB), so nothing is
// applied that the pipeline would not have written itself.
//
// A **no** means "this is the wrong entry, look again": the rejected IGDB id is
// recorded so it can never be offered again, the game is searched afresh, and the
// best *other* result becomes its new candidate. The verdict is then cleared, so
// the row returns to the undecided queue with something new to look at — or, when
// nothing else was found, with no candidate and the manual search box waiting.
// A no therefore never writes to `games`.
func ApplyMatchVerdicts(store *db.Store, client *IGDBClient, progressFn func(ApplyProgress)) (ApplyProgress, error) {
	yes, err := store.ListMatchReview(db.MatchFilterYes, 0, 10000)
	if err != nil {
		return ApplyProgress{}, fmt.Errorf("list yes: %w", err)
	}
	no, err := store.ListMatchReview(db.MatchFilterNo, 0, 10000)
	if err != nil {
		return ApplyProgress{}, fmt.Errorf("list no: %w", err)
	}

	p := ApplyProgress{Total: len(yes) + len(no)}
	if p.Total == 0 {
		p.Message = "No verdicts to apply."
		return p, nil
	}

	ticker := time.NewTicker(250 * time.Millisecond) // 4 req/s, as elsewhere
	defer ticker.Stop()
	report := func() {
		if progressFn != nil {
			progressFn(p)
		}
	}

	// ── Yes: link and re-enrich ──────────────────────────────────────
	p.Phase = "Linking and re-enriching accepted matches"
	for _, row := range yes {
		<-ticker.C
		if row.IGDBID == 0 {
			// A yes with nothing to link to — a manual pick always stores its id,
			// so this is defensive rather than expected.
			p.Errors++
			p.Done++
			report()
			continue
		}
		game, err := client.FetchGame(row.IGDBID)
		if err != nil {
			log.Printf("apply verdicts: igdb %d for %q: %v", row.IGDBID, row.Title, err)
			p.Errors++
			p.Done++
			report()
			continue
		}
		if err := enrichFromIGDB(store, row.ID, *game); err != nil {
			log.Printf("apply verdicts: enrich %s: %v", row.ID, err)
			p.Errors++
			p.Done++
			report()
			continue
		}
		p.Applied++
		p.Done++
		report()
	}

	// ── No: reject the entry and look for a different one ────────────
	p.Phase = "Re-matching rejected candidates"
	known, err := store.MatchedIGDBIDs()
	if err != nil {
		return p, fmt.Errorf("matched ids: %w", err)
	}
	rejected, err := store.RejectedIGDBIDs()
	if err != nil {
		return p, fmt.Errorf("rejected ids: %w", err)
	}

	for _, row := range no {
		<-ticker.C

		// Record the rejection first, so even a failed search cannot offer the
		// same wrong entry again.
		if row.IGDBID != 0 {
			if err := store.RecordMatchRejection(row.ID, row.IGDBID); err != nil {
				log.Printf("apply verdicts: record rejection %s: %v", row.ID, err)
				p.Errors++
				p.Done++
				report()
				continue
			}
			if rejected[row.ID] == nil {
				rejected[row.ID] = map[int64]bool{}
			}
			rejected[row.ID][row.IGDBID] = true
		}

		results, err := client.SearchGame(row.Title)
		candidate, score := db.MatchCandidate{GameID: row.ID, Confidence: "none"}, 0.0
		if err != nil {
			log.Printf("apply verdicts: search %q: %v", row.Title, err)
			p.Errors++
		} else {
			candidate, score = bestCandidate(row.Title, row.ID, results, known, rejected[row.ID])
		}

		// The verdict is cleared before the new candidate is written, because
		// UpsertMatchCandidate deliberately refuses to touch a decided row.
		if _, err := store.SaveMatchDecisions(map[string]*int{row.ID: nil}); err != nil {
			log.Printf("apply verdicts: clear verdict %s: %v", row.ID, err)
			p.Errors++
			p.Done++
			report()
			continue
		}
		if err := store.UpsertMatchCandidate(candidate); err != nil {
			log.Printf("apply verdicts: store candidate %s: %v", row.ID, err)
			p.Errors++
			p.Done++
			report()
			continue
		}

		if score > 0 {
			p.Rematch++
			known[candidate.IGDBID] = true
		} else {
			p.Stranded++
		}
		p.Done++
		report()
	}

	p.Phase = ""
	p.Message = fmt.Sprintf(
		"%d linked and re-enriched; %d rejected and re-matched, %d with nothing else found; %d errors.",
		p.Applied, p.Rematch, p.Stranded, p.Errors)
	return p, nil
}
