package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://endoflife.date/api/v1"

// Release is a single release cycle of a product. The *From fields are nullable
// in the API: a cycle can exist with no announced end date yet, and not every
// product publishes every phase.
type Release struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	ReleaseDate  string `json:"releaseDate"`
	IsLTS        bool   `json:"isLts"`
	IsMaintained bool   `json:"isMaintained"`

	EOLFrom  *string `json:"eolFrom"`
	IsEOL    bool    `json:"isEol"`
	EOASFrom *string `json:"eoasFrom"`
	IsEOAS   bool    `json:"isEoas"`
	EOESFrom *string `json:"eoesFrom"`
	IsEOES   bool    `json:"isEoes"`
}

// phase returns the date the given lifecycle phase ends and whether the product
// publishes the phase for this release. Whether that date has passed is not
// reported: a phase that is over is still owed an alert saying so, and the date
// itself answers the question.
func (r Release) phase(phase string) (date string, ok bool) {
	var from *string
	switch phase {
	case phaseEOL:
		from = r.EOLFrom
	case phaseEOAS:
		from = r.EOASFrom
	case phaseEOES:
		from = r.EOESFrom
	default:
		return "", false
	}

	if from == nil || *from == "" {
		return "", false
	}

	return *from, true
}

// Product is the endoflife.date representation of a tracked product.
type Product struct {
	Name string `json:"name"`
	// Label is the human-readable product name, e.g. "PostgreSQL".
	Label string `json:"label"`
	// Labels maps a lifecycle phase to the vendor's own wording for it, e.g.
	// {"eol": "Security Support", "eoes": "Extended Support"}.
	Labels   map[string]string `json:"labels"`
	Links    ProductLinks      `json:"links"`
	Releases []Release         `json:"releases"`
}

// ProductLinks holds the public endoflife.date page for a product, used to link
// the alert back to the source.
type ProductLinks struct {
	HTML string `json:"html"`
}

// phaseLabel returns the vendor's wording for a phase, falling back to a
// generic description when the product does not name it.
func (p *Product) phaseLabel(phase string) string {
	if label := p.Labels[phase]; label != "" {
		return label
	}

	switch phase {
	case phaseEOAS:
		return "Active Support"
	case phaseEOES:
		return "Extended Support"
	default:
		return "Support"
	}
}

type productResponse struct {
	Result Product `json:"result"`
}

// Client polls the endoflife.date API. BaseURL and HTTPClient are fields rather
// than constants, so tests can point it at a httptest server.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	// RetryDelay is the pause before the single retry attempt.
	RetryDelay time.Duration
}

// NewClient returns a Client with production defaults.
func NewClient() *Client {
	return &Client{
		BaseURL:    defaultBaseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		RetryDelay: 2 * time.Second,
	}
}

// FetchProduct retrieves a single product, retrying once on a transport error
// or a 5xx response. Client errors such as a 404 for an unknown slug are
// returned immediately, since retrying cannot help.
func (c *Client) FetchProduct(ctx context.Context, product string) (*Product, error) {
	var lastErr error

	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			logWarn("retrying %s after error: %v", product, lastErr)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("failed to fetch product %s: %w", product, ctx.Err())
			case <-time.After(c.RetryDelay):
			}
		}

		result, retryable, err := c.fetchOnce(ctx, product)
		if err == nil {
			return result, nil
		}

		lastErr = err
		if !retryable {
			break
		}
	}

	return nil, fmt.Errorf("failed to fetch product %s: %w", product, lastErr)
}

func (c *Client) fetchOnce(ctx context.Context, product string) (*Product, bool, error) {
	endpoint := fmt.Sprintf("%s/products/%s", c.BaseURL, url.PathEscape(product))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		retryable := resp.StatusCode >= http.StatusInternalServerError ||
			resp.StatusCode == http.StatusTooManyRequests

		return nil, retryable, fmt.Errorf("unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("failed to read response body: %w", err)
	}

	var parsed productResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, false, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(parsed.Result.Releases) == 0 {
		return nil, false, fmt.Errorf("response contains no releases")
	}

	return &parsed.Result, false, nil
}
