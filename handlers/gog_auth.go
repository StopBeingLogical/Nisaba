package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
)

// GOGAuthExchange parses pasted auth.json content (form POST from the UI).
func (h *Handler) GOGAuthExchange(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	raw := r.FormValue("code")
	if raw == "" {
		w.Write([]byte(`<span class="text-red-400">Nothing pasted.</span>`))
		return
	}

	msg, err := saveGOGAuthJSON(h.store, []byte(raw))
	if err != nil {
		fmt.Fprintf(w, `<span class="text-red-400">%s</span>`, err.Error())
		return
	}
	fmt.Fprintf(w, `<span class="text-green-400">%s</span>`, msg)
}

// GOGAuthPush accepts a raw auth.json body (for curl from the Steam Deck).
func (h *Handler) GOGAuthPush(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	msg, err := saveGOGAuthJSON(h.store, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fmt.Fprintln(w, msg)
}

func saveGOGAuthJSON(store interface {
	SetConfig(string, string) error
}, raw []byte) (string, error) {
	var outer map[string]gogAuthEntry
	if err := json.Unmarshal(raw, &outer); err != nil || len(outer) == 0 {
		return "", fmt.Errorf("could not parse auth.json")
	}

	var entry gogAuthEntry
	for _, v := range outer {
		entry = v
		break
	}
	if entry.AccessToken == "" {
		return "", fmt.Errorf("no access_token found in auth.json")
	}

	expiresAt := int64(math.Round(entry.LoginTime)) + int64(entry.ExpiresIn)

	pairs := map[string]string{
		"gog.access_token":         entry.AccessToken,
		"gog.access_token_expires": strconv.FormatInt(expiresAt, 10),
	}
	if entry.RefreshToken != "" {
		pairs["gog.refresh_token"] = entry.RefreshToken
	}
	for k, v := range pairs {
		if err := store.SetConfig(k, v); err != nil {
			return "", fmt.Errorf("DB error saving %s", k)
		}
	}

	expires := time.Unix(expiresAt, 0)
	return fmt.Sprintf("✓ GOG token saved. Valid until %s.", expires.Format("15:04 on Jan 2")), nil
}

type gogAuthEntry struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	ExpiresIn    int64   `json:"expires_in"`
	LoginTime    float64 `json:"loginTime"`
}
