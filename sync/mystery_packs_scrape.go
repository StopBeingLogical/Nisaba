package sync

import (
	"strings"
	"unicode"
)

// Known suffixes to strip from scraped game titles before matching.
var titleSuffixStrip = []string{
	" - Steam Key", " Steam Key", " (Steam Key)",
	" - Global", " Global", " (Global)",
	" - PC", " PC", " (PC)",
	" - Region Free", " Region Free", " (Region Free)",
	" - ROW", " (ROW)", " ROW",
	" - Windows", " Windows", " (Windows)",
}

// StripGameTitleSuffixes removes known platform/region suffixes from a scraped game title.
func StripGameTitleSuffixes(title string) string {
	title = strings.TrimSpace(title)
	for _, suffix := range titleSuffixStrip {
		if strings.HasSuffix(strings.ToLower(title), strings.ToLower(suffix)) {
			title = title[:len(title)-len(suffix)]
			title = strings.TrimSpace(title)
			break
		}
	}
	return title
}

// MatchGameTitle attempts to match a game title against provided collections.
// Returns the normalized match key if found, "" if not found.
// matchSets are maps from normalized title → struct{} (like ListOwnedTitleIndex results).
func MatchGameTitle(title string, matchSets ...map[string]struct{}) string {
	cleaned := StripGameTitleSuffixes(title)
	normalized := normalizeTitle(cleaned)

	for _, set := range matchSets {
		if _, found := set[normalized]; found {
			return normalized
		}
	}
	return ""
}

// DescriptionSimilarity computes a 0.0–1.0 Jaccard similarity score between two text blocks.
// Returns 1.0 if both are empty, 0.0 if no word overlap.
func DescriptionSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}

	wordsA := tokenize(a)
	wordsB := tokenize(b)

	if len(wordsA) == 0 && len(wordsB) == 0 {
		return 1.0
	}

	aMap := make(map[string]struct{})
	for _, w := range wordsA {
		aMap[w] = struct{}{}
	}

	bMap := make(map[string]struct{})
	for _, w := range wordsB {
		bMap[w] = struct{}{}
	}

	// Count intersection
	intersection := 0
	for w := range aMap {
		if _, found := bMap[w]; found {
			intersection++
		}
	}

	// Count union
	union := len(aMap) + len(bMap) - intersection

	if union == 0 {
		return 1.0
	}

	return float64(intersection) / float64(union)
}

// tokenize splits text into normalized words (lowercase, alphanumeric only).
func tokenize(s string) []string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Keep only letters, digits, spaces
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			return r
		}
		return -1
	}, s)

	// Split and filter empty
	words := strings.Fields(s)
	var result []string
	for _, w := range words {
		if len(w) > 0 {
			result = append(result, w)
		}
	}
	return result
}

// NormalizePack returns a URL-safe pack ID slug: {site_id}-{title_slug}
// Example: "G2A", "AAA Premium" → "g2a-aaa-premium"
func NormalizePack(siteID, packTitle string) string {
	siteID = strings.ToLower(strings.TrimSpace(siteID))
	title := packTitle

	// Strip punctuation and convert to lowercase
	title = strings.ToLower(title)
	title = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			return r
		}
		return -1
	}, title)

	// Collapse whitespace to single spaces
	title = strings.Join(strings.Fields(title), " ")

	// Replace spaces with dashes
	title = strings.ReplaceAll(title, " ", "-")

	// Combine with site_id
	if len(title) == 0 {
		return siteID
	}
	return siteID + "-" + title
}
