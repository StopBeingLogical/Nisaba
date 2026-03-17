package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// RAWGClient makes authenticated requests to the RAWG API.
type RAWGClient struct {
	apiKey string
	http   *http.Client
}

func NewRAWGClient(apiKey string) *RAWGClient {
	return &RAWGClient{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

type RAWGGame struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	BackgroundImage string `json:"background_image"`
	Released        string `json:"released"` // "YYYY-MM-DD"
	Genres          []struct {
		Name string `json:"name"`
	} `json:"genres"`
}

// SearchGame returns the best-matching RAWG result for the given title, or nil.
func (c *RAWGClient) SearchGame(title string) (*RAWGGame, error) {
	params := url.Values{
		"key":    {c.apiKey},
		"search": {title},
		"page_size": {"5"},
	}
	resp, err := c.http.Get("https://api.rawg.io/api/games?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("RAWG HTTP %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Results []RAWGGame `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	norm := normalizeTitle(title)
	for i := range result.Results {
		if normalizeTitle(result.Results[i].Name) == norm {
			return &result.Results[i], nil
		}
	}
	return nil, nil
}

func (g *RAWGGame) GenreNames() []string {
	names := make([]string, 0, len(g.Genres))
	for _, genre := range g.Genres {
		names = append(names, genre.Name)
	}
	return names
}
