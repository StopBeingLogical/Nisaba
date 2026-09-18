package sync

import (
	"strings"
	"testing"
)

// These pin the search *mechanism*, not just its cleaning. The review page's
// search box returned the wrong game for "Against the Storm" because IGDB's
// ranked search put the correct entry at position 10 of 30 and the old query
// asked for 5 — so the tests below assert that a name-anchored lookup is part of
// the search at all, and that it is asked for more than a handful of rows.
func TestSearchForms(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		want    []string
		comment string
	}{
		{
			name:  "plain title is searched as itself only",
			title: "Against the Storm",
			want:  []string{"Against the Storm"},
		},
		{
			name:  "subtitle is retried on its head",
			title: "Fallout 2: A Post Nuclear Role Playing Game",
			want:  []string{"Fallout 2: A Post Nuclear Role Playing Game", "Fallout 2"},
		},
		{
			name:  "dash subtitle is retried on its head",
			title: "Earth 2150 - Escape from the Blue Planet",
			want:  []string{"Earth 2150 - Escape from the Blue Planet", "Earth 2150"},
		},
		{
			// Cleaning strips the edition first, and the remaining head is the
			// single word "Batman", which is too broad to search on its own.
			name:  "decorated title is cleaned, and its one-word head is refused",
			title: "Batman: Arkham Asylum GOTY Edition",
			want:  []string{"Batman: Arkham Asylum"},
		},
		{
			// Cutting at the last separator keeps the game's own name; cutting at
			// the first would leave the franchise prefix "Warhammer 40,000", for
			// which the best available match is Dawn of War — the wrong game.
			name:  "front decoration and a trailing one keep the game name",
			title: "Warhammer 40,000: Dawn of War II - Anniversary Edition",
			want: []string{
				"Warhammer 40,000: Dawn of War II - Anniversary Edition",
				"Warhammer 40,000: Dawn of War II",
				"Warhammer 40,000",
			},
		},
		{
			name:  "nested subtitles fall back from narrow to broad",
			title: "Hotline Miami 2: Wrong Number - Digital Comics",
			want: []string{
				"Hotline Miami 2: Wrong Number - Digital Comics",
				"Hotline Miami 2: Wrong Number",
				"Hotline Miami 2",
			},
		},
		{
			name:  "no separator means no fallback form",
			title: "Hollow Knight",
			want:  []string{"Hollow Knight"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := searchForms(tc.title)
			if len(got) != len(tc.want) {
				t.Fatalf("searchForms(%q) = %q, want %q", tc.title, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("searchForms(%q)[%d] = %q, want %q", tc.title, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// A one-word head is too broad to be worth searching — "Batman" would anchor
// against every Batman entry IGDB has — so it is rejected rather than used.
func TestHeadSearchTitleRejectsBroadHeads(t *testing.T) {
	tests := []struct {
		in    string
		last  string
		first string
		why   string
	}{
		{"Batman: Arkham Asylum", "", "", "one word is too broad to anchor on"},
		{"Falcon: something", "", "", "one word is too broad to anchor on"},
		{"Fallout 2: A Post Nuclear Role Playing Game", "Fallout 2", "Fallout 2", "one separator, so both heads agree"},
		{"Earth 2150 - Escape from the Blue Planet", "Earth 2150", "Earth 2150", "dash subtitle"},
		{"Warhammer 40,000: Dawn of War II - Anniversary Edition", "Warhammer 40,000: Dawn of War II", "Warhammer 40,000", "last keeps the game, first keeps only the franchise"},
		{"No Separator Here", "", "", "nothing to drop"},
		{": leading separator", "", "", "an empty head is not a query"},
	}
	for _, tc := range tests {
		if got := headSearchTitle(tc.in); got != tc.last {
			t.Errorf("headSearchTitle(%q) = %q, want %q (%s)", tc.in, got, tc.last, tc.why)
		}
		if got := broadHeadSearchTitle(tc.in); got != tc.first {
			t.Errorf("broadHeadSearchTitle(%q) = %q, want %q (%s)", tc.in, got, tc.first, tc.why)
		}
		for _, form := range searchForms(tc.in) {
			if len(strings.Fields(form)) < 2 && form != tc.in {
				t.Errorf("searchForms(%q) fell back to the one-word query %q", tc.in, form)
			}
		}
	}
}

// The whole point of the anchored form is that it does not depend on IGDB's
// ranking, so it must survive a title whose words appear all over IGDB's
// summaries. It also has to be asked for far more than five rows.
func TestAnchoredBody(t *testing.T) {
	body := anchoredBody("Against the Storm")
	if !strings.Contains(body, `where name ~ *"Against the Storm"*`) {
		t.Errorf("anchored body does not look the title up by name: %s", body)
	}
	if !strings.Contains(body, "sort name asc") {
		t.Errorf("anchored body is unsorted, so an exact entry can fall outside the limit: %s", body)
	}
	if maxAnchoredResults <= 5 {
		t.Errorf("maxAnchoredResults = %d: too small to hold an exact entry", maxAnchoredResults)
	}
	if !strings.Contains(body, "limit 25") {
		t.Errorf("anchored body limit is not maxAnchoredResults: %s", body)
	}
}

func TestRankedBodyIsWideEnough(t *testing.T) {
	body := rankedBody("Against the Storm")
	if !strings.Contains(body, "search ") {
		t.Errorf("ranked body is not a search: %s", body)
	}
	// Measured: the correct entry sat at position 14. A limit of 5 is the bug.
	if maxRankedResults < 15 {
		t.Errorf("maxRankedResults = %d: the exact entry sits past this point", maxRankedResults)
	}
}

// A title carrying quotes or wildcards must not be able to break out of the
// literal it is placed in.
func TestWildcardLiteralStripsDirectives(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`Against the Storm`, "Against the Storm"},
		{`Bad"name*`, "Badname"},
		{`back\slash`, "backslash"},
		{`100%`, "100"},
		{`  spaced   out  `, "spaced out"},
	}
	for _, tc := range tests {
		if got := wildcardLiteral(tc.in); got != tc.want {
			t.Errorf("wildcardLiteral(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// A body built from a hostile title stays a single lookup.
	body := anchoredBody(`x"*; drop; "\`)
	if strings.Count(body, `~ *"`) != 1 {
		t.Errorf("hostile title broke out of the literal: %s", body)
	}
}

func TestAppendUniqueDropsDuplicates(t *testing.T) {
	seen := map[int64]bool{}
	first := []IGDBGame{{ID: 1, Name: "Against the Storm"}}
	second := []IGDBGame{{ID: 1, Name: "Against the Storm"}, {ID: 2, Name: "Keepers"}}
	got := appendUnique(appendUnique(nil, seen, first), seen, second)
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("appendUnique = %+v, want ids 1 then 2", got)
	}
}

// The search box shows its hits in this order, so the entry that *is* the typed
// title has to come first whatever order IGDB returned.
func TestRankSearchResultsPutsExactFirst(t *testing.T) {
	games := []IGDBGame{
		{ID: 1, Name: "Life is Strange: Before the Storm - Episode 1: Awake"},
		{ID: 2, Name: "Metal Storm"},
		{ID: 3, Name: "Against the Storm"},
		{ID: 4, Name: "Against the Storm: Nightwatchers"},
	}
	got := RankSearchResults("Against the Storm", games)
	if len(got) != 4 {
		t.Fatalf("RankSearchResults returned %d rows, want 4", len(got))
	}
	if got[0].ID != 3 {
		t.Errorf("first hit is %q (id %d), want the exact title", got[0].Name, got[0].ID)
	}
	// The DLC shares a prefix, nothing else resembles the title at all.
	if got[1].ID != 4 {
		t.Errorf("second hit is %q (id %d), want the prefix match", got[1].Name, got[1].ID)
	}
	if got[3].ID != 2 && got[3].ID != 1 {
		t.Errorf("unrelated hits should sink: got %v", []int64{got[2].ID, got[3].ID})
	}
}

// Stores concatenate words IGDB spaces out. Before this rule the correct entry
// scored zero and was discarded, which is why "Dragonview" found nothing.
func TestScoreMatchIgnoresSpacing(t *testing.T) {
	tests := []struct {
		stored string
		igdb   string
		want   string
	}{
		{"Dragonview", "Dragon View", "exact"},
		{"Half-Life", "Half Life", "exact"},
		{"Against the Storm", "Against the Storm", "exact"},
		{"Dragonview", "Dragon View II", "none"},
	}
	for _, tc := range tests {
		score, label := scoreMatch(tc.stored, tc.igdb)
		if tc.want == "none" {
			if score >= 0.999 {
				t.Errorf("scoreMatch(%q, %q) = %v/%s, want no exact match", tc.stored, tc.igdb, score, label)
			}
			continue
		}
		if label != tc.want || score < 0.999 {
			t.Errorf("scoreMatch(%q, %q) = %v/%s, want %s", tc.stored, tc.igdb, score, label, tc.want)
		}
	}
}
