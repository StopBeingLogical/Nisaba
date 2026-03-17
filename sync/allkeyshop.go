package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// aksOffer mirrors one offer object embedded in an Allkeyshop product page.
type aksOffer struct {
	OriginalPrice float64 `json:"originalPrice"`
	MerchantIcon  string  `json:"merchantIcon"`
	MerchantName  string  `json:"merchantName"`
	Region        string  `json:"region"`
	Price         float64 `json:"price"`
	Dispo         int     `json:"dispo"`   // 1 = in stock
	Account       bool    `json:"account"` // true = account-sharing listing (not a key)
	Platform      string  `json:"activationPlatform"`
}

// aksRegionAllowed restricts results to global and US keys.
// Region "2" = Global, "25" = US. Other codes (433, 477, etc.) are cheap-region
// keys (Argentina, Turkey, etc.) that may not activate outside their target region.
var aksRegionAllowed = map[string]bool{"2": true, "25": true}

var aksOfferRe = regexp.MustCompile(`\{[^{}]*"merchantIcon"[^{}]*\}`)

// aksJitter sleeps for a random duration between 2 and 5 seconds.
// Fixed-interval requests are a bot signal; jitter makes the pattern human-like.
func aksJitter() {
	time.Sleep(time.Duration(2000+rand.Intn(3000)) * time.Millisecond)
}

// aksWarmup fetches the AKS homepage so the cookie jar picks up any session
// cookies before the first search request. Called once per sync run.
func aksWarmup(client *http.Client) {
	req, err := http.NewRequest("GET", "https://www.allkeyshop.com/blog/", nil)
	if err != nil {
		return
	}
	for k, v := range browserHeaders {
		req.Header.Set(k, v)
	}
	for k, v := range chromeSecHeaders {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// searchAllKeyShop fetches the Allkeyshop product page for the given title and
// returns the best price across all in-stock, non-account, globally-valid offers,
// along with the merchant name that holds it.
//
// Returns (0, "", nil) when the game has no Allkeyshop page or no qualifying offers.
func searchAllKeyShop(client *http.Client, title string) (float64, string, error) {
	slug := aksSlug(title)
	pageURL := "https://www.allkeyshop.com/blog/buy-" + slug + "-cd-key-compare-prices/"

	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return 0, "", err
	}
	for k, v := range browserHeaders {
		req.Header.Set(k, v)
	}
	for k, v := range chromeSecHeaders {
		req.Header.Set(k, v)
	}
	// Referer: as if navigating from the AKS homepage.
	req.Header.Set("Referer", "https://www.allkeyshop.com/blog/")

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("allkeyshop: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return 0, "", nil // game not listed on Allkeyshop
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return 0, "", fmt.Errorf("allkeyshop: HTTP %d (bot protection)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("allkeyshop: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return 0, "", fmt.Errorf("allkeyshop read: %w", err)
	}

	var bestPrice float64
	var bestMerchant string

	for _, raw := range aksOfferRe.FindAll(body, -1) {
		var o aksOffer
		if err := json.Unmarshal(raw, &o); err != nil {
			continue
		}
		if o.Dispo != 1 {
			continue // out of stock
		}
		if o.Account {
			continue // skip account-sharing listings, only want keys
		}
		if !aksRegionAllowed[o.Region] {
			continue // skip cheap-region keys unlikely to work in US/Global
		}
		if o.Price <= 0 {
			continue
		}
		if bestPrice == 0 || o.Price < bestPrice {
			bestPrice = o.Price
			bestMerchant = o.MerchantName
		}
	}

	return bestPrice, bestMerchant, nil
}

// aksSlug converts a game title to the Allkeyshop URL slug.
// Mirrors WordPress sanitize_title_with_dashes closely enough for the common case.
func aksSlug(title string) string {
	var b strings.Builder
	prev := '-'
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prev = r
		case r == '\'':
			// apostrophes are dropped entirely (baldur's → baldurs)
		case unicode.IsSpace(r) || r == '-' || r == ':' || r == ',' || r == '.' || r == '!':
			if prev != '-' {
				b.WriteRune('-')
				prev = '-'
			}
		// other chars (™, ®, &, etc.) are dropped
		}
	}
	return strings.Trim(b.String(), "-")
}
