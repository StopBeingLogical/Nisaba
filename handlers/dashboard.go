package handlers

import (
	"net/http"
)

// Dashboard renders the home/dashboard page.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	base, err := h.baseData("dashboard")
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	stats, _ := h.store.GetDashboardStats()
	alerts, _ := h.store.ListPriceAlerts()
	nearLow, _ := h.store.ListNearHistoricalLow()
	highPri, _ := h.store.ListHighPriorityWishlist()
	flagged, _ := h.store.ListFlaggedForRemoval()
	recentSyncs, _ := h.store.RecentSyncs(6)

	base["Stats"] = stats
	base["PriceAlerts"] = alerts
	base["NearHistoricalLow"] = nearLow
	base["HighPriority"] = highPri
	base["FlaggedForRemoval"] = flagged
	base["RecentSyncs"] = recentSyncs

	h.render(w, "dashboard.html", base)
}
