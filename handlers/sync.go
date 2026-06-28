package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	storesync "nisaba/sync"
	"nisaba/db"
)

// syncStatusData is what the sync_status_partial.html template expects.
type syncStatusData struct {
	Running      bool
	Message      string
	Detail       string
	// Step-level progress (shown while running)
	Step         string
	StepDone     int
	StepTotal    int
	StepPct      int // 0-100 for progress bar width
	// Legacy free-text progress line (enrichment uses this)
	Progress     string
	LastMessage  string
	LastFinished *time.Time
	Errors       []string // per-item errors from the last completed run
}

// SyncPanel renders the sync dashboard.
func (h *Handler) SyncPanel(w http.ResponseWriter, r *http.Request) {
	base, err := h.baseData("sync")
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	recent, _ := h.store.RecentSyncs(20)

	lastOwnership, _ := h.store.LatestSyncByType("ownership")
	lastInstall, _ := h.store.LatestSyncByType("install")
	lastPricing, _ := h.store.LatestSyncByType("pricing")
	lastWishlist, _ := h.store.LatestSyncByType("wishlist")

	needsReview, _ := h.store.CountNeedsReview()
	base["NeedsReview"] = needsReview
	base["RecentSyncs"] = recent
	base["SyncStatus"] = h.currentSyncStatus()
	base["CurrentDevice"] = "this device" // TODO: detect from UA

	if lastOwnership != nil && lastOwnership.FinishedAt.Valid {
		base["LastOwnershipSync"] = formatTime(lastOwnership.FinishedAt.String)
	}
	if lastInstall != nil && lastInstall.FinishedAt.Valid {
		base["LastInstallSync"] = formatTime(lastInstall.FinishedAt.String)
	}
	if lastPricing != nil && lastPricing.FinishedAt.Valid {
		base["LastPricingSync"] = formatTime(lastPricing.FinishedAt.String)
	}
	if lastWishlist != nil && lastWishlist.FinishedAt.Valid {
		base["LastWishlistSync"] = formatTime(lastWishlist.FinishedAt.String)
	}

	h.render(w, "sync.html", base)
}

// SyncAll runs ownership → wishlist → pricing → enrichment in sequence.
func (h *Handler) SyncAll(w http.ResponseWriter, r *http.Request) {
	h.syncAll.mu.Lock()
	already := h.syncAll.running
	h.syncAll.mu.Unlock()
	if already {
		status := syncStatusData{Running: true, Message: "Full sync already running…"}
		h.renderPartial(w, "sync_status_partial.html", status)
		return
	}

	h.syncAll.mu.Lock()
	h.syncAll.running = true
	h.syncAll.step = "Starting…"
	h.syncAll.lastMsg = ""
	h.syncAll.mu.Unlock()

	logID, _ := h.store.StartSync("full")

	go func() {
		setStep := func(s string) {
			h.syncAll.mu.Lock()
			h.syncAll.step = s
			h.syncAll.mu.Unlock()
		}
		finish := func(msg string) {
			h.syncAll.mu.Lock()
			h.syncAll.running = false
			h.syncAll.lastMsg = msg
			h.syncAll.mu.Unlock()
			log.Printf("sync all done: %s", msg)
		}
		finishLog := func(status, errMsg string, added, updated int) {
			if logID > 0 {
				_ = h.store.FinishSync(logID, status, added, updated, errMsg)
			}
		}

		// 1. Ownership
		log.Printf("sync-all: step — Steam ownership")
		setStep("Syncing Steam ownership…")
		ownerResult, err := storesync.SyncSteamOwnership(h.store)
		if err != nil {
			msg := "Full sync failed during ownership: " + err.Error()
			finish(msg)
			finishLog("failed", err.Error(), 0, 0)
			return
		}
		log.Printf("sync-all: ownership done — +%d games, %d errors", ownerResult.Added, len(ownerResult.Errors))
		for _, e := range ownerResult.Errors {
			log.Printf("sync-all ownership: %s", e)
		}

		// 2. Wishlist (Steam + GOG)
		log.Printf("sync-all: step — Steam wishlist")
		setStep("Syncing Steam wishlist…")
		wlResult, err := storesync.SyncSteamWishlist(h.store)
		if err != nil {
			msg := "Full sync failed during wishlist: " + err.Error()
			finish(msg)
			finishLog("failed", err.Error(), ownerResult.Added, 0)
			return
		}
		log.Printf("sync-all: Steam wishlist done — %d entries, %d errors", wlResult.Added, len(wlResult.Errors))
		for _, e := range wlResult.Errors {
			log.Printf("sync-all wishlist: %s", e)
		}

		// GOG wishlist — skip gracefully if token not configured
		log.Printf("sync-all: step — GOG wishlist")
		setStep("Syncing GOG wishlist…")
		gogResult, gogErr := storesync.SyncGOGWishlist(h.store)
		if gogErr != nil {
			log.Printf("sync-all: GOG wishlist skipped: %v", gogErr)
		} else {
			log.Printf("sync-all: GOG wishlist done — %d entries, %d errors", gogResult.Added, len(gogResult.Errors))
			for _, e := range gogResult.Errors {
				log.Printf("sync-all gog wishlist: %s", e)
			}
			wlResult.Added += gogResult.Added
		}

		if linked, _ := h.store.LinkWishlistToLibrary(); linked > 0 {
			log.Printf("sync-all: linked %d wishlist entries to library", linked)
		}

		// 3. Pricing
		log.Printf("sync-all: step — GG.deals pricing")
		setStep("Fetching GG.deals prices…")
		pResult, err := storesync.SyncGGDealsPricing(h.store, func(step string, done, total int) {
			setStep(fmt.Sprintf("Pricing — %s (%d / %d)", step, done, total))
		})
		if err != nil {
			msg := "Full sync failed during pricing: " + err.Error()
			finish(msg)
			finishLog("failed", err.Error(), ownerResult.Added+wlResult.Added, 0)
			return
		}
		log.Printf("sync-all: GG.deals done — %d updated, %d not found, %d errors", pResult.Updated, pResult.NotFound, len(pResult.Errors))
		for _, e := range pResult.Errors {
			log.Printf("sync-all pricing: %s", e)
		}

		log.Printf("sync-all: step — reseller pricing")
		setStep("Scraping reseller prices for non-Steam entries…")
		rResult, _ := storesync.SyncResellerPricing(h.store, func(step string, done, total int) {
			setStep(fmt.Sprintf("Pricing — %s (%d / %d)", step, done, total))
		})
		log.Printf("sync-all: resellers done — %d updated, %d not found, %d errors", rResult.Updated, rResult.NotFound, len(rResult.Errors))
		for _, e := range rResult.Errors {
			log.Printf("sync-all reseller: %s", e)
		}

		// 4. Enrichment (only if IGDB is configured)
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

		enrichNote := ""
		if client != nil {
			log.Printf("sync-all: step — IGDB enrichment")
			setStep("Running IGDB enrichment…")
			h.enrichment.mu.Lock()
			h.enrichment.running = true
			h.enrichment.prog = storesync.EnrichProgress{}
			h.enrichment.mu.Unlock()

			err = storesync.EnrichLibrary(h.store, client, func(p storesync.EnrichProgress) {
				h.enrichment.mu.Lock()
				h.enrichment.prog = p
				h.enrichment.mu.Unlock()
				if p.Done%100 == 0 && p.Done > 0 {
					log.Printf("sync-all: enrichment progress — %d / %d", p.Done, p.Total)
				}
			}, h.rawgClient())

			h.enrichment.mu.Lock()
			h.enrichment.running = false
			p := h.enrichment.prog
			h.enrichment.mu.Unlock()

			if err != nil {
				enrichNote = fmt.Sprintf(", enrichment failed: %v", err)
			} else {
				enrichNote = fmt.Sprintf(", enriched %d/%d", p.Matched, p.Total)
			}
		} else {
			enrichNote = ", enrichment skipped (IGDB not configured)"
		}

		summary := fmt.Sprintf(
			"Full sync complete — +%d owned, %d wishlist, %d prices updated%s",
			ownerResult.Added, wlResult.Added, pResult.Updated, enrichNote,
		)
		finish(summary)
		finishLog("done", "", ownerResult.Added+wlResult.Added, pResult.Updated)
	}()

	status := syncStatusData{
		Running: true,
		Message: "Full sync started…",
		Detail:  "Ownership → Wishlist → Pricing → Enrichment",
	}
	h.renderPartial(w, "sync_status_partial.html", status)
}

// SyncWishlistRefresh runs the full wishlist pipeline:
// Steam wishlist → GOG wishlist → Deck status → ProtonDB → Pricing → IGDB wishlist enrichment.
func (h *Handler) SyncWishlistRefresh(w http.ResponseWriter, r *http.Request) {
	h.syncAll.mu.Lock()
	already := h.syncAll.running
	h.syncAll.mu.Unlock()
	if already {
		status := syncStatusData{Running: true, Message: "A sync is already running…"}
		h.renderPartial(w, "sync_status_partial.html", status)
		return
	}

	h.syncAll.mu.Lock()
	h.syncAll.running = true
	h.syncAll.step = "Starting…"
	h.syncAll.lastMsg = ""
	h.syncAll.mu.Unlock()

	logID, _ := h.store.StartSync("wishlist-refresh")

	go func() {
		setStep := func(s string) {
			h.syncAll.mu.Lock()
			h.syncAll.step = s
			h.syncAll.mu.Unlock()
		}
		finish := func(msg string) {
			h.syncAll.mu.Lock()
			h.syncAll.running = false
			h.syncAll.lastMsg = msg
			h.syncAll.mu.Unlock()
			log.Printf("wishlist refresh done: %s", msg)
		}

		// 1. Steam wishlist
		log.Printf("wishlist-refresh: step — Steam wishlist")
		setStep("Syncing Steam wishlist…")
		wlResult, err := storesync.SyncSteamWishlist(h.store)
		if err != nil {
			msg := "Wishlist refresh failed during Steam wishlist: " + err.Error()
			finish(msg)
			if logID > 0 {
				_ = h.store.FinishSync(logID, "failed", 0, 0, err.Error())
			}
			return
		}
		for _, e := range wlResult.Errors {
			log.Printf("wishlist-refresh steam: %s", e)
		}

		// 2. GOG wishlist (skip gracefully if token not configured)
		log.Printf("wishlist-refresh: step — GOG wishlist")
		setStep("Syncing GOG wishlist…")
		gogResult, gogErr := storesync.SyncGOGWishlist(h.store)
		if gogErr != nil {
			log.Printf("wishlist-refresh: GOG wishlist skipped: %v", gogErr)
		} else {
			for _, e := range gogResult.Errors {
				log.Printf("wishlist-refresh gog: %s", e)
			}
			wlResult.Added += gogResult.Added
		}

		if linked, _ := h.store.LinkWishlistToLibrary(); linked > 0 {
			log.Printf("wishlist-refresh: linked %d entries to library", linked)
		}

		// 3. Steam Deck status
		log.Printf("wishlist-refresh: step — Steam Deck status")
		h.deck.mu.Lock()
		h.deck.running = true
		h.deck.done = 0
		h.deck.total = 0
		h.deck.mu.Unlock()
		deckUpdated, deckErr := storesync.SyncSteamDeckStatus(h.store, func(done, total int) {
			h.deck.mu.Lock()
			h.deck.done = done
			h.deck.total = total
			h.deck.mu.Unlock()
			setStep(fmt.Sprintf("Deck status — %d / %d", done, total))
		})
		h.deck.mu.Lock()
		h.deck.running = false
		h.deck.mu.Unlock()
		if deckErr != nil {
			log.Printf("wishlist-refresh: deck status failed: %v", deckErr)
		} else {
			log.Printf("wishlist-refresh: deck status done — %d updated", deckUpdated)
		}

		// 4. ProtonDB ratings
		log.Printf("wishlist-refresh: step — ProtonDB")
		h.proton.mu.Lock()
		h.proton.running = true
		h.proton.done = 0
		h.proton.total = 0
		h.proton.mu.Unlock()
		protonResult, protonErr := storesync.SyncProtonRatings(h.store, func(done, total int) {
			h.proton.mu.Lock()
			h.proton.done = done
			h.proton.total = total
			h.proton.mu.Unlock()
			setStep(fmt.Sprintf("ProtonDB — %d / %d", done, total))
		})
		h.proton.mu.Lock()
		h.proton.running = false
		h.proton.mu.Unlock()
		if protonErr != nil {
			log.Printf("wishlist-refresh: proton failed: %v", protonErr)
		} else {
			log.Printf("wishlist-refresh: proton done — %d updated", protonResult.Updated)
		}

		// 5. Pricing
		log.Printf("wishlist-refresh: step — GG.deals pricing")
		setStep("Fetching GG.deals prices…")
		pResult, pErr := storesync.SyncGGDealsPricing(h.store, func(step string, done, total int) {
			setStep(fmt.Sprintf("Pricing — %s (%d / %d)", step, done, total))
		})
		if pErr != nil {
			msg := "Wishlist refresh failed during pricing: " + pErr.Error()
			finish(msg)
			if logID > 0 {
				_ = h.store.FinishSync(logID, "failed", wlResult.Added, 0, pErr.Error())
			}
			return
		}
		for _, e := range pResult.Errors {
			log.Printf("wishlist-refresh pricing: %s", e)
		}

		log.Printf("wishlist-refresh: step — reseller pricing")
		setStep("Scraping reseller prices for non-Steam entries…")
		rResult, _ := storesync.SyncResellerPricing(h.store, func(step string, done, total int) {
			setStep(fmt.Sprintf("Pricing — %s (%d / %d)", step, done, total))
		})
		for _, e := range rResult.Errors {
			log.Printf("wishlist-refresh reseller: %s", e)
		}

		// 6. IGDB wishlist enrichment
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

		enrichNote := ""
		if client != nil {
			log.Printf("wishlist-refresh: step — IGDB wishlist enrichment")
			setStep("Running IGDB wishlist enrichment…")
			h.enrichment.mu.Lock()
			h.enrichment.running = true
			h.enrichment.prog = storesync.EnrichProgress{}
			h.enrichment.mu.Unlock()

			enrichErr := storesync.EnrichWishlist(h.store, client, func(p storesync.EnrichProgress) {
				h.enrichment.mu.Lock()
				h.enrichment.prog = p
				h.enrichment.mu.Unlock()
			}, h.rawgClient())

			if linked, _ := h.store.LinkWishlistToLibrary(); linked > 0 {
				log.Printf("wishlist-refresh: post-enrich linked %d entries", linked)
			}

			h.enrichment.mu.Lock()
			h.enrichment.running = false
			p := h.enrichment.prog
			h.enrichment.mu.Unlock()

			if enrichErr != nil {
				enrichNote = fmt.Sprintf(", enrichment failed: %v", enrichErr)
			} else {
				enrichNote = fmt.Sprintf(", enriched %d/%d", p.Matched, p.Total)
			}
		} else {
			enrichNote = ", enrichment skipped (IGDB not configured)"
		}

		summary := fmt.Sprintf(
			"Wishlist refresh complete — +%d wishlist entries, %d prices updated, %d deck updated, %d ProtonDB updated%s",
			wlResult.Added, pResult.Updated, deckUpdated, protonResult.Updated, enrichNote,
		)
		finish(summary)
		if logID > 0 {
			_ = h.store.FinishSync(logID, "done", wlResult.Added, pResult.Updated, "")
		}
	}()

	status := syncStatusData{
		Running: true,
		Message: "Wishlist refresh started…",
		Detail:  "Wishlist → Deck Status → ProtonDB → Pricing → Enrichment",
	}
	h.renderPartial(w, "sync_status_partial.html", status)
}

// SyncOwnership pulls owned games from all configured store APIs.
func (h *Handler) SyncOwnership(w http.ResponseWriter, r *http.Request) {
	logID, _ := h.store.StartSync("ownership")
	result, err := storesync.SyncSteamOwnership(h.store)
	if err != nil {
		if logID > 0 {
			_ = h.store.FinishSync(logID, "failed", 0, 0, err.Error())
		}
		status := syncStatusData{Running: false, LastMessage: "Steam sync failed: " + err.Error()}
		h.renderPartial(w, "sync_status_partial.html", status)
		return
	}
	for _, e := range result.Errors {
		log.Printf("steam sync: %s", e)
	}
	if logID > 0 {
		_ = h.store.FinishSync(logID, "done", result.Added, 0, "")
	}
	msg := fmt.Sprintf("Steam sync complete — +%d new games, %d skipped, %d errors",
		result.Added, result.Skipped, len(result.Errors))
	status := syncStatusData{Running: false, LastMessage: msg}
	h.renderPartial(w, "sync_status_partial.html", status)
}

// SyncInstallState accepts a JSON payload from the browser File System Access
// API client and records install state for the identified device.
func (h *Handler) SyncInstallState(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceName string          `json:"device_name"`
		Platform   string          `json:"platform"`
		Games      []db.InstallInput `json:"games"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.DeviceName == "" {
		http.Error(w, "device_name required", http.StatusBadRequest)
		return
	}

	devID := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(req.DeviceName), " ", "-"))

	logID, _ := h.store.StartSync("install")
	installed, notFound, err := h.store.SyncInstallSources(devID, req.DeviceName, req.Platform, req.Games)
	if logID > 0 {
		if err != nil {
			_ = h.store.FinishSync(logID, "failed", 0, 0, err.Error())
		} else {
			_ = h.store.FinishSync(logID, "done", installed, 0, "")
		}
	}
	if err != nil {
		log.Printf("sync install: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	log.Printf("install sync: %d installed, %d not found (device=%s)", installed, notFound, devID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"installed": installed,
		"not_found": notFound,
	})
}

// SyncPricing starts a background ITAD pricing sync job.
func (h *Handler) SyncPricing(w http.ResponseWriter, r *http.Request) {
	h.pricing.mu.Lock()
	already := h.pricing.running
	h.pricing.mu.Unlock()
	if already {
		status := syncStatusData{Running: true, Message: "Pricing sync already running…"}
		h.renderPartial(w, "sync_status_partial.html", status)
		return
	}

	h.pricing.mu.Lock()
	h.pricing.running = true
	h.pricing.step = "Starting…"
	h.pricing.stepDone = 0
	h.pricing.stepTotal = 0
	h.pricing.stepPct = 0
	h.pricing.lastMsg = ""
	h.pricing.mu.Unlock()

	runID := time.Now().Format(time.RFC3339)
	logID, _ := h.store.StartSync("pricing")
	go func() {
		setProgress := func(step string, done, total int) {
			pct := 0
			if total > 0 {
				pct = done * 100 / total
			}
			h.pricing.mu.Lock()
			h.pricing.step = step
			h.pricing.stepDone = done
			h.pricing.stepTotal = total
			h.pricing.stepPct = pct
			h.pricing.mu.Unlock()
		}

		// Phase 1: GG.deals (retail + keyshop prices for Steam-linked games)
		ggResult, ggErr := storesync.SyncGGDealsPricing(h.store, setProgress)
		for _, e := range ggResult.Errors {
			log.Printf("ggdeals pricing: %s", e)
		}

		// Phase 2: reseller scrapers for non-Steam wishlist entries only
		rResult, rErr := storesync.SyncResellerPricing(h.store, setProgress)
		for _, e := range rResult.Errors {
			log.Printf("reseller pricing: %s", e)
		}

		// Combine all per-item errors for display in the UI.
		var allErrors []string
		allErrors = append(allErrors, ggResult.Errors...)
		allErrors = append(allErrors, rResult.Errors...)

		if len(allErrors) > 0 {
			if err := h.store.AppendSyncErrors("pricing", runID, allErrors); err != nil {
				log.Printf("persist pricing errors: %v", err)
			}
		}

		h.pricing.mu.Lock()
		h.pricing.running = false
		h.pricing.lastErrors = allErrors
		if ggErr != nil {
			h.pricing.lastMsg = fmt.Sprintf(
				"GG.deals failed: %v — resellers: %d updated, %d not found, %d not cheaper",
				ggErr, rResult.Updated, rResult.NotFound, rResult.NotCheaper,
			)
		} else {
			h.pricing.lastMsg = fmt.Sprintf(
				"Pricing sync complete — GG.deals: %d updated, %d not found; resellers: %d cheaper, %d not listed, %d already cheaper, %d errors",
				ggResult.Updated, ggResult.NotFound,
				rResult.Updated, rResult.NotFound, rResult.NotCheaper,
				len(allErrors),
			)
		}
		if rErr != nil {
			log.Printf("reseller pricing fatal: %v", rErr)
		}
		log.Printf("pricing done: %s", h.pricing.lastMsg)
		h.pricing.mu.Unlock()

		syncStatus := "done"
		syncErrMsg := ""
		if ggErr != nil {
			syncStatus = "failed"
			syncErrMsg = ggErr.Error()
		}
		if logID > 0 {
			_ = h.store.FinishSync(logID, syncStatus, ggResult.Updated+rResult.Updated, 0, syncErrMsg)
		}
	}()

	status := syncStatusData{
		Running: true,
		Message: "Pricing sync started…",
		Detail:  "Phase 1: ITAD (current + historical low) → Phase 2: Loaded + instant-gaming (undercut check)",
	}
	h.renderPartial(w, "sync_status_partial.html", status)
}

// SyncWishlist fetches the Steam wishlist in the background (name resolution can be slow).
func (h *Handler) SyncWishlist(w http.ResponseWriter, r *http.Request) {
	h.wishlist.mu.Lock()
	already := h.wishlist.running
	h.wishlist.mu.Unlock()
	if already {
		status := syncStatusData{Running: true, Message: "Wishlist sync already running…"}
		h.renderPartial(w, "sync_status_partial.html", status)
		return
	}

	h.wishlist.mu.Lock()
	h.wishlist.running = true
	h.wishlist.lastMsg = ""
	h.wishlist.mu.Unlock()

	wlLogID, _ := h.store.StartSync("wishlist")
	go func() {
		result, err := storesync.SyncSteamWishlist(h.store)
		for _, e := range result.Errors {
			log.Printf("wishlist sync: %s", e)
		}
		linked, _ := h.store.LinkWishlistToLibrary()
		if linked > 0 {
			log.Printf("wishlist sync: linked %d entries to library", linked)
		}
		if wlLogID > 0 {
			if err != nil {
				_ = h.store.FinishSync(wlLogID, "failed", 0, 0, err.Error())
			} else {
				_ = h.store.FinishSync(wlLogID, "done", result.Added, 0, "")
			}
		}
		h.wishlist.mu.Lock()
		h.wishlist.running = false
		if err != nil {
			h.wishlist.lastMsg = "Wishlist sync failed: " + err.Error()
		} else {
			h.wishlist.lastMsg = fmt.Sprintf(
				"Steam wishlist sync complete — %d entries, %d errors",
				result.Added, len(result.Errors),
			)
		}
		log.Printf("wishlist done: %s", h.wishlist.lastMsg)
		h.wishlist.mu.Unlock()
	}()

	status := syncStatusData{
		Running: true,
		Message: "Steam wishlist sync started…",
		Detail:  "Fetching entries and resolving game names",
	}
	h.renderPartial(w, "sync_status_partial.html", status)
}

// SyncGOGWishlist fetches the GOG wishlist using the stored refresh token.
func (h *Handler) SyncGOGWishlist(w http.ResponseWriter, r *http.Request) {
	logID, _ := h.store.StartSync("wishlist")
	result, err := storesync.SyncGOGWishlist(h.store)
	if err != nil {
		if logID > 0 {
			_ = h.store.FinishSync(logID, "failed", 0, 0, err.Error())
		}
		status := syncStatusData{Running: false, LastMessage: "GOG wishlist sync failed: " + err.Error()}
		h.renderPartial(w, "sync_status_partial.html", status)
		return
	}
	for _, e := range result.Errors {
		log.Printf("gog wishlist sync: %s", e)
	}
	if linked, _ := h.store.LinkWishlistToLibrary(); linked > 0 {
		log.Printf("gog wishlist sync: linked %d entries to library", linked)
	}
	if logID > 0 {
		_ = h.store.FinishSync(logID, "done", result.Added, 0, "")
	}
	msg := fmt.Sprintf("GOG wishlist sync complete — %d entries, %d errors", result.Added, len(result.Errors))
	status := syncStatusData{Running: false, LastMessage: msg}
	h.renderPartial(w, "sync_status_partial.html", status)
}

// SyncPlaynite accepts a JSON payload from a Playnite script and records
// game metadata and ownership.
func (h *Handler) SyncPlaynite(w http.ResponseWriter, r *http.Request) {
	// Simple API secret check for automated syncs.
	secret, _ := h.store.GetConfig("sync.api_secret")
	if secret != "" {
		provided := r.Header.Get("X-Nisaba-Secret")
		if provided != secret {
			http.Error(w, "invalid secret", http.StatusUnauthorized)
			return
		}
	} else if !h.validSession(r) {
		http.Error(w, "auth required", http.StatusUnauthorized)
		return
	}

	var req struct {
		Games []struct {
			ID               string   `json:"id"`
			Title            string   `json:"title"`
			Source           string   `json:"source"`
			StoreID          string   `json:"store_id"`
			StoreURL         string   `json:"store_url"`
			PlayTimeMinutes  int      `json:"play_time_minutes"`
			LastPlayed       string   `json:"last_played"` // ISO8601
			Windows          bool     `json:"windows"`
			Mac              bool     `json:"mac"`
			Linux            bool     `json:"linux"`
			Developer        string   `json:"developer"`
			Publisher        string   `json:"publisher"`
			ReleaseDate      string   `json:"release_date"`
			Description      string   `json:"description"`
			ShortDescription string   `json:"short_description"`
		} `json:"games"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("SyncPlaynite: JSON decode error: %v", err)
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("SyncPlaynite: received %d games", len(req.Games))
	logID, _ := h.store.StartSync("playnite")
	var added, updated int
	var errors []string

	for _, g := range req.Games {
		// 1. Try to find the game by store link.
		gameID, err := h.store.FindGameByStoreID(g.Source, g.StoreID)
		if err != nil {
			log.Printf("SyncPlaynite: db error searching for %s: %v", g.Title, err)
			errors = append(errors, fmt.Sprintf("%s: db error searching: %v", g.Title, err))
			continue
		}

		if gameID == "" {
			// Fallback: Try to find by title to prevent duplicates if store link is new.
			gameID, err = h.store.FindGameByTitle(g.Title)
			if err != nil {
				log.Printf("SyncPlaynite: db error searching by title for %s: %v", g.Title, err)
			}
			if gameID != "" {
				log.Printf("SyncPlaynite: matched %s by title (no store link found)", g.Title)
			}
		}

		if gameID == "" {
			gameID = uuid.New().String()
			err = h.store.InsertGame(db.InsertGameParams{
				ID:               gameID,
				Title:            g.Title,
				SortTitle:        makeSortTitleH(g.Title),
				Developer:        &g.Developer,
				Description:      &g.Description,
				ShortDescription: &g.ShortDescription,
				ReleaseDate:      &g.ReleaseDate,
				Windows:          g.Windows,
				Mac:              g.Mac,
				Linux:            g.Linux,
				ArtworkJSON:      "{}",
			})
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to insert: %v", g.Title, err))
				continue
			}
			added++
		} else {
			updated++
		}

		// 2. Upsert store link.
		_ = h.store.UpsertGameStoreLink(gameID, g.Source, g.StoreID, g.StoreURL)

		// 3. Update playtime/last played if greater/later.
		if g.PlayTimeMinutes > 0 {
			_ = h.store.UpdatePlayTimeIfGreater(gameID, g.PlayTimeMinutes)
		}
		if g.LastPlayed != "" {
			if t, err := time.Parse(time.RFC3339, g.LastPlayed); err == nil {
				_ = h.store.UpdateLastPlayedIfLater(gameID, t)
			}
		}
	}

	if logID > 0 {
		status := "done"
		errMsg := ""
		if len(errors) > 0 {
			status = "partial"
			errMsg = fmt.Sprintf("%d errors encountered", len(errors))
			runID := time.Now().Format(time.RFC3339)
			_ = h.store.AppendSyncErrors("playnite", runID, errors)
		}
		_ = h.store.FinishSync(logID, status, added, updated, errMsg)
	}

	log.Printf("SyncPlaynite: finished. added=%d, updated=%d, errors=%d", added, updated, len(errors))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"added":   added,
		"updated": updated,
		"errors":  len(errors),
	})
}

// currentSyncStatus snapshots all running-state flags into a syncStatusData.
// Used both by SyncStatus (polling) and SyncPanel (initial page load).
func (h *Handler) currentSyncStatus() syncStatusData {
	h.syncAll.mu.Lock()
	syncAllRunning := h.syncAll.running
	syncAllStep := h.syncAll.step
	syncAllMsg := h.syncAll.lastMsg
	h.syncAll.mu.Unlock()

	h.enrichment.mu.Lock()
	enrichRunning := h.enrichment.running
	prog := h.enrichment.prog
	enrichMsg := h.enrichment.lastMsg
	h.enrichment.mu.Unlock()

	h.pricing.mu.Lock()
	pricingRunning := h.pricing.running
	pricingStep := h.pricing.step
	pricingDone := h.pricing.stepDone
	pricingTotal := h.pricing.stepTotal
	pricingPct := h.pricing.stepPct
	pricingMsg := h.pricing.lastMsg
	pricingErrors := h.pricing.lastErrors
	h.pricing.mu.Unlock()

	h.wishlist.mu.Lock()
	wishlistRunning := h.wishlist.running
	wishlistMsg := h.wishlist.lastMsg
	h.wishlist.mu.Unlock()

	switch {
	case syncAllRunning:
		return syncStatusData{Running: true, Message: "Full sync running…", Detail: syncAllStep}
	case enrichRunning:
		return syncStatusData{
			Running:  true,
			Message:  "Running IGDB enrichment…",
			Progress: fmt.Sprintf("%d / %d matched (%d errors)", prog.Matched, prog.Total, prog.Errors),
			Detail:   fmt.Sprintf("%d games processed", prog.Done),
		}
	case pricingRunning:
		return syncStatusData{
			Running:   true,
			Message:   "Pricing sync running…",
			Step:      pricingStep,
			StepDone:  pricingDone,
			StepTotal: pricingTotal,
			StepPct:   pricingPct,
		}
	case wishlistRunning:
		return syncStatusData{Running: true, Message: "Running wishlist sync…"}

	default:
	}

	h.deck.mu.Lock()
	deckRunning := h.deck.running
	deckDone := h.deck.done
	deckTotal := h.deck.total
	deckMsg := h.deck.lastMsg
	h.deck.mu.Unlock()

	h.proton.mu.Lock()
	protonRunning := h.proton.running
	protonDone := h.proton.done
	protonTotal := h.proton.total
	protonMsg := h.proton.lastMsg
	h.proton.mu.Unlock()

	h.crossref.mu.Lock()
	crossrefRunning := h.crossref.running
	crossrefMsg := h.crossref.lastMsg
	h.crossref.mu.Unlock()

	h.mysteryPack.mu.Lock()
	mysteryPackRunning := h.mysteryPack.running
	mysteryPackDone := h.mysteryPack.done
	mysteryPackTotal := h.mysteryPack.total
	mysteryPackMsg := h.mysteryPack.lastMsg
	h.mysteryPack.mu.Unlock()

	switch {
	case crossrefRunning:
		return syncStatusData{
			Running: true,
			Message: "Syncing Steam cross-references…",
			Step:    "IGDB external_games API",
		}
	case deckRunning:
		pct := 0
		if deckTotal > 0 {
			pct = deckDone * 100 / deckTotal
		}
		return syncStatusData{
			Running:   true,
			Message:   "Syncing Steam Deck status…",
			Step:      "Steam Store API",
			StepDone:  deckDone,
			StepTotal: deckTotal,
			StepPct:   pct,
		}
	case protonRunning:
		pct := 0
		if protonTotal > 0 {
			pct = protonDone * 100 / protonTotal
		}
		return syncStatusData{
			Running:   true,
			Message:   "Syncing ProtonDB ratings…",
			Step:      "ProtonDB API",
			StepDone:  protonDone,
			StepTotal: protonTotal,
			StepPct:   pct,
		}
	case mysteryPackRunning:
		pct := 0
		if mysteryPackTotal > 0 {
			pct = mysteryPackDone * 100 / mysteryPackTotal
		}
		return syncStatusData{
			Running:   true,
			Message:   "Analyzing mystery packs…",
			Step:      "GG.deals API",
			StepDone:  mysteryPackDone,
			StepTotal: mysteryPackTotal,
			StepPct:   pct,
		}
	case syncAllMsg != "":
		return syncStatusData{Running: false, LastMessage: syncAllMsg}
	case wishlistMsg != "":
		return syncStatusData{Running: false, LastMessage: wishlistMsg}
	case pricingMsg != "":
		return syncStatusData{Running: false, LastMessage: pricingMsg, Errors: pricingErrors}
	case crossrefMsg != "":
		return syncStatusData{Running: false, LastMessage: crossrefMsg}
	case deckMsg != "":
		return syncStatusData{Running: false, LastMessage: deckMsg}
	case protonMsg != "":
		return syncStatusData{Running: false, LastMessage: protonMsg}
	case mysteryPackMsg != "":
		return syncStatusData{Running: false, LastMessage: mysteryPackMsg}
	default:
		return syncStatusData{Running: false, LastMessage: enrichMsg}
	}
}

// SyncStatus returns an HTMX fragment with current sync progress.
// Polled every 2s while a sync is running.
func (h *Handler) SyncStatus(w http.ResponseWriter, r *http.Request) {
	h.renderPartial(w, "sync_status_partial.html", h.currentSyncStatus())
}

// RunEnrichment starts a background IGDB enrichment job.
func (h *Handler) RunEnrichment(w http.ResponseWriter, r *http.Request) {
	h.enrichment.mu.Lock()
	already := h.enrichment.running
	h.enrichment.mu.Unlock()
	if already {
		status := syncStatusData{Running: true, Message: "Enrichment already running…"}
		h.renderPartial(w, "sync_status_partial.html", status)
		return
	}

	// Build/reuse the IGDB client.
	h.igdbMu.Lock()
	if h.igdb == nil {
		clientID, _ := h.store.GetConfig("igdb.client_id")
		clientSecret, _ := h.store.GetConfig("igdb.client_secret")
		if clientID == "" || clientSecret == "" {
			h.igdbMu.Unlock()
			status := syncStatusData{Running: false, LastMessage: "IGDB credentials not set — configure them in Settings"}
			h.renderPartial(w, "sync_status_partial.html", status)
			return
		}
		h.igdb = storesync.NewIGDBClient(clientID, clientSecret)
	}
	client := h.igdb
	h.igdbMu.Unlock()

	h.enrichment.mu.Lock()
	h.enrichment.running = true
	h.enrichment.prog = storesync.EnrichProgress{}
	h.enrichment.lastMsg = ""
	h.enrichment.mu.Unlock()

	go func() {
		err := storesync.EnrichLibrary(h.store, client, func(p storesync.EnrichProgress) {
			h.enrichment.mu.Lock()
			h.enrichment.prog = p
			h.enrichment.mu.Unlock()
		}, h.rawgClient())

		h.enrichment.mu.Lock()
		h.enrichment.running = false
		p := h.enrichment.prog
		if err != nil {
			h.enrichment.lastMsg = fmt.Sprintf("Enrichment failed: %v", err)
		} else {
			h.enrichment.lastMsg = fmt.Sprintf(
				"Enrichment complete — %d matched, %d unmatched, %d errors (out of %d)",
				p.Matched, p.Total-p.Matched-p.Errors, p.Errors, p.Total,
			)
		}
		log.Printf("enrichment done: %s", h.enrichment.lastMsg)
		h.enrichment.mu.Unlock()
	}()

	status := syncStatusData{
		Running: true,
		Message: "IGDB enrichment started…",
		Detail:  "Searching IGDB for each game title (~4 req/sec)",
	}
	h.renderPartial(w, "sync_status_partial.html", status)
}

// RunWishlistEnrichment starts a background IGDB enrichment job for wishlist entries.
func (h *Handler) RunWishlistEnrichment(w http.ResponseWriter, r *http.Request) {
	h.enrichment.mu.Lock()
	already := h.enrichment.running
	h.enrichment.mu.Unlock()
	if already {
		status := syncStatusData{Running: true, Message: "Enrichment already running…"}
		h.renderPartial(w, "sync_status_partial.html", status)
		return
	}

	h.igdbMu.Lock()
	if h.igdb == nil {
		clientID, _ := h.store.GetConfig("igdb.client_id")
		clientSecret, _ := h.store.GetConfig("igdb.client_secret")
		if clientID == "" || clientSecret == "" {
			h.igdbMu.Unlock()
			status := syncStatusData{Running: false, LastMessage: "IGDB credentials not set — configure them in Settings"}
			h.renderPartial(w, "sync_status_partial.html", status)
			return
		}
		h.igdb = storesync.NewIGDBClient(clientID, clientSecret)
	}
	client := h.igdb
	h.igdbMu.Unlock()

	h.enrichment.mu.Lock()
	h.enrichment.running = true
	h.enrichment.prog = storesync.EnrichProgress{}
	h.enrichment.lastMsg = ""
	h.enrichment.mu.Unlock()

	go func() {
		err := storesync.EnrichWishlist(h.store, client, func(p storesync.EnrichProgress) {
			h.enrichment.mu.Lock()
			h.enrichment.prog = p
			h.enrichment.mu.Unlock()
		}, h.rawgClient())
		linked, _ := h.store.LinkWishlistToLibrary()
		if linked > 0 {
			log.Printf("wishlist enrichment: linked %d entries to library", linked)
		}

		h.enrichment.mu.Lock()
		h.enrichment.running = false
		p := h.enrichment.prog
		if err != nil {
			h.enrichment.lastMsg = fmt.Sprintf("Wishlist enrichment failed: %v", err)
		} else {
			h.enrichment.lastMsg = fmt.Sprintf(
				"Wishlist enrichment complete — %d matched, %d unmatched, %d errors (out of %d)",
				p.Matched, p.Total-p.Matched-p.Errors, p.Errors, p.Total,
			)
		}
		log.Printf("wishlist enrichment done: %s", h.enrichment.lastMsg)
		h.enrichment.mu.Unlock()
	}()

	status := syncStatusData{
		Running: true,
		Message: "Wishlist IGDB enrichment started…",
		Detail:  "Searching IGDB for each wishlist title (~4 req/sec)",
	}
	h.renderPartial(w, "sync_status_partial.html", status)
}

// rawgClient lazily loads the RAWG client from config, returning nil if not configured.
func (h *Handler) rawgClient() *storesync.RAWGClient {
	h.rawgMu.Lock()
	defer h.rawgMu.Unlock()
	if h.rawg == nil {
		key, _ := h.store.GetConfig("rawg.api_key")
		if key != "" {
			h.rawg = storesync.NewRAWGClient(key)
		}
	}
	return h.rawg
}

// SyncDeckStatus fetches Steam Deck compatibility for all Steam games missing that status.
func (h *Handler) SyncDeckStatus(w http.ResponseWriter, r *http.Request) {
	h.deck.mu.Lock()
	already := h.deck.running
	h.deck.mu.Unlock()
	if already {
		h.renderPartial(w, "sync_status_partial.html", syncStatusData{Running: true, Message: "Deck status sync already running…"})
		return
	}

	h.deck.mu.Lock()
	h.deck.running = true
	h.deck.done = 0
	h.deck.total = 0
	h.deck.lastMsg = ""
	h.deck.mu.Unlock()

	logID, _ := h.store.StartSync("deck")
	go func() {
		updated, err := storesync.SyncSteamDeckStatus(h.store, func(done, total int) {
			h.deck.mu.Lock()
			h.deck.done = done
			h.deck.total = total
			h.deck.mu.Unlock()
		})
		var msg string
		if err != nil {
			msg = "Deck status sync failed: " + err.Error()
		} else {
			msg = fmt.Sprintf("Steam Deck status sync complete — %d games updated", updated)
		}
		log.Printf("deck sync done: %s", msg)
		if logID > 0 {
			syncStatus := "done"
			errMsg := ""
			if err != nil {
				syncStatus = "failed"
				errMsg = err.Error()
			}
			_ = h.store.FinishSync(logID, syncStatus, updated, 0, errMsg)
		}
		h.deck.mu.Lock()
		h.deck.running = false
		h.deck.lastMsg = msg
		h.deck.mu.Unlock()
	}()

	h.renderPartial(w, "sync_status_partial.html", syncStatusData{
		Running: true,
		Message: "Steam Deck status sync started…",
		Detail:  "Fetching compatibility from Steam Store API (1 req/sec)",
	})
}

// SyncSteamCrossRefs looks up Steam App IDs via IGDB for all non-Steam library
// games that have an IGDB ID, and stores cross-reference rows so that the
// deck status sync can check those games too.
func (h *Handler) SyncSteamCrossRefs(w http.ResponseWriter, r *http.Request) {
	h.crossref.mu.Lock()
	already := h.crossref.running
	h.crossref.mu.Unlock()
	if already {
		h.renderPartial(w, "sync_status_partial.html", syncStatusData{Running: true, Message: "Steam cross-reference sync already running…"})
		return
	}

	h.igdbMu.Lock()
	if h.igdb == nil {
		clientID, _ := h.store.GetConfig("igdb.client_id")
		clientSecret, _ := h.store.GetConfig("igdb.client_secret")
		if clientID == "" || clientSecret == "" {
			h.igdbMu.Unlock()
			h.renderPartial(w, "sync_status_partial.html", syncStatusData{LastMessage: "IGDB credentials not configured."})
			return
		}
		h.igdb = storesync.NewIGDBClient(clientID, clientSecret)
	}
	client := h.igdb
	h.igdbMu.Unlock()

	h.crossref.mu.Lock()
	h.crossref.running = true
	h.crossref.done = 0
	h.crossref.total = 0
	h.crossref.lastMsg = ""
	h.crossref.mu.Unlock()

	go func() {
		inserted, err := storesync.SyncSteamCrossRefs(h.store, client, func(done, total int) {
			h.crossref.mu.Lock()
			h.crossref.done = done
			h.crossref.total = total
			h.crossref.mu.Unlock()
		})
		var msg string
		if err != nil {
			msg = "Steam cross-reference sync failed: " + err.Error()
		} else {
			msg = fmt.Sprintf("Steam cross-reference sync complete — %d games mapped to Steam", inserted)
		}
		log.Printf("crossref sync done: %s", msg)
		h.crossref.mu.Lock()
		h.crossref.running = false
		h.crossref.lastMsg = msg
		h.crossref.mu.Unlock()
	}()

	h.renderPartial(w, "sync_status_partial.html", syncStatusData{
		Running: true,
		Message: "Steam cross-reference sync started…",
		Detail:  "Looking up Steam App IDs via IGDB for non-Steam games",
	})
}

// SyncProtonRatings fetches ProtonDB community ratings for all Steam-owned games
// that don't yet have a stored rating.
func (h *Handler) SyncProtonRatings(w http.ResponseWriter, r *http.Request) {
	h.proton.mu.Lock()
	already := h.proton.running
	h.proton.mu.Unlock()
	if already {
		h.renderPartial(w, "sync_status_partial.html", syncStatusData{Running: true, Message: "ProtonDB sync already running…"})
		return
	}

	h.proton.mu.Lock()
	h.proton.running = true
	h.proton.done = 0
	h.proton.total = 0
	h.proton.lastMsg = ""
	h.proton.mu.Unlock()

	runID := time.Now().Format(time.RFC3339)
	logID, _ := h.store.StartSync("proton")
	go func() {
		result, err := storesync.SyncProtonRatings(h.store, func(done, total int) {
			h.proton.mu.Lock()
			h.proton.done = done
			h.proton.total = total
			h.proton.mu.Unlock()
		})
		if len(result.Errors) > 0 {
			if dbErr := h.store.AppendSyncErrors("proton", runID, result.Errors); dbErr != nil {
				log.Printf("persist proton errors: %v", dbErr)
			}
		}
		var msg string
		if err != nil {
			msg = "ProtonDB sync failed: " + err.Error()
		} else {
			msg = fmt.Sprintf("ProtonDB sync complete — %d updated, %d not found, %d errors",
				result.Updated, result.NotFound, len(result.Errors))
		}
		log.Printf("proton sync done: %s", msg)
		if logID > 0 {
			syncStatus := "done"
			errMsg := ""
			if err != nil {
				syncStatus = "failed"
				errMsg = err.Error()
			}
			_ = h.store.FinishSync(logID, syncStatus, result.Updated, 0, errMsg)
		}
		h.proton.mu.Lock()
		h.proton.running = false
		h.proton.lastMsg = msg
		h.proton.mu.Unlock()
	}()

	h.renderPartial(w, "sync_status_partial.html", syncStatusData{
		Running: true,
		Message: "ProtonDB rating sync started…",
		Detail:  "Fetching community ratings from ProtonDB (2 req/sec)",
	})
}

// UploadHeroicFiles accepts multipart-uploaded Heroic library JSON files,
// writes them to a temp directory, and runs ImportHeroicLibraries.
func (h *Handler) UploadHeroicFiles(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	tmpDir, err := os.MkdirTemp("", "heroic-upload-*")
	if err != nil {
		http.Error(w, "temp dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	allowed := map[string]bool{
		"legendary_library.json": true,
		"gog_library.json":       true,
		"nile_library.json":      true,
	}
	for _, fh := range r.MultipartForm.File["files"] {
		name := filepath.Base(fh.Filename)
		if !allowed[name] {
			continue
		}
		src, err := fh.Open()
		if err != nil {
			continue
		}
		dst, err := os.Create(filepath.Join(tmpDir, name))
		if err != nil {
			src.Close()
			continue
		}
		io.Copy(dst, src) //nolint:errcheck
		src.Close()
		dst.Close()
	}

	results, err := storesync.ImportHeroicLibraries(h.store, tmpDir)
	if err != nil {
		status := syncStatusData{Running: false, LastMessage: "Heroic import failed: " + err.Error()}
		h.renderPartial(w, "sync_status_partial.html", status)
		return
	}

	var parts []string
	var totalAdded, totalErrors int
	for _, res := range results {
		parts = append(parts, fmt.Sprintf("%s: +%d", res.Store, res.Added))
		totalAdded += res.Added
		totalErrors += len(res.Errors)
		for _, e := range res.Errors {
			log.Printf("heroic upload [%s]: %s", res.Store, e)
		}
	}
	msg := fmt.Sprintf("Heroic import complete — %s (%d total, %d errors)", strings.Join(parts, ", "), totalAdded, totalErrors)
	status := syncStatusData{Running: false, LastMessage: msg}
	h.renderPartial(w, "sync_status_partial.html", status)
}

// ImportHeroic reads the Heroic JSON library files and upserts games into the DB.
func (h *Handler) ImportHeroic(w http.ResponseWriter, r *http.Request) {
	dir, err := h.store.GetConfig("heroic.library_path")
	if err != nil || dir == "" {
		dir = "./store_library_files"
	}

	results, err := storesync.ImportHeroicLibraries(h.store, dir)
	if err != nil {
		http.Error(w, "import failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Build a summary message.
	var parts []string
	var totalAdded, totalErrors int
	for _, r := range results {
		parts = append(parts, fmt.Sprintf("%s: +%d", r.Store, r.Added))
		totalAdded += r.Added
		totalErrors += len(r.Errors)
		for _, e := range r.Errors {
			log.Printf("heroic import [%s]: %s", r.Store, e)
		}
	}
	msg := fmt.Sprintf("Heroic import complete — %s (%d total, %d errors)", strings.Join(parts, ", "), totalAdded, totalErrors)

	// Return the status partial showing the result.
	status := syncStatusData{Running: false, LastMessage: msg}
	h.renderPartial(w, "sync_status_partial.html", status)
}

// SyncMysteryPacks analyzes all enabled mystery packs and persists results to the database.
func (h *Handler) SyncMysteryPacks(w http.ResponseWriter, r *http.Request) {
	h.mysteryPack.mu.Lock()
	already := h.mysteryPack.running
	h.mysteryPack.mu.Unlock()
	if already {
		h.renderPartial(w, "sync_status_partial.html", syncStatusData{Running: true, Message: "Mystery pack analysis already running…"})
		return
	}

	h.mysteryPack.mu.Lock()
	h.mysteryPack.running = true
	h.mysteryPack.done = 0
	h.mysteryPack.total = 0
	h.mysteryPack.lastMsg = ""
	h.mysteryPack.mu.Unlock()

	logID, _ := h.store.StartSync("mystery_packs")
	go func() {
		apiKey, _ := h.store.GetConfig("ggdeals.api_key")

		result, err := storesync.SyncMysteryPacks(h.store, apiKey, func(done, total int) {
			h.mysteryPack.mu.Lock()
			h.mysteryPack.done = done
			h.mysteryPack.total = total
			h.mysteryPack.mu.Unlock()
		})

		var msg string
		if err != nil {
			msg = "Mystery pack analysis failed: " + err.Error()
		} else {
			var errorSummary string
			if len(result.Errors) > 0 {
				errorSummary = fmt.Sprintf(" (%d errors)", len(result.Errors))
				for _, e := range result.Errors {
					log.Printf("mystery pack: %s", e)
				}
			}
			msg = fmt.Sprintf("Mystery pack analysis complete — %d analyzed%s", result.Analyzed, errorSummary)
		}
		log.Printf("mystery pack sync done: %s", msg)

		if logID > 0 {
			syncStatus := "done"
			errMsg := ""
			if err != nil {
				syncStatus = "failed"
				errMsg = err.Error()
			}
			_ = h.store.FinishSync(logID, syncStatus, result.Analyzed, 0, errMsg)
		}

		h.mysteryPack.mu.Lock()
		h.mysteryPack.running = false
		h.mysteryPack.lastMsg = msg
		h.mysteryPack.mu.Unlock()
	}()

	h.renderPartial(w, "sync_status_partial.html", syncStatusData{
		Running: true,
		Message: "Mystery pack analysis started…",
		Detail:  "Fetching keyshop prices from GG.deals API",
	})
}
