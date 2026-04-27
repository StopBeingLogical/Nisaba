package sync

import (
	"testing"
)

func TestStripGameTitleSuffixes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Baldur's Gate 3", "Baldur's Gate 3"},
		{"Baldur's Gate 3 - Steam Key", "Baldur's Gate 3"},
		{"Baldur's Gate 3 Steam Key", "Baldur's Gate 3"},
		{"Baldur's Gate 3 (Steam Key)", "Baldur's Gate 3"},
		{"Elden Ring Global", "Elden Ring"},
		{"Elden Ring (Global)", "Elden Ring"},
		{"Elden Ring - Global", "Elden Ring"},
		{"The Witcher 3 PC", "The Witcher 3"},
		{"The Witcher 3 - PC", "The Witcher 3"},
		{"The Witcher 3 (PC)", "The Witcher 3"},
		{"Cyberpunk 2077 Region Free", "Cyberpunk 2077"},
		{"Cyberpunk 2077 (Region Free)", "Cyberpunk 2077"},
		{"Dark Souls III (ROW)", "Dark Souls III"},
		{"Dark Souls III ROW", "Dark Souls III"},
		{"Starfield Windows", "Starfield"},
		{"Starfield - Windows", "Starfield"},
		{"  Game Title  ", "Game Title"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := StripGameTitleSuffixes(tt.input)
			if result != tt.expected {
				t.Errorf("StripGameTitleSuffixes(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizePack(t *testing.T) {
	tests := []struct {
		siteID    string
		packTitle string
		expected  string
	}{
		{"g2a", "AAA Premium", "g2a-aaa-premium"},
		{"G2A", "AAA Premium", "g2a-aaa-premium"},
		{"k4g", "VIP Bundle 2024", "k4g-vip-bundle-2024"},
		{"kinguin", "Standard Pack", "kinguin-standard-pack"},
		{"eneba", "Gold Collection", "eneba-gold-collection"},
		{"fanatical", "Premium Selection", "fanatical-premium-selection"},
		{"g2a", "  Spaces   Between  ", "g2a-spaces-between"},
		{"g2a", "Special-Chars!@#$%", "g2a-specialchars"},
		{"g2a", "", "g2a"},
		{"g2a", "123 Numbers", "g2a-123-numbers"},
	}

	for _, tt := range tests {
		t.Run(tt.siteID+"-"+tt.packTitle, func(t *testing.T) {
			result := NormalizePack(tt.siteID, tt.packTitle)
			if result != tt.expected {
				t.Errorf("NormalizePack(%q, %q) = %q, expected %q", tt.siteID, tt.packTitle, result, tt.expected)
			}
		})
	}
}

func TestDescriptionSimilarity(t *testing.T) {
	tests := []struct {
		a        string
		b        string
		minScore float64 // minimum expected similarity
		maxScore float64 // maximum expected similarity
		desc     string
	}{
		{
			a:        "Bundle includes Baldur's Gate 3, Elden Ring, and Cyberpunk 2077",
			b:        "Bundle includes Baldur's Gate 3, Elden Ring, and Cyberpunk 2077",
			minScore: 1.0,
			maxScore: 1.0,
			desc:     "identical strings",
		},
		{
			a:        "",
			b:        "",
			minScore: 1.0,
			maxScore: 1.0,
			desc:     "both empty",
		},
		{
			a:        "AAA Premium Pack contains top-rated games",
			b:        "AAA Premium Pack contains top-rated games from major studios",
			minScore: 0.6,
			maxScore: 0.9,
			desc:     "high similarity with minor additions",
		},
		{
			a:        "Old description about games",
			b:        "Completely different new description",
			minScore: 0.0,
			maxScore: 0.3,
			desc:     "very low similarity",
		},
		{
			a:        "Description A",
			b:        "Description B",
			minScore: 0.25, // "Description" matches, 1 intersection / 4 union
			maxScore: 0.6,
			desc:     "one word overlap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := DescriptionSimilarity(tt.a, tt.b)
			if result < tt.minScore || result > tt.maxScore {
				t.Errorf("DescriptionSimilarity(%q, %q) = %.2f, expected range [%.2f, %.2f]",
					tt.a, tt.b, result, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestMatchGameTitle(t *testing.T) {
	// Create sample index maps (keys should be normalized, i.e., articles stripped)
	ownedIndex := map[string]struct{}{
		"baldurs gate 3":      {},
		"elden ring":          {},
		"witcher 3":           {}, // normalizeTitle strips "the"
		"cyberpunk 2077":      {},
		"dark souls iii":      {},
	}

	wishlistIndex := map[string]struct{}{
		"starfield":           {},
		"final fantasy xvi":   {},
	}

	tests := []struct {
		title       string
		expectMatch bool
		desc        string
	}{
		{"Baldur's Gate 3", true, "exact match in owned"},
		{"Baldur's Gate 3 - Steam Key", true, "with suffix, owned"},
		{"Elden Ring", true, "exact match in owned"},
		{"Elden Ring (Global)", true, "with suffix, owned"},
		{"Starfield", true, "exact match in wishlist"},
		{"Starfield (PC)", true, "with suffix, wishlist"},
		{"Unknown Game", false, "no match"},
		{"Unknown Game - Steam Key", false, "no match even with suffix"},
		{"The Witcher 3", true, "exact match with article"},
		{"", false, "empty string"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := MatchGameTitle(tt.title, ownedIndex, wishlistIndex)
			matched := result != ""
			if matched != tt.expectMatch {
				t.Errorf("MatchGameTitle(%q) matched=%v, expected=%v (result=%q)",
					tt.title, matched, tt.expectMatch, result)
			}
		})
	}
}

func TestMatchGameTitlePreferenceOrder(t *testing.T) {
	// Verify that owned is checked before wishlist
	ownedIndex := map[string]struct{}{
		"shared game": {},
	}

	wishlistIndex := map[string]struct{}{
		"shared game": {},
	}

	result := MatchGameTitle("Shared Game", ownedIndex, wishlistIndex)
	if result != "shared game" {
		t.Errorf("MatchGameTitle should find in owned index, got %q", result)
	}
}
