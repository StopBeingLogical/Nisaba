package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"nisaba/db"
	storesync "nisaba/sync"
)

// matchPageSize is how many game/candidate pairs one page shows.
const matchPageSize = 25

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

// MatchReviewStatus is polled by the review page while a candidate search runs.
func (h *Handler) MatchReviewStatus(w http.ResponseWriter, r *http.Request) {
	h.renderPartial(w, "sync_status_partial.html", h.matchFindStatus())
}

// matchFindStatus snapshots the candidate-search job in the shape the status
// partial expects.
func (h *Handler) matchFindStatus() syncStatusData {
	h.matchFind.mu.Lock()
	defer h.matchFind.mu.Unlock()

	d := syncStatusData{
		Running:     h.matchFind.running,
		LastMessage: h.matchFind.lastMsg,
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
	default:
		return "no candidate"
	}
}
