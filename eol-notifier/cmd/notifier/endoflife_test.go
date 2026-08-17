package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureServer serves captured endoflife.date responses from testdata, so no
// test reaches the real API.
func fixtureServer(t *testing.T, requests *int) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			*requests++
		}

		product := filepath.Base(r.URL.Path)
		body, err := os.ReadFile(filepath.Join("testdata", product+".json"))
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	return server
}

func testClient(baseURL string) *Client {
	return &Client{BaseURL: baseURL, HTTPClient: http.DefaultClient}
}

func TestFetchProductDecodesNullableFields(t *testing.T) {
	server := fixtureServer(t, nil)
	client := testClient(server.URL)

	tests := []struct {
		name       string
		product    string
		release    string
		phase      string
		wantOK     bool
		wantDate   string
		phaseLabel string
	}{
		{
			name:       "plain end of life date",
			product:    "postgresql",
			release:    "18",
			phase:      phaseEOL,
			wantOK:     true,
			wantDate:   "2030-11-14",
			phaseLabel: "Support Status",
		},
		{
			name:       "extended support date",
			product:    "amazon-rds-postgresql",
			release:    "18",
			phase:      phaseEOES,
			wantOK:     true,
			wantDate:   "2034-02-28",
			phaseLabel: "Extended Support",
		},
		{
			name:       "null end of life date is reported as absent",
			product:    "redis",
			release:    "8.8",
			phase:      phaseEOL,
			wantOK:     false,
			phaseLabel: "Security Support",
		},
		{
			name:       "elapsed phase still reports its date",
			product:    "redis",
			release:    "8.2",
			phase:      phaseEOL,
			wantOK:     true,
			wantDate:   "2026-05-25",
			phaseLabel: "Security Support",
		},
		{
			name:       "product without the phase reports it as absent",
			product:    "postgresql",
			release:    "18",
			phase:      phaseEOES,
			wantOK:     false,
			phaseLabel: "Extended Support",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			product, err := client.FetchProduct(context.Background(), tt.product)
			if err != nil {
				t.Fatalf("FetchProduct(%q) error = %v", tt.product, err)
			}

			var release Release
			for _, candidate := range product.Releases {
				if candidate.Name == tt.release {
					release = candidate
				}
			}
			if release.Name == "" {
				t.Fatalf("fixture %s has no release %s", tt.product, tt.release)
			}

			date, ok := release.phase(tt.phase)
			if ok != tt.wantOK {
				t.Fatalf("phase(%q) ok = %v, want %v", tt.phase, ok, tt.wantOK)
			}
			if ok && date != tt.wantDate {
				t.Errorf("phase(%q) date = %q, want %q", tt.phase, date, tt.wantDate)
			}
			if got := product.phaseLabel(tt.phase); got != tt.phaseLabel {
				t.Errorf("phaseLabel(%q) = %q, want %q", tt.phase, got, tt.phaseLabel)
			}
		})
	}
}

func TestFetchProductRetriesServerErrors(t *testing.T) {
	tests := []struct {
		name         string
		statuses     []int
		wantErr      bool
		wantRequests int
	}{
		{name: "succeeds on the retry", statuses: []int{http.StatusBadGateway}, wantRequests: 2},
		{name: "retries a rate limit", statuses: []int{http.StatusTooManyRequests}, wantRequests: 2},
		{name: "gives up after one retry", statuses: []int{http.StatusBadGateway, http.StatusBadGateway}, wantErr: true, wantRequests: 2},
		{name: "does not retry a client error", statuses: []int{http.StatusNotFound}, wantErr: true, wantRequests: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("testdata", "postgresql.json"))
			if err != nil {
				t.Fatalf("failed to read fixture: %v", err)
			}

			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if requests < len(tt.statuses) {
					w.WriteHeader(tt.statuses[requests])
					requests++
					return
				}
				requests++
				_, _ = w.Write(body)
			}))
			defer server.Close()

			client := testClient(server.URL)
			_, err = client.FetchProduct(context.Background(), "postgresql")

			if (err != nil) != tt.wantErr {
				t.Fatalf("FetchProduct() error = %v, wantErr %v", err, tt.wantErr)
			}
			if requests != tt.wantRequests {
				t.Errorf("made %d request(s), want %d", requests, tt.wantRequests)
			}
		})
	}
}

func TestFetchProductRejectsUnusableResponses(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "malformed json", body: "{", wantErr: "failed to decode response"},
		{name: "no releases", body: `{"result":{"name":"redis","releases":[]}}`, wantErr: "no releases"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			_, err := testClient(server.URL).FetchProduct(context.Background(), "redis")
			if err == nil {
				t.Fatal("expected an error for an unusable response")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestFetchProductsPollsEachProductOnce(t *testing.T) {
	requests := 0
	server := fixtureServer(t, &requests)

	config := testConfig(t,
		Dependency{Name: "Redis", Product: "redis"},
		Dependency{Name: "PostgreSQL", Product: "postgresql"},
		Dependency{Name: "GCP MemoryStore", Product: "redis", UpstreamProxy: true},
		Dependency{Name: "Nonexistent", Product: "does-not-exist"},
	)

	products, failed := fetchProducts(context.Background(), testClient(server.URL), productOrder(config))

	if requests != 3 {
		t.Errorf("made %d request(s), want 3 (redis fetched once for two dependencies)", requests)
	}
	if len(products) != 2 {
		t.Errorf("fetched %d product(s), want 2", len(products))
	}
	if len(failed) != 1 || failed[0] != "does-not-exist" {
		t.Errorf("failed = %v, want [does-not-exist]", failed)
	}
}
