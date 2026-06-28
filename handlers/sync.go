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
	Step         string
	StepDone     int
	StepTotal    int
	StepPct      int
	Progress     string
	LastMessage  string
	LastFinished *time.Time
	Errors       []string
}

// SyncPanel renders the sync dashboard.
func (h *Handler) SyncPanel(w http.ResponseWriter, r *http.Request) {
	base, err := h.baseData("sync")
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	recent, _ := h.store.RecentSyncs(20)
	lastInstall, _ := h.store.LatestSyncByType("install")

	base["RecentSyncs"] = recent
	base["SyncStatus"] = h.currentSyncStatus()

	if lastInstall != nil && lastInstall.FinishedAt.Valid {
		base["LastInstallSync"] = formatTime(lastInstall.FinishedAt.String)
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

		h.cleanupWishlistLinks()

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

// SyncPlaynite accepts a JSON payload from a Playnite script and records
// game metadata and ownership.
func (h *Handler) SyncPlaynite(w http.ResponseWriter, r *http.Request) {
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
			LastPlayed       string   `json:"last_played"`
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
		gameID, err := h.store.FindGameByStoreID(g.Source, g.StoreID)
		if err != nil {
			log.Printf("SyncPlaynite: db error searching for %s: %v", g.Title, err)
			errors = append(errors, fmt.Sprintf("%s: db error searching: %v", g.Title, err))
			continue
		}

		if gameID == "" {
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

		_ = h.store.UpsertGameStoreLink(gameID, g.Source, g.StoreID, g.StoreURL)

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

	h.cleanupWishlistLinks()

	log.Printf("SyncPlaynite: finished. added=%d, updated=%d, errors=%d", added, updated, len(errors))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"added":   added,
		"updated": updated,
		"errors":  len(errors),
	})
}

// currentSyncStatus snapshots all running-state flags into a syncStatusData.
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
	case syncAllMsg != "":
		return syncStatusData{Running: false, LastMessage: syncAllMsg}
	default:
		return syncStatusData{Running: false, LastMessage: enrichMsg}
	}
}

// SyncStatus returns an HTMX fragment with current sync progress.
func (h *Handler) SyncStatus(w http.ResponseWriter, r *http.Request) {
	h.renderPartial(w, "sync_status_partial.html", h.currentSyncStatus())
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

// SyncMysteryPacks analyzes all enabled mystery packs.
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

// UploadHeroicFiles accepts multipart-uploaded Heroic library JSON files.
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
		io.Copy(dst, src)
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
	h.cleanupWishlistLinks()
	status := syncStatusData{Running: false, LastMessage: msg}
	h.renderPartial(w, "sync_status_partial.html", status)
}
