package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const e2eConfig = `
dependencies:
  - name: PostgreSQL
    product: postgresql
    track: [eol]
  - name: Redis
    product: redis
    track: [eol]
  - name: GCP MemoryStore
    product: redis
    track: [eol]
    upstream_proxy: true
`

// e2eOptions wires a full run against fake endoflife.date and Slack servers.
func e2eOptions(t *testing.T, dir string, slack *captureServer, today string, config string) options {
	t.Helper()

	configPath := filepath.Join(dir, "dependencies.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	return options{
		configPath: configPath,
		statePath:  filepath.Join(dir, "state.json"),
		now:        mustDate(t, today),
		client:     testClient(fixtureServer(t, nil).URL),
		slack:      slack.client(),
	}
}

func readState(t *testing.T, path string) State {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}

	state := State{}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("failed to parse state file: %v", err)
	}

	return state
}

// TestExecuteSeedRun covers the first run: it must record a baseline rather than
// announcing every historical release cycle.
func TestExecuteSeedRun(t *testing.T) {
	dir := t.TempDir()
	slack := newCaptureServer(t, http.StatusOK)
	opts := e2eOptions(t, dir, slack, "2026-07-29", e2eConfig)

	if err := execute(context.Background(), opts); err != nil {
		t.Fatalf("execute() error = %v", err)
	}

	if slack.requests != 0 {
		t.Errorf("posted %d message(s), want none on a seed run with no thresholds due", slack.requests)
	}

	state := readState(t, opts.statePath)
	if !state.seen("postgresql")["18"] || !state.seen("redis")["8.8"] {
		t.Errorf("baseline was not recorded: %v", state)
	}
}

// TestExecuteReportsNewVersionAndEOL is the main path: a release the previous
// run did not see, alongside a release hitting a threshold today.
func TestExecuteReportsNewVersionAndEOL(t *testing.T) {
	dir := t.TempDir()
	slack := newCaptureServer(t, http.StatusOK)
	// postgresql 15 reaches end of life on 2027-11-11, twelve months from today.
	opts := e2eOptions(t, dir, slack, "2026-11-11", e2eConfig)

	seed := State{
		"postgresql": {"15", "16", "17"},
		"redis":      {"8.2", "8.4", "8.6", "8.8"},
	}
	if err := saveState(opts.statePath, seed); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	if err := execute(context.Background(), opts); err != nil {
		t.Fatalf("execute() error = %v", err)
	}

	if slack.requests != 1 {
		t.Fatalf("posted %d message(s), want 1", slack.requests)
	}

	var message slackMessage
	if err := json.Unmarshal(slack.body, &message); err != nil {
		t.Fatalf("posted body is not a valid Slack message: %v", err)
	}
	rendered := blockText(message)

	for _, want := range []string{
		"New versions detected",
		"*18*",
		"*In 12 months*",
		"Support Status ends 2027-11-11",
		"affects PostgreSQL",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("digest is missing %q:\n%s", want, rendered)
		}
	}

	if !readState(t, opts.statePath).seen("postgresql")["18"] {
		t.Error("state was not updated after a successful post")
	}
}

// TestExecuteKeepsStateWhenSlackFails guards the invariant that makes the state
// file safe: a release must never be recorded as seen if its alert never landed.
func TestExecuteKeepsStateWhenSlackFails(t *testing.T) {
	dir := t.TempDir()
	slack := newCaptureServer(t, http.StatusInternalServerError)
	opts := e2eOptions(t, dir, slack, "2026-11-11", e2eConfig)

	seed := State{"postgresql": {"15", "16", "17"}, "redis": {"8.2", "8.4", "8.6", "8.8"}}
	if err := saveState(opts.statePath, seed); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	if err := execute(context.Background(), opts); err == nil {
		t.Fatal("execute() succeeded despite Slack rejecting the message")
	}

	if readState(t, opts.statePath).seen("postgresql")["18"] {
		t.Error("state recorded a release whose alert was never delivered")
	}
}

func TestExecuteDryRunLeavesStateUntouched(t *testing.T) {
	dir := t.TempDir()
	slack := newCaptureServer(t, http.StatusOK)
	opts := e2eOptions(t, dir, slack, "2026-11-11", e2eConfig)
	opts.dryRun = true

	if err := execute(context.Background(), opts); err != nil {
		t.Fatalf("execute() error = %v", err)
	}

	if slack.requests != 0 {
		t.Errorf("posted %d message(s), want none on a dry run", slack.requests)
	}
	if _, err := os.Stat(opts.statePath); !os.IsNotExist(err) {
		t.Errorf("dry run wrote a state file at %s", opts.statePath)
	}
}

// TestExecuteFailsOnUnreadableDate turns a date the API states in a form we
// cannot parse into a failed run. The release loses its alert on the single day
// it was due, and a green job would leave nobody a reason to look.
func TestExecuteFailsOnUnreadableDate(t *testing.T) {
	dir := t.TempDir()
	slack := newCaptureServer(t, http.StatusOK)

	config := e2eConfig + `
  - name: Broken Dates
    product: broken-dates
    track: [eol]
`
	opts := e2eOptions(t, dir, slack, "2026-11-11", config)

	// broken-dates is seeded too, so the run fails over the date alone rather
	// than over the release also looking new.
	seed := State{
		"postgresql":   {"15", "16", "17"},
		"redis":        {"8.2", "8.4", "8.6", "8.8"},
		"broken-dates": {"1"},
	}
	if err := saveState(opts.statePath, seed); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	err := execute(context.Background(), opts)
	if err == nil {
		t.Fatal("execute() succeeded despite a date it could not read")
	}
	for _, want := range []string{"broken-dates", "November 2027"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}

	// The failure must not cost the alerts that were readable.
	if slack.requests != 1 {
		t.Errorf("posted %d message(s), want the readable alerts still reported", slack.requests)
	}
	if !readState(t, opts.statePath).seen("postgresql")["18"] {
		t.Error("state was not updated despite a delivered digest")
	}
}

// TestExecuteContinuesAfterFetchFailure keeps one unreachable product from
// silencing the alerts for everything else, while still failing the run.
func TestExecuteContinuesAfterFetchFailure(t *testing.T) {
	dir := t.TempDir()
	slack := newCaptureServer(t, http.StatusOK)

	config := e2eConfig + `
  - name: Missing
    product: does-not-exist
    track: [eol]
`
	opts := e2eOptions(t, dir, slack, "2026-11-11", config)

	seed := State{"postgresql": {"15", "16", "17"}, "redis": {"8.2", "8.4", "8.6", "8.8"}}
	if err := saveState(opts.statePath, seed); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	err := execute(context.Background(), opts)
	if err == nil {
		t.Fatal("execute() succeeded despite an unreachable product")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error = %v, want it to name the unreachable product", err)
	}

	if slack.requests != 1 {
		t.Errorf("posted %d message(s), want the healthy products still reported", slack.requests)
	}

	// The delivered alerts must still be recorded, or the failing run would
	// re-announce the same versions on every future run.
	if !readState(t, opts.statePath).seen("postgresql")["18"] {
		t.Error("state was not updated for the products that were reachable")
	}
}
