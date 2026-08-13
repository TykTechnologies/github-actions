package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const dateLayout = "2006-01-02"

// endedMarker stands in for the threshold count in the key of an alert that
// reports a phase as over rather than approaching.
const endedMarker = "ended"

// DependencyRef names a dependency affected by an alert. Several dependencies
// can share one product, so alerts are grouped and list every dependency the
// product stands in for.
type DependencyRef struct {
	Name          string
	UpstreamProxy bool
}

// EOLAlert reports that a release cycle reaches the end of a lifecycle phase in
// MonthsLeft months, or that it already has when Ended is set.
type EOLAlert struct {
	Product      string
	ProductLabel string
	ProductURL   string
	Release      string
	Phase        string
	PhaseLabel   string
	Date         time.Time
	MonthsLeft   int
	Ended        bool
	Dependencies []DependencyRef
	// keys identify the alert in the state file, so it is delivered once and
	// stays due until it has been. There is more than one when this alert stands
	// in for thresholds it superseded, which are spent along with it.
	keys []string
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
	// BaselineKeys lists, per product, the alert keys a product with no recorded
	// history was already due. They are recorded as delivered without being
	// posted: a product added to the config must not empty years of elapsed
	// thresholds into the channel.
	BaselineKeys map[string][]string
	// Seed is true when no previous state existed, in which case new-version
	// detection is suppressed to avoid announcing every historical release.
	Seed bool
}

// Empty reports whether there is nothing worth posting.
func (r Report) Empty() bool {
	return r.alertCount() == 0
}

// alertCount is how many alerts the report carries, of either kind.
func (r Report) alertCount() int {
	return len(r.NewVersions) + len(r.EOL)
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
// and what previous runs recorded. Products missing from products (because their
// fetch failed) are skipped. now is passed in rather than read from the clock so
// the whole detection path is testable.
//
// An alert is due from the day it comes up until the day it is delivered, not on
// one day only: a run that does not happen must not cost the channel a warning,
// however long the gap. What has already been delivered is read from the state,
// so an alert still fires exactly once.
//
// seed only drives the wording of the report; whether a product's releases count
// as new is decided per product, so adding a dependency to the config does not
// announce that product's entire back catalogue. What it does announce is every
// warning that product is still counting down to, missed days included: a
// version weeks from the end of its support is the first thing a new tracker
// owes the channel, not something to swallow as history.
//
// Releases carrying a date the API states in a form we cannot read are returned
// through the second value rather than only logged. Dropping one quietly would
// leave the job green with nobody a reason to look.
func detectAlerts(config *DependencyConfig, products map[string]*Product, state State, now time.Time, seed bool) (Report, []string) {
	today := truncateToDay(now)
	report := Report{BaselineKeys: map[string][]string{}, Seed: seed}
	var unreadable []string

	for _, name := range productOrder(config) {
		product, ok := products[name]
		if !ok {
			continue
		}

		seen := state.seen(name)
		sent := state.sent(name)

		// A product with no recorded history is a baseline: its releases are not
		// news, and the phases that ended before anyone was watching are not
		// either. What it is still counting down to is news, however, and is
		// announced like any other warning. This covers both the first ever run
		// and a product newly added to the config.
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

				value, ok := release.phase(phase)
				if !ok {
					continue
				}

				date, err := time.Parse(dateLayout, value)
				if err != nil {
					logWarn("skipping %s %s: unparseable %s date %q", name, release.Name, phase, value)
					unreadable = append(unreadable, fmt.Sprintf("%s %s (%s date %q)", name, release.Name, phase, value))
					continue
				}

				owed := unsent(dueAlerts(release.Name, phase, date, today, config.ThresholdsMonths), sent)
				if len(owed) == 0 {
					continue
				}

				// dueAlerts returns the ended alert on its own, so the first entry
				// answers for the whole phase.
				if baseline && owed[0].ended {
					report.BaselineKeys[name] = append(report.BaselineKeys[name], owed[0].key)
					continue
				}

				due, superseded := mostUrgent(owed)

				report.EOL = append(report.EOL, EOLAlert{
					Product:      name,
					ProductLabel: product.Label,
					ProductURL:   product.Links.HTML,
					Release:      releaseLabel(release),
					Phase:        phase,
					PhaseLabel:   product.phaseLabel(phase),
					Date:         date,
					MonthsLeft:   due.months,
					Ended:        due.ended,
					Dependencies: dependencies,
					keys:         append([]string{due.key}, superseded...),
				})
			}
		}
	}

	sort.SliceStable(report.EOL, func(i, j int) bool {
		if report.EOL[i].Ended != report.EOL[j].Ended {
			return report.EOL[i].Ended
		}

		return report.EOL[i].MonthsLeft > report.EOL[j].MonthsLeft
	})

	return report, unreadable
}

// dueAlert is one alert a phase owes: a threshold that has come up, or the phase
// having ended outright.
type dueAlert struct {
	months int
	ended  bool
	key    string
}

// dueAlerts returns what a phase owes as of today, whether it came up today or
// on a day no run happened. Once the date itself has passed the phase has simply
// ended, and the thresholds counting down to it would only say something untrue,
// so the ended alert stands for them.
//
// The date is part of every key, so a date the API later revises is due afresh
// rather than silenced by the alert sent for the date it replaced.
func dueAlerts(cycle, phase string, date, today time.Time, thresholds []int) []dueAlert {
	stamp := date.Format(dateLayout)

	if !date.After(today) {
		return []dueAlert{{ended: true, key: alertKey(cycle, phase, endedMarker, stamp)}}
	}

	var due []dueAlert
	for _, months := range thresholds {
		if monthsBefore(date, months).After(today) {
			continue
		}

		due = append(due, dueAlert{months: months, key: alertKey(cycle, phase, strconv.Itoa(months), stamp)})
	}

	return due
}

// unsent drops the alerts already delivered.
func unsent(due []dueAlert, sent map[string]bool) []dueAlert {
	owed := make([]dueAlert, 0, len(due))
	for _, candidate := range due {
		if sent[candidate.key] {
			continue
		}
		owed = append(owed, candidate)
	}

	return owed
}

// mostUrgent picks the one alert to send when several thresholds for the same
// phase are owed at once, which happens on the first run and after a gap in
// runs. Only the closest one is still true by then - "twelve months" is a lie
// once the six-month day has passed too - so the rest are returned as
// superseded. They are spent along with the alert that stands in for them
// rather than left to come up again.
func mostUrgent(due []dueAlert) (dueAlert, []string) {
	pick := 0
	for index, candidate := range due {
		if candidate.months < due[pick].months {
			pick = index
		}
	}

	var superseded []string
	for index, candidate := range due {
		if index != pick {
			superseded = append(superseded, candidate.key)
		}
	}

	return due[pick], superseded
}

// alertKey identifies one alert across runs.
func alertKey(cycle, phase, marker, date string) string {
	return strings.Join([]string{cycle, phase, marker, date}, "|")
}

// monthsBefore returns the date exactly months months before date, clamped to
// the last day of the target month. Clamping matters: Go's AddDate turns
// 2027-03-31 minus one month into 2027-03-03, which would hold the one-month
// warning back to three days into the month it was meant to open.
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
