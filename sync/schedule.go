package sync

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"nisaba/db"
)

// priceSyncHourKey is the app_config key holding the hour (0-23, in the
// container's clock) that the daily price sync runs at.
const priceSyncHourKey = "sync.price_hour"

// priceSyncHourDefault is 07:00 US Eastern while the container runs on UTC.
const priceSyncHourDefault = 11

// gogLibrarySyncHourKey is the app_config key holding the hour that the daily
// GOG library sync runs at.
const gogLibrarySyncHourKey = "sync.gog_hour"

// gogLibrarySyncHourDefault matches the price window: 07:00 US Eastern.
const gogLibrarySyncHourDefault = 11

// syncStaleness is how old the last run of a daily job may be before a restart
// makes the scheduler catch up rather than wait for the next window.
const syncStaleness = 20 * time.Hour

// syncRetry is how long to wait before looking again after a sync could not
// start (another sync running, provider unconfigured).
const syncRetry = 10 * time.Minute

// StartDailyPriceSync refreshes prices once a day at the configured hour. It
// runs the price providers only — never ownership, wishlists, resellers, or
// enrichment — and records the run in sync_log as type "pricing". A run that
// starts and fails is recorded once and waits for the next window; the manual
// full sync is the recovery path.
func StartDailyPriceSync(store *db.Store) {
	go dailySync(store, "price", priceSyncHourKey, priceSyncHourDefault, "pricing", runScheduledPriceSync)
}

// StartDailyGOGLibrarySync imports the GOG library once a day at the configured
// hour. It has its own window so the price job stays price-only, and records
// the run in sync_log as type "ownership".
func StartDailyGOGLibrarySync(store *db.Store) {
	go dailySync(store, "gog library", gogLibrarySyncHourKey, gogLibrarySyncHourDefault, "ownership", runScheduledGOGLibrarySync)
}

// dailySync runs one daily job: at the configured hour, or immediately when
// today's window has passed and that job's last run is stale.
func dailySync(store *db.Store, label, hourKey string, hourDefault int, syncType string, run func(*db.Store) bool) {
	for {
		hour := configuredHour(store, hourKey, hourDefault)
		now := time.Now()

		if windowPassed(store, now, hour, syncType) {
			if run(store) {
				continue
			}
			time.Sleep(syncRetry)
			continue
		}

		next := nextWindow(now, hour)
		log.Printf("%s sync: next run %s (in %s)",
			label, next.Format("2006-01-02 15:04"), time.Until(next).Round(time.Minute))
		time.Sleep(time.Until(next))
	}
}

// configuredHour reads the configured hour, falling back to the default when it
// is absent or unparsable.
func configuredHour(store *db.Store, key string, fallback int) int {
	raw, err := store.GetConfig(key)
	if err != nil {
		return fallback
	}
	hour, err := strconv.Atoi(raw)
	if err != nil || hour < 0 || hour > 23 {
		return fallback
	}
	return hour
}

// nextWindow is the next occurrence of the configured hour.
func nextWindow(now time.Time, hour int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// windowPassed reports whether today's window has already passed and this job's
// last run is stale enough to justify running now. This is the catch-up path for
// a container that was down, or a deploy, at the scheduled hour.
func windowPassed(store *db.Store, now time.Time, hour int, syncType string) bool {
	if now.Hour() < hour {
		return false
	}
	last, ok := lastSyncOfType(store, syncType)
	return !ok || now.UTC().Sub(last) >= syncStaleness
}

// lastSyncOfType is the start time of the most recent run of this type, if any.
func lastSyncOfType(store *db.Store, syncType string) (time.Time, bool) {
	entry, err := store.LatestSyncByType(syncType)
	if err != nil || entry == nil {
		return time.Time{}, false
	}
	started, err := time.Parse("2006-01-02 15:04:05", entry.StartedAt)
	if err != nil {
		return time.Time{}, false
	}
	return started, true
}

// runScheduledPriceSync performs one price-only sync. It returns false, without
// writing a sync_log row, when it could not start at all.
func runScheduledPriceSync(store *db.Store) bool {
	busy, err := store.HasRunningSync()
	if err != nil {
		log.Printf("price sync: skipped, could not read sync state: %v", err)
		return false
	}
	if busy {
		log.Printf("price sync: skipped, another sync is running")
		return false
	}
	if apiKey, _ := store.GetConfig("itad.api_key"); apiKey == "" {
		log.Printf("price sync: skipped, itad.api_key is not configured")
		return false
	}

	logID, _ := store.StartSync("pricing")
	log.Printf("price sync: starting")

	itad, itadErr := SyncITADPricing(store, nil)
	gg, ggErr := SyncGGDealsPricing(store, nil)

	status, errMsg := "done", ""
	switch {
	case itadErr != nil && ggErr != nil:
		status, errMsg = "failed", itadErr.Error()+"; "+ggErr.Error()
	case itadErr != nil:
		status, errMsg = "partial", itadErr.Error()
	case ggErr != nil:
		status, errMsg = "partial", ggErr.Error()
	}
	if logID > 0 {
		_ = store.FinishSync(logID, status, 0, itad.Updated, errMsg)
	}
	log.Printf("price sync: %s — ITAD %d updated (%d errors), GG.deals %d priced (%d errors)",
		status, itad.Updated, len(itad.Errors), gg.Updated, len(gg.Errors))
	return true
}

// runScheduledGOGLibrarySync performs one GOG library sync. It returns false,
// without writing a sync_log row, when it could not start at all.
func runScheduledGOGLibrarySync(store *db.Store) bool {
	busy, err := store.HasRunningSync()
	if err != nil {
		log.Printf("gog library sync: skipped, could not read sync state: %v", err)
		return false
	}
	if busy {
		log.Printf("gog library sync: skipped, another sync is running")
		return false
	}
	if refreshToken, _ := store.GetConfig("gog.refresh_token"); refreshToken == "" {
		log.Printf("gog library sync: skipped, GOG is not configured")
		return false
	}

	logID, _ := store.StartSync("ownership")
	log.Printf("gog library sync: starting")

	result, syncErr := SyncGOGLibrary(store)

	status, errMsg := "done", ""
	switch {
	case syncErr != nil:
		status, errMsg = "failed", syncErr.Error()
	case len(result.Errors) > 0:
		status = "partial"
		errMsg = fmt.Sprintf("%d errors encountered", len(result.Errors))
	}
	if logID > 0 {
		_ = store.FinishSync(logID, status, result.Added, 0, errMsg)
	}
	log.Printf("gog library sync: %s — %d added, %d skipped, %d errors",
		status, result.Added, result.Skipped, len(result.Errors))
	return true
}
