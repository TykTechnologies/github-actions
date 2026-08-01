package main

import (
	"fmt"
	"sort"
	"time"
)

const dateLayout = "2006-01-02"

// DependencyRef names a dependency affected by an alert. Several dependencies
// can share one product, so alerts are grouped and list every dependency the
// product stands in for.
type DependencyRef struct {
	Name          string
	UpstreamProxy bool
}

// EOLAlert reports that a release cycle reaches the end of a lifecycle phase in
// exactly MonthsLeft months.
type EOLAlert struct {
	Product      string
	ProductLabel string
	ProductURL   string
	Release      string
	Phase        string
	PhaseLabel   string
	Date         time.Time
	MonthsLeft   int
	Dependencies []DependencyRef
}

// NewVersionAlert reports a release cycle that the API now lists but the
// previous run did not see.
type NewVersionAlert struct {
	Product      string
	ProductLabel string
	ProductURL   string
	Release      string
	ReleaseDate  string
	IsLTS        bool
	Dependencies []DependencyRef
}

// Report is everything a single run found.
type Report struct {
	NewVersions []NewVersionAlert
	EOL         []EOLAlert
	// Seed is true when no previous state existed, in which case new-version
	// detection is suppressed to avoid announcing every historical release.
	Seed bool
}

// Empty reports whether there is nothing worth posting.
func (r Report) Empty() bool {
	return len(r.NewVersions) == 0 && len(r.EOL) == 0
}

// hasProxy reports whether any alert relies on an upstream engine standing in
// for an untracked managed service.
func (r Report) hasProxy() bool {
	for _, alert := range r.NewVersions {
		if hasProxyRef(alert.Dependencies) {
			return true
		}
	}
	for _, alert := range r.EOL {
		if hasProxyRef(alert.Dependencies) {
			return true
		}
	}

	return false
}

func hasProxyRef(refs []DependencyRef) bool {
	for _, ref := range refs {
		if ref.UpstreamProxy {
			return true
		}
	}

	return false
}

// detectAlerts compares the fetched products against the configured thresholds
// and the previously seen state. Products missing from products (because their
// fetch failed) are skipped. now is passed in rather than read from the clock so
// the whole detection path is testable.
//
// seed only drives the wording of the report; whether a product's releases count
// as new is decided per product, so adding a dependency to the config does not
// announce that product's entire back catalogue.
//
// Releases carrying a date the API states in a form we cannot read are returned
// through the second value rather than only logged. Such a release is due its
// alert on a single day, so dropping it quietly would lose that alert while the
// job stayed green and nobody had a reason to look.
func detectAlerts(config *DependencyConfig, products map[string]*Product, state State, now time.Time, seed bool) (Report, []string) {
	today := truncateToDay(now)
	report := Report{Seed: seed}
	var unreadable []string

	for _, name := range productOrder(config) {
		product, ok := products[name]
		if !ok {
			continue
		}

		seen := state.seen(name)

		// A product with no recorded history is a baseline, not news. This covers
		// both the first ever run and a product newly added to the config.
		baseline := !state.has(name)
		if baseline && !seed {
			log("%s has no recorded history, recording its current releases as a baseline", name)
		}

		for _, release := range product.Releases {
			if !baseline && !seen[release.Name] && !release.IsEOL {
				report.NewVersions = append(report.NewVersions, NewVersionAlert{
					Product:      name,
					ProductLabel: product.Label,
					ProductURL:   product.Links.HTML,
					Release:      releaseLabel(release),
					ReleaseDate:  release.ReleaseDate,
					IsLTS:        release.IsLTS,
					Dependencies: dependenciesFor(config, name, ""),
				})
			}

			for _, phase := range phaseOrder {
				dependencies := dependenciesFor(config, name, phase)
				if len(dependencies) == 0 {
					continue
				}

				value, past, ok := release.phase(phase)
				if !ok || past {
					continue
				}

				date, err := time.Parse(dateLayout, value)
				if err != nil {
					logWarn("skipping %s %s: unparseable %s date %q", name, release.Name, phase, value)
					unreadable = append(unreadable, fmt.Sprintf("%s %s (%s date %q)", name, release.Name, phase, value))
					continue
				}

				for _, months := range config.ThresholdsMonths {
					if !monthsBefore(date, months).Equal(today) {
						continue
					}

					report.EOL = append(report.EOL, EOLAlert{
						Product:      name,
						ProductLabel: product.Label,
						ProductURL:   product.Links.HTML,
						Release:      releaseLabel(release),
						Phase:        phase,
						PhaseLabel:   product.phaseLabel(phase),
						Date:         date,
						MonthsLeft:   months,
						Dependencies: dependencies,
					})
				}
			}
		}
	}

	sort.SliceStable(report.EOL, func(i, j int) bool {
		return report.EOL[i].MonthsLeft > report.EOL[j].MonthsLeft
	})

	return report, unreadable
}

// monthsBefore returns the date exactly months months before date, clamped to
// the last day of the target month. Clamping matters: Go's AddDate turns
// 2027-03-31 minus one month into 2027-03-03, which would make a single EoL
// date trigger the same threshold on more than one day.
func monthsBefore(date time.Time, months int) time.Time {
	year, month, day := date.Date()

	target := time.Date(year, month-time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	if last := daysInMonth(target); day > last {
		day = last
	}

	return time.Date(target.Year(), target.Month(), day, 0, 0, 0, 0, time.UTC)
}

// daysInMonth returns the number of days in the month of date. Day 0 of the
// following month is the last day of this one.
func daysInMonth(date time.Time) int {
	return time.Date(date.Year(), date.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// truncateToDay drops the time of day so dates can be compared for equality.
func truncateToDay(t time.Time) time.Time {
	year, month, day := t.UTC().Date()

	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// productOrder lists the distinct products in the order they first appear in
// the config, so a shared product is fetched and reported once.
func productOrder(config *DependencyConfig) []string {
	seen := make(map[string]bool, len(config.Dependencies))
	order := make([]string, 0, len(config.Dependencies))

	for _, dependency := range config.Dependencies {
		if seen[dependency.Product] {
			continue
		}
		seen[dependency.Product] = true
		order = append(order, dependency.Product)
	}

	return order
}

// dependenciesFor returns the dependencies backed by a product. An empty phase
// matches every dependency, which is what new-version alerts want; otherwise
// only the dependencies configured to track that phase are returned.
func dependenciesFor(config *DependencyConfig, product, phase string) []DependencyRef {
	var refs []DependencyRef

	for _, dependency := range config.Dependencies {
		if dependency.Product != product {
			continue
		}
		if phase != "" && !dependency.tracks(phase) {
			continue
		}

		refs = append(refs, DependencyRef{Name: dependency.Name, UpstreamProxy: dependency.UpstreamProxy})
	}

	return refs
}

// releaseLabel prefers the API's display label, which is occasionally friendlier
// than the raw cycle name.
func releaseLabel(release Release) string {
	if release.Label != "" {
		return release.Label
	}

	return release.Name
}
