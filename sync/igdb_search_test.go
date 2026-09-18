package sync

import "testing"

// The cases after "real library misses" are titles taken from the live review
// queue that IGDB's search returned *nothing* for, each paired with the query that
// finds the game. They are the reason this cleaning exists, so they are pinned.
func TestSearchTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		why      string
	}{
		// Undecorated titles must pass through untouched.
		{"Hollow Knight", "Hollow Knight", "untouched"},
		{"Baldur's Gate 3", "Baldur's Gate 3", "untouched"},
		{"Sid Meier's Civilization VI", "Sid Meier's Civilization VI", "no decoration"},

		// Trademark noise, as before.
		{"Batman™: Arkham Knight", "Batman: Arkham Knight", "trademark stripped"},
		{"RollerCoaster Tycoon® Deluxe", "RollerCoaster Tycoon", "trademark and edition"},
		{"  Spaced   Out  ", "Spaced Out", "whitespace collapsed"},

		// Real library misses.
		{"Batman: Arkham Asylum GOTY Edition", "Batman: Arkham Asylum", "real library miss"},
		{"Batman: Arkham City GOTY", "Batman: Arkham City", "real library miss"},
		{"Baldur's Gate: The Original Saga", "Baldur's Gate", "real library miss"},
		{"Astebreed: Definitive Edition", "Astebreed", "real library miss"},
		{"BloodNet (FDD version)", "BloodNet", "real library miss"},
		{"Bloodnet (CD version)", "Bloodnet", "real library miss"},
		{"Amerzone: The Explorer's Legacy (1999)", "Amerzone: The Explorer's Legacy", "year in parens"},
		{"Beneath a Steel Sky (1994)", "Beneath a Steel Sky", "year in parens"},
		{"Besiege + The Splintered Sea DLC", "Besiege", "bundle suffix"},
		{"BioShock Infinite Complete Edition", "BioShock Infinite", "edition suffix"},
		{"Battle Isle 2: Scenery CD - Titan's Legacy", "Battle Isle 2: Scenery CD - Titan's Legacy", "trailing text is part of the name"},
		{"Chicken Invaders 5: Christmas Edition", "Chicken Invaders 5: Christmas Edition", "a variant product, not an edition sticker"},
		{"Above Snakes", "Above Snakes", "untouched"},

		// Guards: cleaning must never empty or gut a title.
		{"Deluxe", "Deluxe", "would empty — kept"},
		{"GOTY", "GOTY", "would empty — kept"},
		{"(1999)", "(1999)", "would empty — kept"},
		{"A", "A", "too short to strip"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := searchTitle(tt.input); got != tt.expected {
				t.Errorf("searchTitle(%q) = %q, expected %q (%s)", tt.input, got, tt.expected, tt.why)
			}
		})
	}
}

// Scoring must still see the title exactly as stored, so a cleaned search cannot
// change which entry wins — only whether IGDB returned anything to score.
func TestBestMatchUsesRawTitle(t *testing.T) {
	results := []IGDBGame{
		{ID: 1, Name: "Batman: Arkham Asylum"},
		{ID: 2, Name: "Batman: Arkham Asylum - Game of the Year Edition"},
	}
	// The decorated stored title is not equal to either IGDB name, so nothing may
	// be accepted automatically — the review page offers it instead.
	if m := bestMatch("Batman: Arkham Asylum GOTY Edition", results); m != nil {
		t.Errorf("bestMatch accepted %q for a decorated title; it must stay strict", m.Name)
	}
	// The same list still matches exactly when the stored title is undecorated.
	if m := bestMatch("Batman: Arkham Asylum", results); m == nil || m.ID != 1 {
		t.Errorf("bestMatch failed an exact title: %v", m)
	}
}
