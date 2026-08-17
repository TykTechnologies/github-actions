package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

// captureServer stands in for the Slack webhook and records what was posted. A
// digest can take more than one request, so every body is kept.
type captureServer struct {
	*httptest.Server
	requests int
	bodies   [][]byte
	body     []byte
	header   http.Header
	status   int
	// failFrom is the 1-based request from which the server starts rejecting,
	// for the runs where Slack goes away partway through a split digest.
	failFrom int
}

func newCaptureServer(t *testing.T, status int) *captureServer {
	t.Helper()

	capture := &captureServer{status: status}
	capture.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.requests++
		capture.header = r.Header.Clone()
		capture.body, _ = io.ReadAll(r.Body)
		capture.bodies = append(capture.bodies, capture.body)

		if capture.failFrom > 0 && capture.requests >= capture.failFrom {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("nope"))

			return
		}

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

// render builds the digest and flattens it, for the assertions that only care
// about the text.
func render(report Report) string {
	return blockText(buildMessage(report))
}

// bigReport builds a digest of count alerts whose lines are lineLength long. A
// line long enough to fill a section on its own is how a digest is pushed past
// the block limit without needing thousands of alerts.
func bigReport(t *testing.T, count, lineLength int) Report {
	t.Helper()

	var report Report
	for i := 0; i < count; i++ {
		release := fmt.Sprintf("%d", i)
		report.EOL = append(report.EOL, EOLAlert{
			Product:      "postgresql",
			ProductLabel: "PostgreSQL",
			Release:      release,
			PhaseLabel:   "Support Status",
			Date:         mustDate(t, "2026-11-12"),
			MonthsLeft:   6,
			Dependencies: []DependencyRef{{Name: strings.Repeat("x", lineLength)}},
			keys:         []string{alertKey(release, phaseEOL, "6", "2026-11-12")},
		})
	}

	return report
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
	message := buildMessage(testReport(t))
	rendered := blockText(message)

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

// TestBuildMessageExplainsNewlyTrackedProduct covers a dependency added to the
// config long after the first run. Its already-ended phases are withheld exactly
// as on a first run, so the digest has to say so - without the caveat, a reader
// takes the warnings listed for the whole picture.
func TestBuildMessageExplainsNewlyTrackedProduct(t *testing.T) {
	report := testReport(t)
	report.BaselineKeys = map[string][]string{"mysql": {"5.7|eol|ended|2023-10-31"}}

	rendered := render(report)
	if !strings.Contains(rendered, "First run for one or more products") {
		t.Errorf("the withheld phases are not explained:\n%s", rendered)
	}
	if !strings.Contains(rendered, "had already ended") {
		t.Errorf("the caveat does not mention the withheld end-of-life phases:\n%s", rendered)
	}
}

// TestBuildMessageOmitsCaveatWhenNothingWithheld keeps the caveat off the
// digests that withheld nothing.
func TestBuildMessageOmitsCaveatWhenNothingWithheld(t *testing.T) {
	if rendered := render(testReport(t)); strings.Contains(rendered, "First run") {
		t.Errorf("a routine digest carries the first-run caveat:\n%s", rendered)
	}
}

func TestBuildMessagePacksLinesIntoSections(t *testing.T) {
	message := buildMessage(bigReport(t, 120, 60))

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
		t.Errorf("got %d section(s), want the lines split across several", sections)
	}
}

// TestSplitReportKeepsEveryAlert covers the digest too large for one message.
// Every part has to be postable on its own, and between them they have to carry
// the whole report: a part dropped to make the digest fit is an alert nobody
// ever sees.
func TestSplitReportKeepsEveryAlert(t *testing.T) {
	report := bigReport(t, 60, slackMaxSectionChars)
	report.NewVersions = testReport(t).NewVersions
	report.BaselineKeys = map[string][]string{"mysql": {"5.7|eol|ended|2023-10-31"}}

	parts := splitReport(report)

	if len(parts) < 2 {
		t.Fatalf("a %d-alert digest was left in %d part(s)", report.alertCount(), len(parts))
	}

	seen := make(map[string]bool)
	for _, part := range parts {
		if blocks := len(buildMessage(part).Blocks); blocks > slackMaxBlocks {
			t.Errorf("a part has %d blocks, want at most %d", blocks, slackMaxBlocks)
		}
		// Each part is read on its own, so each has to carry the caveat.
		if !strings.Contains(blockText(buildMessage(part)), "First run for one or more products") {
			t.Error("a part of a split digest lost the first-run caveat")
		}
		for _, alert := range part.EOL {
			seen[alert.keys[0]] = true
		}
		for _, alert := range part.NewVersions {
			seen[alert.Product+"|"+alert.Release] = true
		}
	}

	if len(seen) != report.alertCount() {
		t.Errorf("the parts carry %d of the report's %d alerts", len(seen), report.alertCount())
	}
}

// TestBuildMessageRendersEndedPhases covers the alert for a phase that is over.
// It is the one alert that cannot be worded as a countdown.
func TestBuildMessageRendersEndedPhases(t *testing.T) {
	report := Report{EOL: []EOLAlert{
		{
			ProductLabel: "Redis",
			Release:      "7.2",
			PhaseLabel:   "Security Support",
			Date:         mustDate(t, "2026-08-01"),
			Ended:        true,
			Dependencies: []DependencyRef{{Name: "Redis"}},
		},
		{
			ProductLabel: "PostgreSQL",
			Release:      "15",
			PhaseLabel:   "Support Status",
			Date:         mustDate(t, "2027-02-11"),
			MonthsLeft:   6,
			Dependencies: []DependencyRef{{Name: "PostgreSQL"}},
		},
	}}

	rendered := render(report)

	for _, want := range []string{
		"*Already ended*",
		"Redis *7.2* — Security Support ended 2026-08-01",
		"*In 6 months*",
		"PostgreSQL *15* — Support Status ends 2027-02-11",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered message is missing %q:\n%s", want, rendered)
		}
	}

	if strings.Index(rendered, "Already ended") > strings.Index(rendered, "In 6 months") {
		t.Errorf("the ended phase is listed after the countdown:\n%s", rendered)
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

	message := buildMessage(report)

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

			payload, err := encodeMessage(buildMessage(testReport(t)))
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

	delivered, err := publish(context.Background(), capture.client(), Report{}, false)
	if err != nil {
		t.Fatalf("publish() error = %v", err)
	}

	if capture.requests != 0 {
		t.Errorf("made %d request(s), want none for an empty report", capture.requests)
	}
	if len(delivered) != 0 {
		t.Errorf("reported %d part(s) as delivered, want none", len(delivered))
	}
}

func TestPublishHonoursDryRun(t *testing.T) {
	capture := newCaptureServer(t, http.StatusOK)

	delivered, err := publish(context.Background(), capture.client(), testReport(t), true)
	if err != nil {
		t.Fatalf("publish() error = %v", err)
	}

	if capture.requests != 0 {
		t.Errorf("made %d request(s), want none on a dry run", capture.requests)
	}
	// Nothing was delivered, so nothing may be recorded as delivered: a dry run
	// that marked alerts as sent would silence them for good.
	if len(delivered) != 0 {
		t.Errorf("reported %d part(s) as delivered on a dry run", len(delivered))
	}
}

// TestPublishPostsEveryPart covers a digest too large for one message: every
// part has to be posted, not just the first.
func TestPublishPostsEveryPart(t *testing.T) {
	capture := newCaptureServer(t, http.StatusOK)
	report := bigReport(t, 60, slackMaxSectionChars)

	delivered, err := publish(context.Background(), capture.client(), report, false)
	if err != nil {
		t.Fatalf("publish() error = %v", err)
	}

	if capture.requests < 2 {
		t.Fatalf("made %d request(s), want the digest posted in parts", capture.requests)
	}
	if len(delivered) != capture.requests {
		t.Errorf("reported %d part(s) as delivered over %d request(s)", len(delivered), capture.requests)
	}

	alerts := 0
	for _, part := range delivered {
		alerts += part.alertCount()
	}
	if alerts != report.alertCount() {
		t.Errorf("delivered %d of %d alerts", alerts, report.alertCount())
	}
}

// TestPublishReportsWhatLandedBeforeFailing covers Slack going away halfway
// through a split digest. The parts that landed have to be reported as
// delivered, or they would be posted again on the next run.
func TestPublishReportsWhatLandedBeforeFailing(t *testing.T) {
	capture := newCaptureServer(t, http.StatusOK)
	capture.failFrom = 2

	delivered, err := publish(context.Background(), capture.client(), bigReport(t, 60, slackMaxSectionChars), false)
	if err == nil {
		t.Fatal("publish() succeeded despite Slack rejecting a part")
	}

	if len(delivered) != 1 {
		t.Errorf("reported %d part(s) as delivered, want only the one that landed", len(delivered))
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
