package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(dateLayout, value)
	if err != nil {
		t.Fatalf("failed to parse test date %q: %v", value, err)
	}

	return parsed
}

func strPtr(value string) *string {
	return &value
}

// tracked builds a state that has seen a product before, so detection treats it
// as tracked rather than as a baseline with nothing to announce.
func tracked(product string, releases ...string) State {
	return State{product: {Releases: releases}}
}

// firedMonths lists what a report alerts on, an ended phase counting as 0, so
// tests can assert on which thresholds came up rather than only how many.
func firedMonths(report Report) []int {
	months := make([]int, 0, len(report.EOL))
	for _, alert := range report.EOL {
		if alert.Ended {
			months = append(months, 0)
			continue
		}
		months = append(months, alert.MonthsLeft)
	}

	return months
}

// testConfig builds a validated config, so tests exercise the same defaulting
// the real config file goes through.
func testConfig(t *testing.T, dependencies ...Dependency) *DependencyConfig {
	t.Helper()

	config := &DependencyConfig{Dependencies: dependencies}
	if err := config.validate(); err != nil {
		t.Fatalf("failed to validate test config: %v", err)
	}

	return config
}

func TestMonthsBefore(t *testing.T) {
	tests := []struct {
		name   string
		date   string
		months int
		want   string
	}{
		{"twelve months", "2027-11-11", 12, "2026-11-11"},
		{"six months", "2027-11-11", 6, "2027-05-11"},
		{"one month", "2027-11-11", 1, "2027-10-11"},
		{"clamps onto a shorter month", "2027-03-31", 1, "2027-02-28"},
		{"clamps onto a leap february", "2028-03-31", 1, "2028-02-29"},
		{"leap day keeps its day number", "2028-02-29", 1, "2028-01-29"},
		{"crosses the year boundary", "2027-01-15", 12, "2026-01-15"},
		{"crosses the year boundary and clamps", "2027-01-31", 2, "2026-11-30"},
		{"twelve months from a leap day", "2028-02-29", 12, "2027-02-28"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monthsBefore(mustDate(t, tt.date), tt.months)
			if want := mustDate(t, tt.want); !got.Equal(want) {
				t.Errorf("monthsBefore(%s, %d) = %s, want %s",
					tt.date, tt.months, got.Format(dateLayout), tt.want)
			}
		})
	}
}

// TestEveryAlertFiresExactlyOnce drives the detection over a stretch of days,
// recording what each run delivered, and covers both halves of the guarantee:
// nothing is announced twice, and nothing is lost when the day an alert came up
// had no run at all. The dates are the ones that motivated the clamping in
// monthsBefore, where a threshold is most likely to land on the wrong day.
func TestEveryAlertFiresExactlyOnce(t *testing.T) {
	eolDates := []string{"2027-01-31", "2027-02-28", "2027-03-31", "2028-02-29", "2027-08-31"}

	// Runs every day, and runs that skip four days in five: the outcome must not
	// depend on which days the action happened to be up.
	for _, step := range []int{1, 5} {
		for _, eol := range eolDates {
			t.Run(fmt.Sprintf("%s every %d day(s)", eol, step), func(t *testing.T) {
				config := testConfig(t, Dependency{Name: "PostgreSQL", Product: "postgresql"})
				products := map[string]*Product{
					"postgresql": {
						Name:     "postgresql",
						Label:    "PostgreSQL",
						Releases: []Release{{Name: "15", EOLFrom: strPtr(eol)}},
					},
				}

				state := tracked("postgresql", "15")
				fired := make(map[int]int)

				end := mustDate(t, eol).AddDate(0, 0, 10)
				for day := monthsBefore(mustDate(t, eol), 12).AddDate(0, 0, -10); !day.After(end); day = day.AddDate(0, 0, step) {
					report, _ := detectAlerts(config, products, state, day, false)
					for _, months := range firedMonths(report) {
						fired[months]++
					}
					for _, alert := range report.EOL {
						state.markSent(alert.Product, []string{alert.key})
					}
				}

				for _, months := range append([]int{0}, config.ThresholdsMonths...) {
					if fired[months] != 1 {
						t.Errorf("the %s alert fired %d time(s), want exactly 1", thresholdName(months), fired[months])
					}
				}
			})
		}
	}
}

func thresholdName(months int) string {
	if months == 0 {
		return "ended"
	}

	return fmt.Sprintf("%d month", months)
}

func TestDetectAlertsEOL(t *testing.T) {
	postgres := func(releases ...Release) map[string]*Product {
		return map[string]*Product{
			"postgresql": {
				Name:     "postgresql",
				Label:    "PostgreSQL",
				Labels:   map[string]string{phaseEOL: "Support Status"},
				Releases: releases,
			},
		}
	}

	// The alerts a run would have delivered on the way to 2027-10-11.
	alreadySent := []string{
		alertKey("15", phaseEOL, "12", "2027-11-11"),
		alertKey("15", phaseEOL, "6", "2027-11-11"),
	}

	tests := []struct {
		name     string
		products map[string]*Product
		sent     []string
		today    string
		want     []int
	}{
		{
			name:     "fires twelve months ahead",
			products: postgres(Release{Name: "15", EOLFrom: strPtr("2027-11-11")}),
			today:    "2026-11-11",
			want:     []int{12},
		},
		{
			name:     "fires one month ahead",
			products: postgres(Release{Name: "15", EOLFrom: strPtr("2027-11-11")}),
			sent:     alreadySent,
			today:    "2027-10-11",
			want:     []int{1},
		},
		{
			name:     "silent a day early",
			products: postgres(Release{Name: "15", EOLFrom: strPtr("2027-11-11")}),
			sent:     alreadySent,
			today:    "2027-10-10",
			want:     nil,
		},
		{
			name:     "silent once delivered",
			products: postgres(Release{Name: "15", EOLFrom: strPtr("2027-11-11")}),
			sent:     append(alreadySent, alertKey("15", phaseEOL, "1", "2027-11-11")),
			today:    "2027-10-12",
			want:     nil,
		},
		{
			// The action was down for the days the first two thresholds came up.
			// Both are still owed, and a missed warning is worth more late than
			// never.
			name:     "catches up on every threshold that was missed",
			products: postgres(Release{Name: "15", EOLFrom: strPtr("2027-11-11")}),
			today:    "2027-10-11",
			want:     []int{12, 6, 1},
		},
		{
			name:     "skips a release with no announced date",
			products: postgres(Release{Name: "18", EOLFrom: nil}),
			today:    "2026-11-11",
			want:     nil,
		},
		{
			// A phase that is over is reported as over rather than skipped: it is
			// the one thing a channel that missed the countdown still needs.
			name:     "reports a phase that has already passed",
			products: postgres(Release{Name: "13", EOLFrom: strPtr("2026-05-25"), IsEOL: true}),
			today:    "2026-11-11",
			want:     []int{0},
		},
		{
			name:     "skips a product whose fetch failed",
			products: map[string]*Product{},
			today:    "2026-11-11",
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := testConfig(t, Dependency{Name: "PostgreSQL", Product: "postgresql"})
			state := tracked("postgresql", "13", "15", "18")
			state.markSent("postgresql", tt.sent)

			report, _ := detectAlerts(config, tt.products, state, mustDate(t, tt.today), false)

			if got := firedMonths(report); !slices.Equal(got, tt.want) {
				t.Fatalf("fired %v, want %v", got, tt.want)
			}
			if len(tt.want) > 0 && report.EOL[0].PhaseLabel != "Support Status" {
				t.Errorf("phase label = %q, want the product's own wording", report.EOL[0].PhaseLabel)
			}
		})
	}
}

// TestDetectAlertsRefiresAfterDateRevision covers endoflife.date moving a date:
// the alert delivered for the old date says nothing about the new one, so the
// new one is due on its own.
func TestDetectAlertsRefiresAfterDateRevision(t *testing.T) {
	config := testConfig(t, Dependency{Name: "PostgreSQL", Product: "postgresql"})
	products := map[string]*Product{
		"postgresql": {
			Name:     "postgresql",
			Label:    "PostgreSQL",
			Releases: []Release{{Name: "15", EOLFrom: strPtr("2027-09-30")}},
		},
	}

	state := tracked("postgresql", "15")
	state.markSent("postgresql", []string{alertKey("15", phaseEOL, "12", "2027-11-11")})

	report, _ := detectAlerts(config, products, state, mustDate(t, "2026-11-11"), false)

	if got := firedMonths(report); !slices.Equal(got, []int{12}) {
		t.Fatalf("fired %v, want the twelve-month alert for the revised date", got)
	}
	if want := mustDate(t, "2027-09-30"); !report.EOL[0].Date.Equal(want) {
		t.Errorf("alert date = %s, want the revised date", report.EOL[0].Date.Format(dateLayout))
	}
}

// TestDetectAlertsBaselineRecordsWithoutAlerting covers a product with no
// recorded history: on the first run, or when a dependency is added to the
// config, its elapsed thresholds are already spent and must not be emptied into
// the channel. They are recorded as delivered so they stay that way.
func TestDetectAlertsBaselineRecordsWithoutAlerting(t *testing.T) {
	config := testConfig(t, Dependency{Name: "PostgreSQL", Product: "postgresql"})
	products := map[string]*Product{
		"postgresql": {
			Name:  "postgresql",
			Label: "PostgreSQL",
			Releases: []Release{
				{Name: "13", EOLFrom: strPtr("2025-11-13"), IsEOL: true},
				{Name: "15", EOLFrom: strPtr("2027-11-11")},
			},
		},
	}

	report, _ := detectAlerts(config, products, State{}, mustDate(t, "2026-11-11"), true)

	if len(report.EOL) != 0 {
		t.Errorf("a product with no recorded history announced %d alert(s)", len(report.EOL))
	}

	want := []string{
		alertKey("13", phaseEOL, endedMarker, "2025-11-13"),
		alertKey("15", phaseEOL, "12", "2027-11-11"),
	}
	if got := report.BaselineKeys["postgresql"]; !slices.Equal(got, want) {
		t.Errorf("baseline keys = %v, want %v", got, want)
	}
}

// TestDetectAlertsReportsUnreadableDates covers a date the API states in a form
// the layout cannot read. The release loses its alert either way, so the run has
// to report it: a silent skip leaves a green job and an empty channel, which is
// indistinguishable from having nothing to say.
func TestDetectAlertsReportsUnreadableDates(t *testing.T) {
	products := map[string]*Product{
		"postgresql": {
			Name:  "postgresql",
			Label: "PostgreSQL",
			Releases: []Release{
				{Name: "15", EOLFrom: strPtr("November 2027")},
				{Name: "16", EOLFrom: strPtr("2027-11-11")},
			},
		},
	}
	config := testConfig(t, Dependency{Name: "PostgreSQL", Product: "postgresql"})

	report, unreadable := detectAlerts(config, products, tracked("postgresql", "15", "16"), mustDate(t, "2026-11-11"), false)

	if len(unreadable) != 1 {
		t.Fatalf("got %d unreadable date(s), want 1: %v", len(unreadable), unreadable)
	}
	for _, want := range []string{"postgresql", "15", phaseEOL, "November 2027"} {
		if !strings.Contains(unreadable[0], want) {
			t.Errorf("unreadable entry %q does not mention %q", unreadable[0], want)
		}
	}

	// One bad date is not a reason to stop reading the releases around it.
	if len(report.EOL) != 1 {
		t.Fatalf("got %d EoL alert(s), want the readable release still reported", len(report.EOL))
	}
	if report.EOL[0].Release != "16" {
		t.Errorf("alert is for release %q, want 16", report.EOL[0].Release)
	}
}

func TestDetectAlertsTracksConfiguredPhasesOnly(t *testing.T) {
	products := map[string]*Product{
		"amazon-rds-postgresql": {
			Name:   "amazon-rds-postgresql",
			Label:  "Amazon RDS for PostgreSQL",
			Labels: map[string]string{phaseEOL: "Security Support", phaseEOES: "Extended Support"},
			Releases: []Release{{
				Name:     "15",
				EOLFrom:  strPtr("2028-02-29"),
				EOESFrom: strPtr("2031-02-28"),
			}},
		},
	}

	// By 2030 the security support the rows below run past is long delivered.
	spent := []string{
		alertKey("15", phaseEOL, "12", "2028-02-29"),
		alertKey("15", phaseEOL, "6", "2028-02-29"),
		alertKey("15", phaseEOL, "1", "2028-02-29"),
		alertKey("15", phaseEOL, endedMarker, "2028-02-29"),
	}

	tests := []struct {
		name  string
		track []string
		sent  []string
		today string
		want  string
	}{
		{"eol only, on the eol trigger", []string{phaseEOL}, nil, "2027-02-28", phaseEOL},
		{"eol only, ignores extended support", []string{phaseEOL}, spent, "2030-02-28", ""},
		{"extended support tracked", []string{phaseEOL, phaseEOES}, spent, "2030-02-28", phaseEOES},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := testConfig(t, Dependency{
				Name:    "Amazon RDS PostgreSQL",
				Product: "amazon-rds-postgresql",
				Track:   tt.track,
			})
			state := tracked("amazon-rds-postgresql", "15")
			state.markSent("amazon-rds-postgresql", tt.sent)

			report, _ := detectAlerts(config, products, state, mustDate(t, tt.today), false)

			if tt.want == "" {
				if len(report.EOL) != 0 {
					t.Fatalf("got %d alert(s), want none", len(report.EOL))
				}
				return
			}

			if len(report.EOL) != 1 {
				t.Fatalf("got %d alert(s), want 1", len(report.EOL))
			}
			if report.EOL[0].Phase != tt.want {
				t.Errorf("phase = %q, want %q", report.EOL[0].Phase, tt.want)
			}
		})
	}
}

// TestDetectAlertsGroupsSharedProduct covers the upstream-fallback mapping: a
// managed service the API does not track rides on the upstream engine, and both
// dependencies must appear on one alert rather than producing two.
func TestDetectAlertsGroupsSharedProduct(t *testing.T) {
	config := testConfig(t,
		Dependency{Name: "Redis", Product: "redis"},
		Dependency{Name: "GCP MemoryStore", Product: "redis", UpstreamProxy: true},
	)
	products := map[string]*Product{
		"redis": {
			Name:     "redis",
			Label:    "Redis",
			Releases: []Release{{Name: "7.2", EOLFrom: strPtr("2027-11-11")}},
		},
	}

	state := tracked("redis", "7.2")
	state.markSent("redis", []string{alertKey("7.2", phaseEOL, "12", "2027-11-11")})

	report, _ := detectAlerts(config, products, state, mustDate(t, "2027-05-11"), false)

	if len(report.EOL) != 1 {
		t.Fatalf("got %d alert(s), want 1 grouped alert", len(report.EOL))
	}

	alert := report.EOL[0]
	if alert.MonthsLeft != 6 {
		t.Errorf("months left = %d, want 6", alert.MonthsLeft)
	}
	if len(alert.Dependencies) != 2 {
		t.Fatalf("got %d dependencies, want Redis and GCP MemoryStore", len(alert.Dependencies))
	}
	if alert.Dependencies[0].Name != "Redis" || alert.Dependencies[0].UpstreamProxy {
		t.Errorf("first dependency = %+v, want Redis tracked directly", alert.Dependencies[0])
	}
	if alert.Dependencies[1].Name != "GCP MemoryStore" || !alert.Dependencies[1].UpstreamProxy {
		t.Errorf("second dependency = %+v, want GCP MemoryStore marked as a proxy", alert.Dependencies[1])
	}
	if !report.hasProxy() {
		t.Error("report does not report a proxy, so the caveat would be omitted")
	}
}

func TestDetectAlertsNewVersions(t *testing.T) {
	products := map[string]*Product{
		"postgresql": {
			Name:  "postgresql",
			Label: "PostgreSQL",
			Releases: []Release{
				{Name: "18", ReleaseDate: "2025-09-25", EOLFrom: strPtr("2030-11-14")},
				{Name: "17", ReleaseDate: "2024-09-26", EOLFrom: strPtr("2029-11-08")},
				{Name: "13", ReleaseDate: "2020-09-24", EOLFrom: strPtr("2025-11-13"), IsEOL: true},
			},
		},
	}

	tests := []struct {
		name  string
		state State
		seed  bool
		want  []string
	}{
		{
			name:  "seed run announces nothing",
			state: State{},
			seed:  true,
			want:  nil,
		},
		{
			name:  "unseen release is announced",
			state: tracked("postgresql", "17"),
			want:  []string{"18"},
		},
		{
			name:  "everything already seen is silent",
			state: tracked("postgresql", "13", "17", "18"),
			want:  nil,
		},
		{
			// A release that was already dead when it first appeared is a
			// backfill, not news. It still gets an end-of-life alert saying so.
			name:  "backfilled dead release is not announced as new",
			state: tracked("postgresql", "17", "18"),
			want:  nil,
		},
		{
			// Adding a dependency to the config must not announce that product's
			// entire back catalogue on the next run.
			name:  "product with no recorded history is a baseline",
			state: tracked("redis", "8.8"),
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := testConfig(t, Dependency{Name: "PostgreSQL", Product: "postgresql"})

			report, _ := detectAlerts(config, products, tt.state, mustDate(t, "2026-07-29"), tt.seed)

			if len(report.NewVersions) != len(tt.want) {
				t.Fatalf("got %d new version(s), want %d", len(report.NewVersions), len(tt.want))
			}
			for i, want := range tt.want {
				if report.NewVersions[i].Release != want {
					t.Errorf("new version %d = %q, want %q", i, report.NewVersions[i].Release, want)
				}
			}
		})
	}
}

func TestProductOrderDeduplicates(t *testing.T) {
	config := testConfig(t,
		Dependency{Name: "Redis", Product: "redis"},
		Dependency{Name: "PostgreSQL", Product: "postgresql"},
		Dependency{Name: "GCP MemoryStore", Product: "redis", UpstreamProxy: true},
	)

	got := productOrder(config)
	want := []string{"redis", "postgresql"}

	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
