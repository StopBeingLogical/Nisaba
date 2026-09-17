package sync

import (
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

// priceSyncStaleness is how old the last price sync may be before a restart
// makes the scheduler catch up rather than wait for the next window.
const priceSyncStaleness = 20 * time.Hour

// priceSyncRetry is how long to wait before looking again after a sync could
// not start (another sync running, provider unconfigured).
const priceSyncRetry = 10 * time.Minute

// StartDailyPriceSync refreshes prices once a day at the configured hour. It
// runs the price providers only — never ownership, wishlists, resellers, or
// enrichment — and records the run in sync_log as type "pricing". A run that
// starts and fails is recorded once and waits for the next window; the manual
// full sync is the recovery path.
func StartDailyPriceSync(store *db.Store) {
	go func() {
		for {
			hour := priceSyncHour(store)
			now := time.Now()

			if overdue(store, now, hour) {
				if runScheduledPriceSync(store) {
					continue
				}
				time.Sleep(priceSyncRetry)
				continue
			}

			next := nextPriceSync(now, hour)
			log.Printf("price sync: next run %s (in %s)",
				next.Format("2006-01-02 15:04"), time.Until(next).Round(time.Minute))
			time.Sleep(time.Until(next))
		}
	}()
}

// priceSyncHour reads the configured hour, falling back to the default when it
// is absent or unparsable.
func priceSyncHour(store *db.Store) int {
	raw, err := store.GetConfig(priceSyncHourKey)
	if err != nil {
		return priceSyncHourDefault
	}
	hour, err := strconv.Atoi(raw)
	if err != nil || hour < 0 || hour > 23 {
		return priceSyncHourDefault
	}
	return hour
}

// nextPriceSync is the next occurrence of the configured hour.
func nextPriceSync(now time.Time, hour int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// overdue reports whether today's window has already passed and prices are
// stale enough to justify running now. This is the catch-up path for a
// container that was down, or a deploy, at the scheduled hour.
func overdue(store *db.Store, now time.Time, hour int) bool {
	if now.Hour() < hour {
		return false
	}
	last, ok := lastPriceSync(store)
	return !ok || now.UTC().Sub(last) >= priceSyncStaleness
}

// lastPriceSync is the start time of the most recent price sync, if any.
func lastPriceSync(store *db.Store) (time.Time, bool) {
	entry, err := store.LatestSyncByType("pricing")
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
