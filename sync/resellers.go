package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	gosync "sync"
	"time"

	"nisaba/db"
)

// ResellerResult summarises a reseller pricing sync run.
type ResellerResult struct {
	Updated    int
	NotFound   int      // price returned 0 from both scrapers — not listed on either site
	NotCheaper int      // found a price but it's not lower than the stored price
	Errors     []string // per-item fetch or parse failures
}

// browserHeaders are applied to all scraper requests to pass basic bot filters.
var browserHeaders = map[string]string{
	"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
	"Accept-Language": "en-US,en;q=0.9",
	"Cache-Control":   "no-cache",
}

// priceRe matches a bare decimal price like 12.99 or 3.00 (not integers alone).
var priceRe = regexp.MustCompile(`[\$€£]?\s*(\d{1,4}\.\d{2})`)

// SyncResellerPricing scrapes Loaded.com and instant-gaming.com for wishlist
// prices, updating best_current_price only when a cheaper price is found.
// progress is called with (step label, done count, total count) as work advances;
// it may be nil.
func SyncResellerPricing(store *db.Store, progress func(step string, done, total int)) (ResellerResult, error) {
	var result ResellerResult

	prog := func(done, total int) {
		if progress != nil {
			progress("Reseller scraping", done, total)
		}
	}

	entries, err := store.ListWishlistForResellerPricing()
	if err != nil {
		return result, fmt.Errorf("listing wishlist: %w", err)
	}
	if len(entries) == 0 {
		return result, nil
	}

	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			for k, v := range browserHeaders {
				req.Header.Set(k, v)
			}
			return nil
		},
	}

	type scrapeResult struct {
		entryID string
		title   string
		price   float64
		store   string
		err     error
	}

	type entryState struct {
		responses int
		bestPrice float64
		bestStore string
	}

	const numSites = 2
	total := len(entries)
	resultCh := make(chan scrapeResult, numSites*total)

	// One goroutine per site, each with its own rate limiter.
	var wg gosync.WaitGroup

	// ── instant-gaming.com goroutine ────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for _, entry := range entries {
			<-ticker.C
			price, err := searchInstantGaming(client, entry.Title)
			if err != nil {
				log.Printf("reseller instant-gaming %q: %v", entry.Title, err)
			}
			resultCh <- scrapeResult{entryID: entry.ID, title: entry.Title, price: price, store: "instant-gaming", err: err}
		}
	}()

	// ── loaded.com goroutine ────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for _, entry := range entries {
			<-ticker.C
			price, err := searchLoaded(client, entry.Title)
			if err != nil {
				log.Printf("reseller loaded %q: %v", entry.Title, err)
			}
			resultCh <- scrapeResult{entryID: entry.ID, title: entry.Title, price: price, store: "loaded", err: err}
		}
	}()

	// Close channel once both goroutines finish.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	states := make(map[string]*entryState, total)

	for r := range resultCh {
		s, ok := states[r.entryID]
		if !ok {
			s = &entryState{}
			states[r.entryID] = s
		}
		s.responses++

		if r.err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s %q: %v", r.store, r.title, r.err))
		} else if r.price > 0 && (s.bestPrice == 0 || r.price < s.bestPrice) {
			s.bestPrice = r.price
			s.bestStore = r.store
		}

		if s.responses == numSites {
			if s.bestPrice == 0 {
				result.NotFound++
			} else {
				if updated, err := store.UpdateWishlistBestPriceIfLower(r.entryID, s.bestPrice, s.bestStore); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("update %q: %v", r.title, err))
				} else if updated {
					result.Updated++
				} else {
					result.NotCheaper++
				}
			}
		}
	}

	prog(total, total)

	return result, nil
}

// ── instant-gaming.com ────────────────────────────────────────────────────────

// searchInstantGaming searches instant-gaming.com for the best current price
// of a game, using the title search page.
//
// The site embeds product data in the HTML as both JSON-LD and itemprop
// microdata. We try JSON-LD first (most reliable), then microdata, then a
// plain price-near-title scan.
func searchInstantGaming(client *http.Client, title string) (float64, error) {
	searchURL := "https://www.instant-gaming.com/en/search/?q=" + url.QueryEscape(title)
	body, err := fetchPage(client, searchURL)
	if err != nil {
		return 0, err
	}

	norm := normalizeTitle(title)

	// Strategy 1: JSON-LD structured data
	if price := extractJSONLDPrice(body, norm); price > 0 {
		return price, nil
	}

	// Strategy 2: itemprop microdata (itemprop="price" content="X.XX")
	if price := extractMicrodataPrice(body, norm); price > 0 {
		return price, nil
	}

	// Strategy 3: scan for price text near matching title text
	return extractPriceNearTitle(body, norm), nil
}

// ── loaded.com ────────────────────────────────────────────────────────────────

// searchLoaded searches loaded.com for the best current price of a game.
// It tries the Shopify search JSON endpoint first, falling back to HTML scraping.
func searchLoaded(client *http.Client, title string) (float64, error) {
	// Attempt Shopify search JSON endpoint (works on many Shopify stores).
	jsonURL := "https://www.loaded.com/search.json?type=product&q=" + url.QueryEscape(title)
	req, err := http.NewRequest("GET", jsonURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range browserHeaders {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		rawBody, _ := io.ReadAll(resp.Body)
		if price := parseShopifySearchJSON(rawBody, normalizeTitle(title)); price > 0 {
			return price, nil
		}
	} else if resp != nil {
		resp.Body.Close()
	}

	// Fall back to HTML search page.
	searchURL := "https://www.loaded.com/search?type=product&q=" + url.QueryEscape(title)
	body, err := fetchPage(client, searchURL)
	if err != nil {
		// loaded.com returns 404 when a search yields no results — not an error.
		if strings.Contains(err.Error(), "HTTP 404") {
			return 0, nil
		}
		return 0, err
	}
	norm := normalizeTitle(title)
	if price := extractJSONLDPrice(body, norm); price > 0 {
		return price, nil
	}
	return extractPriceNearTitle(body, norm), nil
}

// ── Shopify JSON parser ───────────────────────────────────────────────────────

type shopifySearchResult struct {
	Results []struct {
		Title string  `json:"title"`
		Price float64 `json:"price"` // Shopify may return cents (int) or dollars (float)
	} `json:"results"`
}

func parseShopifySearchJSON(body []byte, normTitle string) float64 {
	var parsed shopifySearchResult
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0
	}
	var best float64
	for _, r := range parsed.Results {
		if normalizeTitle(r.Title) != normTitle {
			continue
		}
		p := r.Price
		// Shopify sometimes returns prices in cents as large integers.
		if p > 10000 {
			p /= 100
		}
		if p > 0 && (best == 0 || p < best) {
			best = p
		}
	}
	return best
}

// ── JSON-LD parser ────────────────────────────────────────────────────────────

var (
	jsonLDBlockRe  = regexp.MustCompile(`(?s)<script[^>]+type=["']application/ld\+json["'][^>]*>(.*?)</script>`)
	jsonLDPriceRe  = regexp.MustCompile(`"price"\s*:\s*"?(\d+\.?\d*)"?`)
	jsonLDNameRe   = regexp.MustCompile(`"name"\s*:\s*"([^"]+)"`)
)

func extractJSONLDPrice(body []byte, normTitle string) float64 {
	matches := jsonLDBlockRe.FindAllSubmatch(body, -1)
	var best float64
	for _, m := range matches {
		block := m[1]
		nameM := jsonLDNameRe.FindSubmatch(block)
		if nameM == nil {
			continue
		}
		if normalizeTitle(string(nameM[1])) != normTitle {
			continue
		}
		priceM := jsonLDPriceRe.FindSubmatch(block)
		if priceM == nil {
			continue
		}
		p, err := strconv.ParseFloat(string(priceM[1]), 64)
		if err != nil || p <= 0 {
			continue
		}
		if best == 0 || p < best {
			best = p
		}
	}
	return best
}

// ── Microdata parser ──────────────────────────────────────────────────────────

// microdataBlockRe captures a stretch of HTML between itemtype="Product" and
// the next closing tag at the same structural level. We approximate by capturing
// up to 2000 bytes after the tag and then scanning for price microdata.
var (
	itemtypeRe     = regexp.MustCompile(`(?i)itemtype=["'][^"']*Product["']`)
	itempropNameRe = regexp.MustCompile(`(?i)itemprop=["']name["'][^>]*(?:content=["']([^"']+)["']|>([^<]+)<)`)
	itempropPriceRe= regexp.MustCompile(`(?i)itemprop=["']price["'][^>]*(?:content=["'](\d+\.?\d*)["']|>([^<]+)<)`)
)

func extractMicrodataPrice(body []byte, normTitle string) float64 {
	text := string(body)
	locs := itemtypeRe.FindAllStringIndex(text, -1)
	var best float64
	for _, loc := range locs {
		block := text[loc[0]:]
		if len(block) > 3000 {
			block = block[:3000]
		}
		nameM := itempropNameRe.FindStringSubmatch(block)
		if nameM == nil {
			continue
		}
		name := nameM[1]
		if name == "" {
			name = nameM[2]
		}
		if normalizeTitle(strings.TrimSpace(name)) != normTitle {
			continue
		}
		priceM := itempropPriceRe.FindStringSubmatch(block)
		if priceM == nil {
			continue
		}
		priceStr := priceM[1]
		if priceStr == "" {
			priceStr = strings.TrimSpace(priceM[2])
			// Strip currency symbols
			priceStr = strings.TrimLeft(priceStr, "$€£ \t")
		}
		p, err := strconv.ParseFloat(priceStr, 64)
		if err != nil || p <= 0 {
			continue
		}
		if best == 0 || p < best {
			best = p
		}
	}
	return best
}

// ── Title+price proximity scanner ────────────────────────────────────────────

// extractPriceNearTitle does a last-resort scan: strip HTML tags, look for the
// normalised title in the plain text, then find the next price within 200 chars.
var tagStripRe = regexp.MustCompile(`<[^>]+>`)

func extractPriceNearTitle(body []byte, normTitle string) float64 {
	plain := tagStripRe.ReplaceAllString(string(body), " ")
	plainNorm := strings.ToLower(plain)
	needle := strings.ReplaceAll(normTitle, " ", "")

	// Walk the plain text looking for a position where all words of the title
	// appear within a small window (simple containment check).
	words := strings.Fields(normTitle)
	if len(words) == 0 {
		return 0
	}

	var best float64
	searchIn := plainNorm
	offset := 0
	for {
		idx := strings.Index(searchIn[offset:], words[0])
		if idx < 0 {
			break
		}
		abs := offset + idx
		window := searchIn[abs:]
		if len(window) > 300 {
			window = window[:300]
		}
		// Verify all title words appear in this window.
		_ = needle
		allMatch := true
		for _, w := range words {
			if !strings.Contains(window, w) {
				allMatch = false
				break
			}
		}
		if allMatch {
			// Scan for the next price in this window.
			if pm := priceRe.FindStringSubmatch(window); pm != nil {
				p, err := strconv.ParseFloat(pm[1], 64)
				if err == nil && p > 0 && (best == 0 || p < best) {
					best = p
				}
			}
		}
		offset = abs + 1
		if offset >= len(searchIn) {
			break
		}
	}
	return best
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

func fetchPage(client *http.Client, rawURL string) ([]byte, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range browserHeaders {
		req.Header.Set(k, v)
	}
	for k, v := range chromeSecHeaders {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("HTTP %d (bot protection active — try from a residential IP)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MB cap
	return body, err
}
