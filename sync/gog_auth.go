package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"nisaba/db"
)

// GOGClientID is the GOG Galaxy public client ID.
const GOGClientID = "46899977096215655"

// gogTokenURL is GOG's OAuth token endpoint.
const gogTokenURL = "https://auth.gog.com/token"

// gogGetAccessToken returns a usable GOG access token, refreshing it from the
// stored refresh token when the recorded expiry has passed.
//
// GOG accepts a token long past that stamp (proved 2026-09-16: a token recorded
// as expired 2026-03-09 still answered 200), so a failed refresh does not
// invalidate a stored token — the stored one is returned instead, and the
// refresh error is logged.
func gogGetAccessToken(store *db.Store) (string, error) {
	stored, _ := store.GetConfig("gog.access_token")

	if stored != "" && !gogTokenExpired(store) {
		return stored, nil
	}

	refreshed, refreshErr := gogRefreshAccessToken(store)
	if refreshErr == nil {
		return refreshed, nil
	}
	if stored != "" {
		log.Printf("gog: refresh failed, using the stored token: %v", refreshErr)
		return stored, nil
	}
	return "", refreshErr
}

// gogTokenExpired reports whether the recorded expiry has passed. A missing or
// unparsable stamp counts as expired, which sends the caller to a refresh.
func gogTokenExpired(store *db.Store) bool {
	raw, _ := store.GetConfig("gog.access_token_expires")
	if raw == "" {
		return true
	}
	exp, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return true
	}
	return time.Now().Unix() >= exp
}

// gogRefreshAccessToken exchanges the stored refresh token for a new access
// token and stores both back. GOG returns a rotated refresh token; the previous
// one keeps working (proved 2026-09-16), so this write is safe to repeat.
func gogRefreshAccessToken(store *db.Store) (string, error) {
	refreshToken, _ := store.GetConfig("gog.refresh_token")
	if refreshToken == "" {
		return "", fmt.Errorf("GOG not configured — paste auth.json in Settings")
	}
	clientSecret, _ := store.GetConfig("gog.client_secret")
	if clientSecret == "" {
		return "", fmt.Errorf("GOG client secret is not set — add it in Settings")
	}

	query := url.Values{
		"client_id":     {GOGClientID},
		"client_secret": {clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(gogTokenURL + "?" + query.Encode())
	if err != nil {
		return "", fmt.Errorf("GOG token refresh: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GOG token refresh HTTP %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("GOG token refresh: %w", err)
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("GOG token refresh returned no access token")
	}

	_ = store.SetConfig("gog.access_token", payload.AccessToken)
	if payload.RefreshToken != "" {
		_ = store.SetConfig("gog.refresh_token", payload.RefreshToken)
	}
	if payload.ExpiresIn > 0 {
		expires := time.Now().Unix() + payload.ExpiresIn
		_ = store.SetConfig("gog.access_token_expires", strconv.FormatInt(expires, 10))
	}

	return payload.AccessToken, nil
}
