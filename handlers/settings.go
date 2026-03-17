package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// AppConfig is the settings view model matching the settings.html template.
type AppConfig struct {
	InstantBuyThreshold string
	ConsiderThreshold   string
	Currency            string
	SteamUserID         string
	SteamAPIKeySet      bool
	IGDBClientID        string
	IGDBClientSecretSet bool
	RAWGAPIKeySet       bool
	GGDealsAPIKeySet    bool
	ITADAPIKeySet       bool
	GOGRefreshTokenSet  bool
	GOGTokenStatus      string // "valid", "expired", or ""
	GOGTokenExpiry      string // human-readable expiry time
	HeroicLibraryPath   string
	SessionHours        string
	PasswordSet         bool
}

// Settings renders the settings page.
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	base, err := h.baseData("settings")
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	cfg, err := h.store.AllConfig()
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	view := AppConfig{
		InstantBuyThreshold: orDefault(cfg["pricing.instant_buy_threshold"], "2.00"),
		ConsiderThreshold:   orDefault(cfg["pricing.consider_threshold"], "5.00"),
		Currency:            orDefault(cfg["pricing.currency"], "USD"),
		SteamUserID:         cfg["steam.user_id"],
		SteamAPIKeySet:      cfg["steam.api_key"] != "",
		IGDBClientID:        cfg["igdb.client_id"],
		IGDBClientSecretSet: cfg["igdb.client_secret"] != "",
		RAWGAPIKeySet:       cfg["rawg.api_key"] != "",
		GGDealsAPIKeySet:    cfg["ggdeals.api_key"] != "",
		ITADAPIKeySet:       cfg["itad.api_key"] != "",
		GOGRefreshTokenSet:  cfg["gog.refresh_token"] != "",
		HeroicLibraryPath:   orDefault(cfg["heroic.library_path"], "./store_library_files"),
		SessionHours:        orDefault(cfg["auth.session_hours"], "12"),
		PasswordSet:         cfg["auth.password_hash"] != "",
	}

	if exp, err := strconv.ParseInt(cfg["gog.access_token_expires"], 10, 64); err == nil {
		t := time.Unix(exp, 0)
		view.GOGTokenExpiry = t.Format("15:04 on Jan 2")
		if time.Now().Unix() < exp {
			view.GOGTokenStatus = "valid"
		} else {
			view.GOGTokenStatus = "expired"
		}
	}

	thresholds, _ := h.store.ListPriceThresholds()

	base["Config"] = view
	base["Host"] = r.Host
	base["PriceThresholds"] = thresholds
	h.render(w, "settings.html", base)
}

// SaveSettings persists settings form values to app_config.
func (h *Handler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	pairs := map[string]string{
		"pricing.instant_buy_threshold": r.FormValue("instant_buy_threshold"),
		"pricing.consider_threshold":    r.FormValue("consider_threshold"),
		"pricing.currency":              r.FormValue("currency"),
		"steam.user_id":                 r.FormValue("steam_user_id"),
		"igdb.client_id":                r.FormValue("igdb_client_id"),
		"heroic.library_path":           r.FormValue("heroic_library_path"),
	}

	// Only update secrets if non-empty (empty = "don't change existing value")
	for _, pair := range [][2]string{
		{"steam_api_key", "steam.api_key"},
		{"igdb_client_secret", "igdb.client_secret"},
		{"rawg_api_key", "rawg.api_key"},
		{"ggdeals_api_key", "ggdeals.api_key"},
		{"itad_api_key", "itad.api_key"},
		{"gog_refresh_token", "gog.refresh_token"},
	} {
		if v := r.FormValue(pair[0]); v != "" {
			pairs[pair[1]] = v
		}
	}

	for k, v := range pairs {
		if v == "" {
			continue
		}
		if err := h.store.SetConfig(k, v); err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<span class="text-green-400">Saved.</span>`))
}

// AddPriceThreshold creates a new named price threshold.
func (h *Handler) AddPriceThreshold(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	label := r.FormValue("label")
	price, err := strconv.ParseFloat(r.FormValue("max_price"), 64)
	if err != nil || label == "" || price <= 0 {
		http.Error(w, "label and valid price required", http.StatusBadRequest)
		return
	}
	if err := h.store.AddPriceThreshold(label, price); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	// Re-render the thresholds list partial.
	thresholds, _ := h.store.ListPriceThresholds()
	h.renderPartial(w, "settings_thresholds_partial.html", thresholds)
}

// DeletePriceThreshold removes a named price threshold.
func (h *Handler) DeletePriceThreshold(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.store.DeletePriceThreshold(id); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	thresholds, _ := h.store.ListPriceThresholds()
	h.renderPartial(w, "settings_thresholds_partial.html", thresholds)
}

// ChangePassword hashes and stores a new app password, and updates the
// session timeout duration.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	if hours := r.FormValue("session_hours"); hours != "" {
		_ = h.store.SetConfig("auth.session_hours", hours)
	}

	password := r.FormValue("password")
	if password == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<span class="text-green-400">Timeout saved.</span>`))
		return
	}

	hash, err := HashPassword(password)
	if err != nil {
		http.Error(w, "hash error", http.StatusInternalServerError)
		return
	}
	if err := h.store.SetConfig("auth.password_hash", hash); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<span class="text-green-400">Password updated. You will be prompted to log in again after your current session expires.</span>`))
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
