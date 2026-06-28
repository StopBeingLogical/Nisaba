package sync

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"nisaba/db"
)

type MysteryPackAnalysisResult struct {
	PackID            string
	PackPriceUSD      float64
	PoolSize          int
	OverlapCount      int
	NewGamesCount     int
	KeyshopValueTotal float64
	KeyshopValueNew   float64
	ROIKeyshop        float64
	ROIPerKey         float64
	VarianceScore     int
	Recommendation    string
	AnalyzedAt        string
	OverlapTitles     []string
	NotableGames      []string
	Errors            []string
}

// AnalyzeSetListPack analyzes a mystery pack with a disclosed game list.
// It fetches gg.deals keyshop prices for each game, cross-references against
// the owned library, and computes ROI and variance metrics.
func AnalyzeSetListPack(
	client *http.Client,
	apiKey string,
	pack *db.MysteryPackDetail,
	ownedIndex map[string]struct{},
	progress func(done, total int),
) (MysteryPackAnalysisResult, error) {
	var result MysteryPackAnalysisResult
	result.PackID = pack.ID
	if pack.PriceUSD != nil {
		result.PackPriceUSD = *pack.PriceUSD
	}

	if len(pack.Games) == 0 {
		result.Recommendation = "SKIP"
		result.VarianceScore = 1
		return result, nil
	}

	result.PoolSize = len(pack.Games)

	// Collect Steam App IDs for gg.deals lookup
	appIDs := []string{}
	gamesByAppID := make(map[string]*db.MysteryPackGame)
	for i := range pack.Games {
		g := &pack.Games[i]
		if g.SteamAppID != nil && *g.SteamAppID != "" {
			appIDs = append(appIDs, *g.SteamAppID)
			gamesByAppID[*g.SteamAppID] = g
		}
	}

	// Fetch keyshop prices from gg.deals
	if len(appIDs) > 0 {
		prices, err := ggDealsFetch(client, apiKey, appIDs)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("ggdeals fetch: %v", err))
		} else {
			for appID, game := range gamesByAppID {
				if p, found := prices[appID]; found && p != nil {
					keyshopPrice := parseGGPrice(p.Prices.CurrentKeyshops)
					if keyshopPrice > 0 {
						game.KeyshopPriceUSD = &keyshopPrice
						result.KeyshopValueTotal += keyshopPrice

						// Check if owned
						normTitle := normalizeTitle(game.Title)
						if _, isOwned := ownedIndex[normTitle]; !isOwned {
							result.KeyshopValueNew += keyshopPrice
							result.NewGamesCount++
							result.NotableGames = append(result.NotableGames, game.Title)
						} else {
							result.OverlapCount++
							result.OverlapTitles = append(result.OverlapTitles, game.Title)
						}
					}
				}
			}
		}
		if progress != nil {
			progress(len(appIDs), len(appIDs))
		}
	}

	// Calculate metrics
	if result.KeyshopValueNew > 0 && result.PackPriceUSD > 0 {
		result.ROIKeyshop = result.KeyshopValueNew / result.PackPriceUSD
		result.ROIPerKey = result.KeyshopValueNew / float64(pack.KeyCount)
	}

	// Cap notable games to top 5
	if len(result.NotableGames) > 5 {
		result.NotableGames = result.NotableGames[:5]
	}

	// Variance score: larger pools have lower variance
	result.VarianceScore = varianceScore(result.PoolSize, len(result.NotableGames))

	// Recommendation
	result.Recommendation = getRecommendation(result.ROIKeyshop)

	return result, nil
}

// AnalyzeMinValuePack analyzes a mystery pack with minimum-value guarantees.
// Since no game list is disclosed, analysis is based on the value_spec JSON.
func AnalyzeMinValuePack(pack *db.MysteryPackDetail) MysteryPackAnalysisResult {
	var result MysteryPackAnalysisResult
	result.PackID = pack.ID
	if pack.PriceUSD != nil {
		result.PackPriceUSD = *pack.PriceUSD
	}
	result.VarianceScore = 5 // unknown games = high variance
	result.Recommendation = "SKIP"

	// For min_value packs, we can estimate based on value_spec JSON.
	// Parse if present, but conservatively estimate ROI.
	var valueSpec map[string]interface{}
	if pack.ValueSpec != "" {
		if err := json.Unmarshal([]byte(pack.ValueSpec), &valueSpec); err == nil {
			// Rough estimate: assume minimum value is conservative
			// This is speculative — recommend "SKIP" unless very high value.
			if minVal, ok := valueSpec["min_value"].(float64); ok && result.PackPriceUSD > 0 {
				roi := minVal / result.PackPriceUSD
				if roi > 5.0 {
					result.Recommendation = "CONSIDER"
				}
			}
		}
	}

	return result
}

// varianceScore returns a 1–5 variance score based on pool size.
// Larger pools = lower variance; smaller pools = higher variance.
func varianceScore(poolSize, newGamesCount int) int {
	if poolSize >= 50 {
		return 1 // very stable
	}
	if poolSize >= 35 {
		return 2
	}
	if poolSize >= 20 {
		return 3
	}
	if poolSize >= 10 {
		return 4
	}
	return 5 // very high variance
}

// getRecommendation returns a recommendation badge based on ROI.
func getRecommendation(roi float64) string {
	if roi >= 12.0 {
		return "STRONG BUY"
	}
	if roi >= 8.0 {
		return "BUY"
	}
	if roi >= 4.0 {
		return "CONSIDER"
	}
	if roi >= 1.0 {
		return "SKIP"
	}
	return "AVOID"
}

// SyncMysteryPacksResult summarizes a full mystery packs sync run.
type SyncMysteryPacksResult struct {
	Analyzed int
	Errors   []string
}

// SyncMysteryPacks analyzes all enabled mystery packs and persists results to the database.
// progress is called with (done, total) to track progress; may be nil.
func SyncMysteryPacks(
	store *db.Store,
	apiKey string,
	progress func(done, total int),
) (SyncMysteryPacksResult, error) {
	var result SyncMysteryPacksResult

	packs, err := store.ListMysteryPacks()
	if err != nil {
		return result, fmt.Errorf("list packs: %w", err)
	}

	enabledPacks := []db.MysteryPackRow{}
	for _, p := range packs {
		if p.Enabled {
			enabledPacks = append(enabledPacks, p)
		}
	}

	if len(enabledPacks) == 0 {
		return result, nil
	}

	// Pre-fetch owned title index for all set_list packs
	ownedIndex, err := store.ListOwnedTitleIndex()
	if err != nil {
		return result, fmt.Errorf("fetch owned titles: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	total := len(enabledPacks)

	for i, packRow := range enabledPacks {
		// Fetch full pack detail
		pack, err := store.GetMysteryPack(packRow.ID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("pack %s: %v", packRow.ID, err))
			if progress != nil {
				progress(i+1, total)
			}
			continue
		}
		if pack == nil {
			if progress != nil {
				progress(i+1, total)
			}
			continue
		}

		// Analyze based on pack type
		var analysisResult MysteryPackAnalysisResult
		switch pack.PackType {
		case "set_list":
			analysisResult, _ = AnalyzeSetListPack(client, apiKey, pack, ownedIndex, nil)
		case "min_value":
			analysisResult = AnalyzeMinValuePack(pack)
		default:
			result.Errors = append(result.Errors, fmt.Sprintf("pack %s: unknown type %s", packRow.ID, pack.PackType))
			if progress != nil {
				progress(i+1, total)
			}
			continue
		}

		// Save analysis to database
		params := db.MysteryPackAnalysisParams{
			PackID:            pack.ID,
			AnalyzedAt:        time.Now().UTC().Format(time.RFC3339),
			PackPriceUSD:      &analysisResult.PackPriceUSD,
			PoolSize:          &analysisResult.PoolSize,
			OverlapCount:      &analysisResult.OverlapCount,
			NewGamesCount:     &analysisResult.NewGamesCount,
			KeyshopValueTotal: &analysisResult.KeyshopValueTotal,
			KeyshopValueNew:   &analysisResult.KeyshopValueNew,
			ROIKeyshop:        &analysisResult.ROIKeyshop,
			ROIPerKey:         &analysisResult.ROIPerKey,
			VarianceScore:     &analysisResult.VarianceScore,
			Recommendation:    &analysisResult.Recommendation,
			OverlapTitles:     analysisResult.OverlapTitles,
			NotableGames:      analysisResult.NotableGames,
		}

		if err := store.SaveMysteryPackAnalysis(params); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("save %s: %v", pack.ID, err))
		} else {
			result.Analyzed++
		}

		if progress != nil {
			progress(i+1, total)
		}
	}

	return result, nil
}
