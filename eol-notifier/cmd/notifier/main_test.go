package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
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

// e2eState seeds the state a healthy run would have left on day: every release
// recorded, and every alert due by then already delivered. The releases named by
// unseen are then dropped, standing in for cycles the API listed since.
func e2eState(t *testing.T, day string, unseen ...string) State {
	t.Helper()

	slack := newCaptureServer(t, http.StatusOK)
	opts := e2eOptions(t, t.TempDir(), slack, day, e2eConfig)

	if err := execute(context.Background(), opts); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}
	if slack.requests != 0 {
		t.Fatalf("the seeding run posted %d message(s), so it is not a clean slate", slack.requests)
	}

	state := readState(t, opts.statePath)
	for product, recorded := range state {
		recorded.Releases = slices.DeleteFunc(recorded.Releases, func(name string) bool {
			return slices.Contains(unseen, name)
		})
		state[product] = recorded
	}

	return state
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

	// redis 8.2 was already out of support when the action first saw it. The
	// alert saying so is spent, not owed, and has to be recorded as such or the
	// next run would announce it.
	if !state.sent("redis")[alertKey("8.2", phaseEOL, endedMarker, "2026-05-25")] {
		t.Errorf("the baseline did not record its elapsed alerts: %v", state["redis"].Sent)
	}
}

// TestExecuteSeedRunAnnouncesLiveWarnings covers the first run against a product
// already inside one of its warning windows. Recording the warning silently
// would leave it in a file nobody reads while the channel, which is where people
// actually find out, hears nothing.
func TestExecuteSeedRunAnnouncesLiveWarnings(t *testing.T) {
	dir := t.TempDir()
	slack := newCaptureServer(t, http.StatusOK)
	// postgresql 15's twelve-month warning came up the day before, and no run
	// has ever happened to deliver it.
	opts := e2eOptions(t, dir, slack, "2026-11-12", e2eConfig)

	if err := execute(context.Background(), opts); err != nil {
		t.Fatalf("execute() error = %v", err)
	}

	if slack.requests != 1 {
		t.Fatalf("posted %d message(s), want the warning announced", slack.requests)
	}

	var message slackMessage
	if err := json.Unmarshal(slack.body, &message); err != nil {
		t.Fatalf("posted body is not a valid Slack message: %v", err)
	}
	rendered := blockText(message)

	for _, want := range []string{"*In 12 months*", "*15* — Support Status ends 2027-11-11"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("digest is missing %q:\n%s", want, rendered)
		}
	}

	// The back catalogue still stays out of it: no new-version alerts, and the
	// phase that ended before the first run is recorded rather than announced.
	if strings.Contains(rendered, "New versions detected") {
		t.Errorf("the first run announced new versions:\n%s", rendered)
	}
	if strings.Contains(rendered, "Already ended") {
		t.Errorf("the first run announced a phase that ended before it:\n%s", rendered)
	}
	if !readState(t, opts.statePath).sent("redis")[alertKey("8.2", phaseEOL, endedMarker, "2026-05-25")] {
		t.Error("the spent alert was not recorded, so the next run would announce it")
	}
}

// TestExecuteReportsNewVersionAndEOL is the main path: a release the previous
// run did not see, alongside a release hitting a threshold today.
func TestExecuteReportsNewVersionAndEOL(t *testing.T) {
	dir := t.TempDir()
	slack := newCaptureServer(t, http.StatusOK)
	// postgresql 15 reaches end of life on 2027-11-11, twelve months from today.
	opts := e2eOptions(t, dir, slack, "2026-11-11", e2eConfig)

	if err := saveState(opts.statePath, e2eState(t, "2026-11-10", "18")); err != nil {
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

	if err := saveState(opts.statePath, e2eState(t, "2026-11-10", "18")); err != nil {
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

	// broken-dates is absent from the seeded state, so it is a baseline product
	// and the run fails over the date alone rather than over its releases also
	// looking new.
	if err := saveState(opts.statePath, e2eState(t, "2026-11-10", "18")); err != nil {
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

	if err := saveState(opts.statePath, e2eState(t, "2026-11-10", "18")); err != nil {
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

// TestExecuteCatchesUpAfterAnOutage is the point of the whole design: a year
// with no runs at all must not cost the channel a single alert. postgresql 15
// reached its end of life during the gap, and 16 came up on its twelve-month
// warning; both are owed on the first run back.
func TestExecuteCatchesUpAfterAnOutage(t *testing.T) {
	dir := t.TempDir()
	slack := newCaptureServer(t, http.StatusOK)
	opts := e2eOptions(t, dir, slack, "2027-11-12", e2eConfig)

	if err := saveState(opts.statePath, e2eState(t, "2026-11-10")); err != nil {
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
		"*Already ended*",
		"*15* — Support Status ended 2027-11-11",
		"*In 12 months*",
		"*16* — Support Status ends 2028-11-09",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("digest is missing %q:\n%s", want, rendered)
		}
	}

	// The alerts it caught up on must now be spent, or the next run repeats them.
	state := readState(t, opts.statePath)
	for _, key := range []string{
		alertKey("15", phaseEOL, endedMarker, "2027-11-11"),
		alertKey("16", phaseEOL, "12", "2028-11-09"),
	} {
		if !state.sent("postgresql")[key] {
			t.Errorf("alert %q was delivered but not recorded", key)
		}
	}
}

// TestRecordRunKeepsUndeliveredAlertsDue covers a digest that only partly
// landed. What reached the channel is recorded and what did not stays owed, so
// the next run repeats nothing and loses nothing.
func TestRecordRunKeepsUndeliveredAlertsDue(t *testing.T) {
	report := Report{
		NewVersions: []NewVersionAlert{
			{Product: "postgresql", Release: "18"},
			{Product: "redis", Release: "8.8"},
		},
		EOL: []EOLAlert{
			{Product: "postgresql", key: "15|eol|12|2027-11-11"},
			{Product: "redis", key: "8.2|eol|1|2026-05-25"},
		},
		BaselineKeys: map[string][]string{"mysql": {"8.0|eol|ended|2026-04-30"}},
	}
	delivered := []Report{{
		NewVersions: report.NewVersions[:1],
		EOL:         report.EOL[:1],
	}}
	products := map[string]*Product{
		"postgresql": {Releases: []Release{{Name: "18"}}},
		"redis":      {Releases: []Release{{Name: "8.8"}}},
	}

	state := State{}
	recordRun(state, products, report, delivered)

	if !state.sent("postgresql")["15|eol|12|2027-11-11"] {
		t.Error("a delivered alert was not recorded, so it would be sent again")
	}
	if state.sent("redis")["8.2|eol|1|2026-05-25"] {
		t.Error("an alert that never landed was recorded as delivered")
	}
	if !state.sent("mysql")["8.0|eol|ended|2026-04-30"] {
		t.Error("a baseline product's spent alerts were not recorded")
	}

	if !state.seen("postgresql")["18"] {
		t.Error("a product whose alerts all landed was not recorded as seen")
	}
	if state.seen("redis")["8.8"] {
		t.Error("a release was recorded as seen although its alert never landed")
	}
}
