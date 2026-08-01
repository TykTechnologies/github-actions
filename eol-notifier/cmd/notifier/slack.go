package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	// Slack rejects a section whose text exceeds 3000 characters, and a message
	// carrying more than 50 blocks. Long digests are split to stay under both.
	slackMaxSectionChars = 2900
	slackMaxBlocks       = 50
)

type slackText struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Emoji bool   `json:"emoji,omitempty"`
}

type slackBlock struct {
	Type     string      `json:"type"`
	Text     *slackText  `json:"text,omitempty"`
	Elements []slackText `json:"elements,omitempty"`
}

type slackMessage struct {
	Text   string       `json:"text"`
	Blocks []slackBlock `json:"blocks"`
}

// slackClient posts to an incoming webhook. The fields exist so tests can point
// it at an httptest server.
type slackClient struct {
	WebhookURL string
	HTTPClient *http.Client
}

func newSlackClient(webhookURL string) *slackClient {
	return &slackClient{
		WebhookURL: webhookURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Post sends an already-encoded message to the webhook.
func (c *slackClient) Post(ctx context.Context, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to build Slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to post to Slack: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Slack returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	return nil
}

// buildMessage renders a report as a single Block Kit digest. The second return
// value reports whether the digest had to be cut to fit Slack's block limit, so
// the caller can put the whole thing in the run log the notice points at.
func buildMessage(report Report) (slackMessage, bool) {
	message := slackMessage{
		Text:   summaryLine(report),
		Blocks: []slackBlock{headerBlock("Dependency lifecycle digest")},
	}

	if len(report.NewVersions) > 0 {
		message.Blocks = append(message.Blocks, sectionBlocks("*🆕 New versions detected*", newVersionLines(report))...)
	}

	if len(report.EOL) > 0 {
		if len(message.Blocks) > 1 {
			message.Blocks = append(message.Blocks, slackBlock{Type: "divider"})
		}
		message.Blocks = append(message.Blocks, sectionBlocks("*⚠️ Approaching end of life*", eolLines(report))...)
	}

	message.Blocks = append(message.Blocks, contextBlock(footerText(report)))

	if len(message.Blocks) > slackMaxBlocks {
		kept := message.Blocks[:slackMaxBlocks-1]
		message.Blocks = append(kept, contextBlock(
			"Digest truncated to fit Slack's block limit; the full list is in the workflow run log.",
		))

		return message, true
	}

	return message, false
}

// summaryLine is the notification/fallback text shown outside the message body.
func summaryLine(report Report) string {
	parts := make([]string, 0, 2)
	if count := len(report.NewVersions); count > 0 {
		parts = append(parts, fmt.Sprintf("%d new %s", count, noun(count, "version", "versions")))
	}
	if count := len(report.EOL); count > 0 {
		parts = append(parts, fmt.Sprintf("%d %s approaching end of life", count, noun(count, "release", "releases")))
	}

	if len(parts) == 0 {
		return "Dependency lifecycle digest"
	}

	return "Dependency lifecycle digest: " + strings.Join(parts, ", ")
}

func newVersionLines(report Report) []string {
	lines := make([]string, 0, len(report.NewVersions))

	for _, alert := range report.NewVersions {
		line := fmt.Sprintf("• %s *%s*", productLink(alert.ProductLabel, alert.Product, alert.ProductURL), escape(alert.Release))
		if alert.IsLTS {
			line += " _(LTS)_"
		}
		if alert.ReleaseDate != "" {
			line += fmt.Sprintf(" released %s", alert.ReleaseDate)
		}
		lines = append(lines, line+" — "+affects(alert.Dependencies))
	}

	return lines
}

func eolLines(report Report) []string {
	byThreshold := make(map[int][]EOLAlert)
	thresholds := make([]int, 0)
	for _, alert := range report.EOL {
		if _, ok := byThreshold[alert.MonthsLeft]; !ok {
			thresholds = append(thresholds, alert.MonthsLeft)
		}
		byThreshold[alert.MonthsLeft] = append(byThreshold[alert.MonthsLeft], alert)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(thresholds)))

	var lines []string
	for _, months := range thresholds {
		lines = append(lines, fmt.Sprintf("*In %d %s*", months, noun(months, "month", "months")))
		for _, alert := range byThreshold[months] {
			lines = append(lines, fmt.Sprintf(
				"• %s *%s* — %s ends %s — %s",
				productLink(alert.ProductLabel, alert.Product, alert.ProductURL),
				escape(alert.Release),
				escape(alert.PhaseLabel),
				alert.Date.Format(dateLayout),
				affects(alert.Dependencies),
			))
		}
	}

	return lines
}

func footerText(report Report) string {
	parts := []string{
		"Adding or removing a version is a manual PR against `TykTechnologies/github-actions`.",
	}

	if report.hasProxy() {
		parts = append(parts,
			"Entries marked _upstream proxy_ are not tracked by endoflife.date; "+
				"the date shown is the upstream engine's and must be confirmed with the cloud provider.",
		)
	}

	if report.Seed {
		parts = append(parts, "First run: there is no previous state to compare against, so new versions are not listed.")
	}

	return strings.Join(parts, " ")
}

// affects renders the dependencies an alert applies to, flagging the ones whose
// lifecycle is only inferred from the upstream engine.
func affects(dependencies []DependencyRef) string {
	names := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		name := escape(dependency.Name)
		if dependency.UpstreamProxy {
			name += " _(upstream proxy)_"
		}
		names = append(names, name)
	}

	return "affects " + strings.Join(names, ", ")
}

func productLink(label, product, url string) string {
	if label == "" {
		label = product
	}
	if url == "" {
		return escape(label)
	}

	return fmt.Sprintf("<%s|%s>", url, escape(label))
}

// sectionBlocks packs lines into as many section blocks as Slack's size limit
// requires, with the heading on the first one.
func sectionBlocks(heading string, lines []string) []slackBlock {
	var (
		blocks  []slackBlock
		builder strings.Builder
	)
	builder.WriteString(heading)

	flush := func() {
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: builder.String()},
		})
		builder.Reset()
	}

	for _, line := range lines {
		line = truncateLine(line)

		if builder.Len() > 0 && builder.Len()+len(line)+1 > slackMaxSectionChars {
			flush()
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(line)
	}

	if builder.Len() > 0 {
		flush()
	}

	return blocks
}

// truncateLine keeps a single line within the section limit. Without this a very
// long line - many dependencies sharing one product, say - would be written into
// a freshly flushed builder and push that section over Slack's cap, which makes
// Slack reject the whole payload.
func truncateLine(line string) string {
	const ellipsis = "…"

	if len(line) <= slackMaxSectionChars {
		return line
	}

	limit := slackMaxSectionChars - len(ellipsis)
	cut := 0
	for index := range line {
		if index > limit {
			break
		}
		cut = index
	}

	return line[:cut] + ellipsis
}

func headerBlock(text string) slackBlock {
	return slackBlock{Type: "header", Text: &slackText{Type: "plain_text", Text: text, Emoji: true}}
}

func contextBlock(text string) slackBlock {
	return slackBlock{Type: "context", Elements: []slackText{{Type: "mrkdwn", Text: text}}}
}

// escape neutralises the three characters Slack treats as markup control
// characters in mrkdwn text.
func escape(text string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
}

func noun(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}

	return plural
}

// encodeMessage renders the payload posted to the webhook.
func encodeMessage(message slackMessage) ([]byte, error) {
	payload, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to encode Slack message: %w", err)
	}

	return payload, nil
}
