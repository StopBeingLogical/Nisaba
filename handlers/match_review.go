package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"nisaba/db"
	storesync "nisaba/sync"
)

// matchPageSize is how many game/candidate pairs one page shows.
const matchPageSize = 25

// maxSearchHits bounds the list the inline search box renders. The search itself
// merges two sources, so it can return far more rows than a row's dropdown should
// ever show.
const maxSearchHits = 15

// igdbClient returns the shared IGDB client, building it from config on first
// use. Returns nil when the credentials are not configured.
func (h *Handler) igdbClient() *storesync.IGDBClient {
	h.igdbMu.Lock()
	defer h.igdbMu.Unlock()
	if h.igdb == nil {
		clientID, _ := h.store.GetConfig("igdb.client_id")
		clientSecret, _ := h.store.GetConfig("igdb.client_secret")
		if clientID == "" || clientSecret == "" {
			return nil
		}
		h.igdb = storesync.NewIGDBClient(clientID, clientSecret)
	}
	return h.igdb
}

// MatchReview renders the candidate-match review page: the game as it is listed
// today on the left, the best IGDB candidate on the right, and a yes/no pair per
// row so the owner can work through the queue over several sittings.
func (h *Handler) MatchReview(w http.ResponseWriter, r *http.Request) {
	base, err := h.baseData("match-review")
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	filter := db.MatchReviewFilter(r.URL.Query().Get("filter"))
	switch filter {
	case db.MatchFilterYes, db.MatchFilterNo, db.MatchFilterAll:
	default:
		filter = db.MatchFilterTodo
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	rows, err := h.store.ListMatchReview(filter, offset, matchPageSize)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	filtered, err := h.store.CountMatchReviewFilter(filter)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	counts, err := h.store.CountMatchReview()
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	base["Rows"] = rows
	base["Filter"] = string(filter)
	base["Offset"] = offset
	base["Filtered"] = filtered
	base["PrevOffset"] = offset - matchPageSize
	base["NextOffset"] = offset + matchPageSize
	base["HasPrev"] = offset > 0
	base["HasNext"] = offset+matchPageSize < filtered
	base["FinalOffset"] = ((filtered - 1) / matchPageSize) * matchPageSize
	base["Counts"] = counts
	base["FindStatus"] = h.matchFindStatus()

	if filtered > 0 {
		base["PageStart"] = offset + 1
		base["PageEnd"] = min(offset+matchPageSize, filtered)
	}

	if saved, err := strconv.Atoi(r.URL.Query().Get("saved")); err == nil && saved > 0 {
		base["Saved"] = saved
	}

	h.render(w, "match_review.html", base)
}

// MatchReviewSave persists the page's verdicts. Every row on the page is listed
// in game_ids, so a row whose radio was cleared writes NULL back and returns to
// the undecided pool rather than silently keeping its old value.
func (h *Handler) MatchReviewSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	decisions := make(map[string]*int, len(r.Form["game_ids"]))
	for _, id := range r.Form["game_ids"] {
		if id == "" {
			continue
		}
		switch r.FormValue("decision_" + id) {
		case "yes":
			one := 1
			decisions[id] = &one
		case "no":
			zero := 0
			decisions[id] = &zero
		default:
			decisions[id] = nil // cleared → undecided
		}
	}

	saved, err := h.store.SaveMatchDecisions(decisions)
	if err != nil {
		log.Printf("MatchReviewSave: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	filter := r.FormValue("filter")
	switch db.MatchReviewFilter(filter) {
	case db.MatchFilterYes, db.MatchFilterNo, db.MatchFilterAll:
	default:
		filter = string(db.MatchFilterTodo)
	}
	offset := r.FormValue("offset")
	if offset == "" {
		offset = "0"
	}

	http.Redirect(w, r,
		fmt.Sprintf("/match-review?filter=%s&offset=%s&saved=%d", filter, offset, saved),
		http.StatusSeeOther)
}

// MatchReviewFind searches IGDB for every undecided game and stores its best
// candidate. It runs in the background because a full pass is ~4 req/sec, and
// returns the status fragment so the page can poll it.
func (h *Handler) MatchReviewFind(w http.ResponseWriter, r *http.Request) {
	h.matchFind.mu.Lock()
	if h.matchFind.running {
		h.matchFind.mu.Unlock()
		h.renderPartial(w, "sync_status_partial.html", syncStatusData{
			Running: true, Message: "Candidate search already running…",
			PollURL: "/match-review/status",
		})
		return
	}
	h.matchFind.running = true
	h.matchFind.done = 0
	h.matchFind.total = 0
	h.matchFind.lastMsg = ""
	h.matchFind.mu.Unlock()

	client := h.igdbClient()
	if client == nil {
		h.matchFind.mu.Lock()
		h.matchFind.running = false
		h.matchFind.lastMsg = "IGDB credentials are not configured."
		h.matchFind.mu.Unlock()
		h.renderPartial(w, "sync_status_partial.html", h.matchFindStatus())
		return
	}

	go func() {
		var last storesync.EnrichProgress
		err := storesync.FindMatchCandidates(h.store, client, func(p storesync.EnrichProgress) {
			last = p
			h.matchFind.mu.Lock()
			h.matchFind.done = p.Done
			h.matchFind.total = p.Total
			h.matchFind.mu.Unlock()
			if p.Done%50 == 0 && p.Done > 0 {
				log.Printf("match-review: candidate search %d / %d", p.Done, p.Total)
			}
		})

		h.matchFind.mu.Lock()
		h.matchFind.running = false
		if err != nil {
			h.matchFind.lastMsg = "Search failed: " + err.Error()
			log.Printf("match-review: candidate search: %v", err)
		} else {
			h.matchFind.lastMsg = fmt.Sprintf(
				"Searched %d games — %d with a candidate, %d errors.",
				last.Total, last.Matched, last.Errors)
			log.Printf("match-review: %s", h.matchFind.lastMsg)
		}
		h.matchFind.mu.Unlock()
	}()

	h.renderPartial(w, "sync_status_partial.html", h.matchFindStatus())
}

// matchSearchResult is one IGDB hit offered when the owner searches a title by
// hand. The pick carries only the id — the handler re-fetches the entry so the
// stored candidate has the same evidence as a searched one.
type matchSearchResult struct {
	GameID      string
	IGDBID      int64
	Name        string
	CoverURL    string
	ReleaseYear string
}

// MatchReviewSearch proxies an IGDB title search from a row on the review page,
// so a wrong or missing candidate can be replaced without leaving the queue.
func (h *Handler) MatchReviewSearch(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "id")
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		h.renderPartial(w, "match_search_results_partial.html", nil)
		return
	}

	client := h.igdbClient()
	if client == nil {
		http.Error(w, "IGDB credentials are not configured", http.StatusServiceUnavailable)
		return
	}

	games, err := client.SearchGame(q)
	if err != nil {
		log.Printf("MatchReviewSearch %s: %v", q, err)
		http.Error(w, "IGDB search failed", http.StatusBadGateway)
		return
	}
	// Closest name match first: the search merges a name-anchored lookup with
	// IGDB's own ranking, so IGDB's order is not the order to show.
	games = storesync.RankSearchResults(q, games)
	if len(games) > maxSearchHits {
		games = games[:maxSearchHits]
	}

	results := make([]matchSearchResult, 0, len(games))
	for _, g := range games {
		results = append(results, matchSearchResult{
			GameID:      gameID,
			IGDBID:      g.ID,
			Name:        g.Name,
			CoverURL:    g.CoverURL(),
			ReleaseYear: g.ReleaseYear(),
		})
	}
	h.renderPartial(w, "match_search_results_partial.html", results)
}

// MatchReviewSetCandidate stores an IGDB entry the owner picked by hand and marks
// the row yes, then returns the re-rendered row for an HTMX swap. Picking a match
// is the verdict itself, so this persists immediately rather than waiting for
// Save — the page's Save still controls the yes/no radios.
func (h *Handler) MatchReviewSetCandidate(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	igdbID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("igdb_id")), 10, 64)
	if err != nil || igdbID <= 0 {
		http.Error(w, "igdb_id required", http.StatusBadRequest)
		return
	}

	client := h.igdbClient()
	if client == nil {
		http.Error(w, "IGDB credentials are not configured", http.StatusServiceUnavailable)
		return
	}

	game, err := client.FetchGame(igdbID)
	if err != nil {
		log.Printf("MatchReviewSetCandidate fetch %d: %v", igdbID, err)
		http.Error(w, "IGDB lookup failed", http.StatusBadGateway)
		return
	}

	known, err := h.store.MatchedIGDBIDs()
	if err != nil {
		log.Printf("MatchReviewSetCandidate matched ids: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	candidate := storesync.CandidateFromGame(gameID, *game, "manual", 0, known[game.ID])
	if err := h.store.SetManualMatch(candidate); err != nil {
		log.Printf("MatchReviewSetCandidate save %s: %v", gameID, err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	row, err := h.store.GetMatchReviewRow(gameID)
	if err != nil {
		log.Printf("MatchReviewSetCandidate reload %s: %v", gameID, err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	h.renderPartial(w, "match_row_partial.html", row)
}

// MatchReviewStatus is polled by the review page while a candidate search runs.
func (h *Handler) MatchReviewStatus(w http.ResponseWriter, r *http.Request) {
	h.renderPartial(w, "sync_status_partial.html", h.matchFindStatus())
}

// MatchReviewApply runs the true-up: it turns the saved verdicts into changes to
// the library. This is the only review endpoint that writes to `games`, and it
// only runs when asked. Backgrounded because linking and re-enriching costs one
// IGDB lookup per accepted match.
func (h *Handler) MatchReviewApply(w http.ResponseWriter, r *http.Request) {
	h.matchApply.mu.Lock()
	if h.matchApply.running {
		h.matchApply.mu.Unlock()
		h.renderPartial(w, "sync_status_partial.html", syncStatusData{
			Running: true, Message: "Applying verdicts already running…",
			PollURL: "/match-review/apply/status",
		})
		return
	}
	h.matchApply.running = true
	h.matchApply.done = 0
	h.matchApply.total = 0
	h.matchApply.phase = ""
	h.matchApply.lastMsg = ""
	h.matchApply.mu.Unlock()

	// The two jobs share one status panel and both read the queue, so let one
	// finish before the other starts rather than interleaving their progress.
	h.matchFind.mu.Lock()
	finderRunning := h.matchFind.running
	h.matchFind.mu.Unlock()
	if finderRunning {
		h.matchApply.mu.Lock()
		h.matchApply.running = false
		h.matchApply.mu.Unlock()
		h.renderPartial(w, "sync_status_partial.html", syncStatusData{
			Running: false, Message: "A candidate search is running — let it finish first.",
			PollURL: "/match-review/status",
		})
		return
	}

	client := h.igdbClient()
	if client == nil {
		h.matchApply.mu.Lock()
		h.matchApply.running = false
		h.matchApply.lastMsg = "IGDB credentials are not configured."
		h.matchApply.mu.Unlock()
		h.renderPartial(w, "sync_status_partial.html", h.matchApplyStatus())
		return
	}

	go func() {
		result, err := storesync.ApplyMatchVerdicts(h.store, client, func(p storesync.ApplyProgress) {
			h.matchApply.mu.Lock()
			h.matchApply.done = p.Done
			h.matchApply.total = p.Total
			h.matchApply.phase = p.Phase
			h.matchApply.mu.Unlock()
		})

		h.matchApply.mu.Lock()
		h.matchApply.running = false
		h.matchApply.phase = ""
		if err != nil {
			h.matchApply.lastMsg = "Apply failed: " + err.Error()
			log.Printf("match-review: apply verdicts: %v", err)
		} else {
			h.matchApply.lastMsg = result.Message
			log.Printf("match-review: %s", result.Message)
		}
		h.matchApply.mu.Unlock()
	}()

	h.renderPartial(w, "sync_status_partial.html", h.matchApplyStatus())
}

// MatchReviewApplyStatus is polled while the true-up runs.
func (h *Handler) MatchReviewApplyStatus(w http.ResponseWriter, r *http.Request) {
	h.renderPartial(w, "sync_status_partial.html", h.matchApplyStatus())
}

// matchFindStatus snapshots the candidate-search job in the shape the status
// partial expects.
func (h *Handler) matchFindStatus() syncStatusData {
	h.matchFind.mu.Lock()
	defer h.matchFind.mu.Unlock()

	d := syncStatusData{
		Running:     h.matchFind.running,
		LastMessage: h.matchFind.lastMsg,
		PollURL:     "/match-review/status",
	}
	if h.matchFind.running {
		d.Message = "Searching IGDB for the best candidate…"
		d.Step = "Games searched"
		d.StepDone = h.matchFind.done
		d.StepTotal = h.matchFind.total
		if d.StepTotal > 0 {
			d.StepPct = d.StepDone * 100 / d.StepTotal
		}
	}
	return d
}

// matchApplyStatus snapshots the true-up job in the shape the status partial
// expects. It polls its own endpoint: the two jobs report into the same panel, so
// a shared URL would make one job's panel show the other's progress.
func (h *Handler) matchApplyStatus() syncStatusData {
	h.matchApply.mu.Lock()
	defer h.matchApply.mu.Unlock()

	d := syncStatusData{
		Running:     h.matchApply.running,
		LastMessage: h.matchApply.lastMsg,
		PollURL:     "/match-review/apply/status",
	}
	if h.matchApply.running {
		d.Message = "Applying your verdicts…"
		d.Step = h.matchApply.phase
		d.StepDone = h.matchApply.done
		d.StepTotal = h.matchApply.total
		if d.StepTotal > 0 {
			d.StepPct = d.StepDone * 100 / d.StepTotal
		}
	}
	return d
}

// matchConfidenceLabel turns a stored confidence key into a UI label.
func matchConfidenceLabel(confidence string) string {
	switch strings.TrimSpace(confidence) {
	case "exact":
		return "exact title"
	case "prefix":
		return "title is a prefix"
	case "contains":
		return "title contained"
	case "tokens":
		return "word overlap"
	case "weak":
		return "weak match"
	case "manual":
		return "you picked this"
	default:
		return "no candidate"
	}
}
