package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

// captureServer stands in for the Slack webhook and records what was posted.
type captureServer struct {
	*httptest.Server
	requests int
	body     []byte
	header   http.Header
	status   int
}

func newCaptureServer(t *testing.T, status int) *captureServer {
	t.Helper()

	capture := &captureServer{status: status}
	capture.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.requests++
		capture.header = r.Header.Clone()
		capture.body, _ = io.ReadAll(r.Body)

		w.WriteHeader(capture.status)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(capture.Close)

	return capture
}

func (c *captureServer) client() *slackClient {
	return &slackClient{WebhookURL: c.URL, HTTPClient: http.DefaultClient}
}

func testReport(t *testing.T) Report {
	t.Helper()

	return Report{
		NewVersions: []NewVersionAlert{{
			Product:      "postgresql",
			ProductLabel: "PostgreSQL",
			ProductURL:   "https://endoflife.date/postgresql",
			Release:      "18",
			ReleaseDate:  "2025-09-25",
			Dependencies: []DependencyRef{
				{Name: "PostgreSQL"},
				{Name: "GCP Cloud SQL", UpstreamProxy: true},
			},
		}},
		EOL: []EOLAlert{
			{
				Product:      "postgresql",
				ProductLabel: "PostgreSQL",
				ProductURL:   "https://endoflife.date/postgresql",
				Release:      "14",
				Phase:        phaseEOL,
				PhaseLabel:   "Support Status",
				Date:         mustDate(t, "2026-11-12"),
				MonthsLeft:   12,
				Dependencies: []DependencyRef{{Name: "PostgreSQL"}},
			},
			{
				Product:      "redis",
				ProductLabel: "Redis",
				ProductURL:   "https://endoflife.date/redis",
				Release:      "7.2",
				Phase:        phaseEOL,
				PhaseLabel:   "Security Support",
				Date:         mustDate(t, "2026-02-28"),
				MonthsLeft:   1,
				Dependencies: []DependencyRef{{Name: "Redis"}, {Name: "GCP MemoryStore", UpstreamProxy: true}},
			},
		},
	}
}

// render builds the digest and flattens it, for the assertions that do not care
// whether it had to be truncated.
func render(report Report) string {
	message, _ := buildMessage(report)

	return blockText(message)
}

// blockText flattens the rendered message so assertions can look for content
// without depending on how it was split into blocks.
func blockText(message slackMessage) string {
	var builder strings.Builder

	for _, block := range message.Blocks {
		if block.Text != nil {
			builder.WriteString(block.Text.Text + "\n")
		}
		for _, element := range block.Elements {
			builder.WriteString(element.Text + "\n")
		}
	}

	return builder.String()
}

func TestBuildMessage(t *testing.T) {
	message, truncated := buildMessage(testReport(t))
	rendered := blockText(message)

	if truncated {
		t.Error("a two-entry digest was reported as truncated")
	}

	wants := []string{
		"Dependency lifecycle digest",
		"New versions detected",
		"<https://endoflife.date/postgresql|PostgreSQL>",
		"released 2025-09-25",
		"Approaching end of life",
		"*In 12 months*",
		"*In 1 month*",
		"Support Status ends 2026-11-12",
		"Security Support ends 2026-02-28",
		"affects Redis, GCP MemoryStore _(upstream proxy)_",
		"must be confirmed with the cloud provider",
		"manual PR against `TykTechnologies/github-actions`",
	}

	for _, want := range wants {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered message is missing %q:\n%s", want, rendered)
		}
	}

	if message.Blocks[0].Type != "header" {
		t.Errorf("first block = %q, want header", message.Blocks[0].Type)
	}
	if !strings.Contains(message.Text, "1 new version") {
		t.Errorf("fallback text = %q, want a summary", message.Text)
	}
}

func TestBuildMessageOmitsProxyCaveatWhenUnused(t *testing.T) {
	report := Report{EOL: []EOLAlert{{
		ProductLabel: "PostgreSQL",
		Release:      "14",
		PhaseLabel:   "Support Status",
		Date:         mustDate(t, "2026-11-12"),
		MonthsLeft:   6,
		Dependencies: []DependencyRef{{Name: "PostgreSQL"}},
	}}}

	if rendered := render(report); strings.Contains(rendered, "upstream proxy") {
		t.Errorf("proxy caveat rendered for a report with no proxies:\n%s", rendered)
	}
}

func TestBuildMessageExplainsSeedRun(t *testing.T) {
	report := testReport(t)
	report.Seed = true

	if rendered := render(report); !strings.Contains(rendered, "First run") {
		t.Errorf("seed run is not explained:\n%s", rendered)
	}
}

func TestBuildMessageSplitsLongDigests(t *testing.T) {
	var report Report
	for i := 0; i < 120; i++ {
		report.EOL = append(report.EOL, EOLAlert{
			ProductLabel: "PostgreSQL",
			Release:      "14",
			PhaseLabel:   "Support Status",
			Date:         mustDate(t, "2026-11-12"),
			MonthsLeft:   6,
			Dependencies: []DependencyRef{{Name: strings.Repeat("x", 60)}},
		})
	}

	message, _ := buildMessage(report)

	sections := 0
	for _, block := range message.Blocks {
		if block.Type == "section" {
			sections++
		}
		if block.Text != nil && len(block.Text.Text) > 3000 {
			t.Fatalf("section of %d characters exceeds the Slack limit", len(block.Text.Text))
		}
	}

	if sections < 2 {
		t.Errorf("got %d section(s), want the digest split across several", sections)
	}
	if len(message.Blocks) > slackMaxBlocks {
		t.Errorf("message has %d blocks, want at most %d", len(message.Blocks), slackMaxBlocks)
	}
}

// TestBuildMessageReportsTruncation covers the digest that does not fit Slack's
// block limit. The notice it carries tells readers the full list is in the run
// log, so buildMessage has to tell the caller to put it there.
func TestBuildMessageReportsTruncation(t *testing.T) {
	var report Report
	for i := 0; i < 60; i++ {
		report.EOL = append(report.EOL, EOLAlert{
			ProductLabel: "PostgreSQL",
			Release:      "14",
			PhaseLabel:   "Support Status",
			Date:         mustDate(t, "2026-11-12"),
			MonthsLeft:   6,
			Dependencies: []DependencyRef{{Name: strings.Repeat("x", slackMaxSectionChars)}},
		})
	}

	message, truncated := buildMessage(report)

	if !truncated {
		t.Fatalf("a digest of %d blocks was not reported as truncated", len(message.Blocks))
	}
	if len(message.Blocks) > slackMaxBlocks {
		t.Errorf("message has %d blocks, want at most %d", len(message.Blocks), slackMaxBlocks)
	}
	if !strings.Contains(blockText(message), "the full list is in the workflow run log") {
		t.Error("the truncated digest does not say where the full list is")
	}
}

// TestBuildMessageTruncatesOverlongLine covers a single line that cannot fit in
// any section on its own. Slack rejects the whole payload when one section goes
// over its limit, so the line has to be cut - on a rune boundary, since the lines
// carry multi-byte characters.
func TestBuildMessageTruncatesOverlongLine(t *testing.T) {
	report := Report{EOL: []EOLAlert{{
		ProductLabel: "PostgreSQL",
		Release:      "14",
		PhaseLabel:   "Support Status",
		Date:         mustDate(t, "2026-11-12"),
		MonthsLeft:   6,
		Dependencies: []DependencyRef{{Name: strings.Repeat("ü", 4000)}},
	}}}

	message, _ := buildMessage(report)

	truncated := false
	for _, block := range message.Blocks {
		if block.Text == nil {
			continue
		}
		if len(block.Text.Text) > 3000 {
			t.Fatalf("section of %d bytes exceeds the Slack limit", len(block.Text.Text))
		}
		if !utf8.ValidString(block.Text.Text) {
			t.Fatal("section was cut in the middle of a rune")
		}
		if strings.Contains(block.Text.Text, "…") {
			truncated = true
		}
	}

	if !truncated {
		t.Error("the overlong line was not marked as truncated")
	}
}

func TestSlackPost(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{name: "accepted", status: http.StatusOK},
		{name: "rejected", status: http.StatusBadRequest, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := newCaptureServer(t, tt.status)

			message, _ := buildMessage(testReport(t))

			payload, err := encodeMessage(message)
			if err != nil {
				t.Fatalf("encodeMessage() error = %v", err)
			}

			err = capture.client().Post(context.Background(), payload)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Post() error = %v, wantErr %v", err, tt.wantErr)
			}

			if capture.requests != 1 {
				t.Fatalf("made %d request(s), want 1", capture.requests)
			}
			if got := capture.header.Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}

			var decoded slackMessage
			if err := json.Unmarshal(capture.body, &decoded); err != nil {
				t.Fatalf("posted body is not a valid Slack message: %v", err)
			}
			if len(decoded.Blocks) == 0 {
				t.Error("posted message carries no blocks")
			}
		})
	}
}

func TestPublishSkipsEmptyReports(t *testing.T) {
	capture := newCaptureServer(t, http.StatusOK)

	if err := publish(context.Background(), capture.client(), Report{}, false); err != nil {
		t.Fatalf("publish() error = %v", err)
	}

	if capture.requests != 0 {
		t.Errorf("made %d request(s), want none for an empty report", capture.requests)
	}
}

func TestPublishHonoursDryRun(t *testing.T) {
	capture := newCaptureServer(t, http.StatusOK)

	if err := publish(context.Background(), capture.client(), testReport(t), true); err != nil {
		t.Fatalf("publish() error = %v", err)
	}

	if capture.requests != 0 {
		t.Errorf("made %d request(s), want none on a dry run", capture.requests)
	}
}

func TestEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ampersand", "AT&T", "AT&amp;T"},
		{"angle brackets", "<script>", "&lt;script&gt;"},
		{"plain text", "PostgreSQL", "PostgreSQL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escape(tt.input); got != tt.want {
				t.Errorf("escape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
