package main

import (
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

// TestThresholdFiresOnExactlyOneDay is the guard against the failure mode that
// motivated the clamping: a single EoL date must not alert on more than one day,
// and must not slip through unannounced either.
func TestThresholdFiresOnExactlyOneDay(t *testing.T) {
	eolDates := []string{"2027-01-31", "2027-02-28", "2027-03-31", "2028-02-29", "2027-08-31"}

	for _, eol := range eolDates {
		t.Run(eol, func(t *testing.T) {
			config := testConfig(t, Dependency{Name: "PostgreSQL", Product: "postgresql"})
			products := map[string]*Product{
				"postgresql": {
					Name:     "postgresql",
					Label:    "PostgreSQL",
					Releases: []Release{{Name: "15", EOLFrom: strPtr(eol)}},
				},
			}

			for _, months := range config.ThresholdsMonths {
				trigger := monthsBefore(mustDate(t, eol), months)

				fired := 0
				for offset := -20; offset <= 20; offset++ {
					day := trigger.AddDate(0, 0, offset)
					report, _ := detectAlerts(config, products, State{}, day, false)
					for _, alert := range report.EOL {
						if alert.MonthsLeft == months {
							fired++
						}
					}
				}

				if fired != 1 {
					t.Errorf("threshold %d months fired on %d days around %s, want exactly 1",
						months, fired, trigger.Format(dateLayout))
				}
			}
		})
	}
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

	tests := []struct {
		name     string
		products map[string]*Product
		today    string
		want     int
	}{
		{
			name:     "fires twelve months ahead",
			products: postgres(Release{Name: "15", EOLFrom: strPtr("2027-11-11")}),
			today:    "2026-11-11",
			want:     1,
		},
		{
			name:     "fires one month ahead",
			products: postgres(Release{Name: "15", EOLFrom: strPtr("2027-11-11")}),
			today:    "2027-10-11",
			want:     1,
		},
		{
			name:     "silent a day early",
			products: postgres(Release{Name: "15", EOLFrom: strPtr("2027-11-11")}),
			today:    "2027-10-10",
			want:     0,
		},
		{
			name:     "silent a day late",
			products: postgres(Release{Name: "15", EOLFrom: strPtr("2027-11-11")}),
			today:    "2027-10-12",
			want:     0,
		},
		{
			name:     "skips a release with no announced date",
			products: postgres(Release{Name: "18", EOLFrom: nil}),
			today:    "2026-11-11",
			want:     0,
		},
		{
			name:     "skips a phase that has already passed",
			products: postgres(Release{Name: "13", EOLFrom: strPtr("2027-11-11"), IsEOL: true}),
			today:    "2026-11-11",
			want:     0,
		},
		{
			name:     "skips a product whose fetch failed",
			products: map[string]*Product{},
			today:    "2026-11-11",
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := testConfig(t, Dependency{Name: "PostgreSQL", Product: "postgresql"})

			report, _ := detectAlerts(config, tt.products, State{}, mustDate(t, tt.today), false)

			if got := len(report.EOL); got != tt.want {
				t.Fatalf("got %d EoL alert(s), want %d", got, tt.want)
			}
			if tt.want > 0 && report.EOL[0].PhaseLabel != "Support Status" {
				t.Errorf("phase label = %q, want the product's own wording", report.EOL[0].PhaseLabel)
			}
		})
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

	report, unreadable := detectAlerts(config, products, State{}, mustDate(t, "2026-11-11"), false)

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

	tests := []struct {
		name  string
		track []string
		today string
		want  string
	}{
		{"eol only, on the eol trigger", []string{phaseEOL}, "2027-02-28", phaseEOL},
		{"eol only, ignores extended support", []string{phaseEOL}, "2030-02-28", ""},
		{"extended support tracked", []string{phaseEOL, phaseEOES}, "2030-02-28", phaseEOES},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := testConfig(t, Dependency{
				Name:    "Amazon RDS PostgreSQL",
				Product: "amazon-rds-postgresql",
				Track:   tt.track,
			})

			report, _ := detectAlerts(config, products, State{}, mustDate(t, tt.today), false)

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

	report, _ := detectAlerts(config, products, State{}, mustDate(t, "2027-05-11"), false)

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
			state: State{"postgresql": {"17"}},
			want:  []string{"18"},
		},
		{
			name:  "everything already seen is silent",
			state: State{"postgresql": {"13", "17", "18"}},
			want:  nil,
		},
		{
			name:  "backfilled dead release is not announced",
			state: State{"postgresql": {"17", "18"}},
			want:  nil,
		},
		{
			// Adding a dependency to the config must not announce that product's
			// entire back catalogue on the next run.
			name:  "product with no recorded history is a baseline",
			state: State{"redis": {"8.8"}},
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
