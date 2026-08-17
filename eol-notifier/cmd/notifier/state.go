package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
)

// State records what previous runs saw and reported, keyed by product slug. It
// is what makes both detections possible: the API carries no notion of when a
// cycle was added to it, and no notion of what has already been announced.
type State map[string]productState

// productState is one product's history.
type productState struct {
	// Releases lists the release cycles the last run saw.
	Releases []string `json:"releases"`
	// Sent lists the keys of the alerts already delivered for this product. It
	// is never pruned: an entry is the only thing standing between a delivered
	// alert and a repeat of it, and an alert is due until it has landed however
	// long the action was not running.
	Sent []string `json:"sent"`
}

// loadState reads the state file. A missing file is not an error: it marks a
// seed run, reported through the second return value.
func loadState(path string) (State, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to read state file %s: %w", path, err)
	}

	state := State{}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, false, fmt.Errorf("failed to parse state file %s: %w", path, err)
	}

	return state, true, nil
}

// saveState writes the state file with everything sorted, so a run that changes
// nothing produces no diff.
func saveState(path string, state State) error {
	for _, recorded := range state {
		sort.Strings(recorded.Releases)
		sort.Strings(recorded.Sent)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode state: %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write state file %s: %w", path, err)
	}

	return nil
}

// has reports whether the product has ever been recorded. A product that is
// absent has never been polled before, so its existing releases are a baseline
// rather than news.
func (s State) has(product string) bool {
	_, ok := s[product]

	return ok
}

// seen returns the release names already recorded for a product.
func (s State) seen(product string) map[string]bool {
	releases := make(map[string]bool, len(s[product].Releases))
	for _, release := range s[product].Releases {
		releases[release] = true
	}

	return releases
}

// sent returns the keys of the alerts already delivered for a product.
func (s State) sent(product string) map[string]bool {
	keys := make(map[string]bool, len(s[product].Sent))
	for _, key := range s[product].Sent {
		keys[key] = true
	}

	return keys
}

// record replaces the recorded releases for a product.
func (s State) record(product string, releases []Release) {
	names := make([]string, 0, len(releases))
	for _, release := range releases {
		names = append(names, release.Name)
	}
	sort.Strings(names)

	recorded := s[product]
	recorded.Releases = names
	s[product] = recorded
}

// markSent records alert keys as delivered, ignoring the ones already there.
func (s State) markSent(product string, keys []string) {
	recorded := s[product]
	already := s.sent(product)

	for _, key := range keys {
		if already[key] {
			continue
		}
		already[key] = true
		recorded.Sent = append(recorded.Sent, key)
	}

	s[product] = recorded
}
