package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Lifecycle phases exposed by the endoflife.date v1 API. Not every product
// carries every phase; see the per-product "labels" object in the response.
const (
	phaseEOL  = "eol"  // end of life / security support
	phaseEOAS = "eoas" // end of active support
	phaseEOES = "eoes" // end of extended support
)

// phaseOrder fixes the order alerts are emitted in, so output is deterministic.
var phaseOrder = []string{phaseEOL, phaseEOAS, phaseEOES}

// defaultThresholdsMonths matches the EoL policy: flag a version 12, 6 and 1
// month before it reaches the end of a lifecycle phase.
var defaultThresholdsMonths = []int{12, 6, 1}

// Config holds the environment configuration, loaded with the EN_ prefix.
type Config struct {
	SlackWebhookURL string `envconfig:"SLACK_WEBHOOK_URL"`
}

// Dependency is a single-tracked dependency. Several dependencies may share a
// product: GCP MemoryStore is not tracked by endoflife.date, so it rides on the
// upstream redis lifecycle.
type Dependency struct {
	// Name is how the dependency is referred to in the alert.
	Name string `yaml:"name"`
	// Product is the endoflife.date product slug to poll.
	Product string `yaml:"product"`
	// Track lists the lifecycle phases to alert on. Defaults to [eol].
	Track []string `yaml:"track"`
	// UpstreamProxy marks a dependency whose lifecycle is not published by its
	// vendor, so the upstream OSS engine is used as a stand-in. Its dates are
	// indicative only and are labelled as such in the alert.
	UpstreamProxy bool `yaml:"upstream_proxy"`
}

// DependencyConfig is the YAML config file supplied by the caller.
type DependencyConfig struct {
	ThresholdsMonths []int        `yaml:"thresholds_months"`
	Dependencies     []Dependency `yaml:"dependencies"`
}

// loadConfig loads and validates the environment configuration.
func loadConfig(process func(string, interface{}) error, dryRun bool) (*Config, error) {
	var config Config
	if err := process("EN", &config); err != nil {
		return nil, fmt.Errorf("failed to load environment configuration: %w", err)
	}

	if !dryRun && strings.TrimSpace(config.SlackWebhookURL) == "" {
		return nil, fmt.Errorf("Slack webhook URL is required to post alerts (EN_SLACK_WEBHOOK_URL)")
	}

	return &config, nil
}

// loadDependencyConfig reads and validates the YAML dependency config.
func loadDependencyConfig(path string) (*DependencyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read dependency config %s: %w", path, err)
	}

	var config DependencyConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse dependency config %s: %w", path, err)
	}

	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("failed to validate dependency config %s: %w", path, err)
	}

	return &config, nil
}

// validate applies defaults and rejects configs that would produce confusing or
// silently empty alerts. It runs before any network call.
func (c *DependencyConfig) validate() error {
	if len(c.ThresholdsMonths) == 0 {
		c.ThresholdsMonths = append([]int(nil), defaultThresholdsMonths...)
	}

	seenThreshold := make(map[int]bool, len(c.ThresholdsMonths))
	for _, months := range c.ThresholdsMonths {
		if months <= 0 {
			return fmt.Errorf("threshold must be a positive number of months, got %d", months)
		}
		if seenThreshold[months] {
			return fmt.Errorf("duplicate threshold %d", months)
		}
		seenThreshold[months] = true
	}

	if len(c.Dependencies) == 0 {
		return fmt.Errorf("at least one dependency must be configured")
	}

	seenName := make(map[string]bool, len(c.Dependencies))
	for i := range c.Dependencies {
		dep := &c.Dependencies[i]

		dep.Name = strings.TrimSpace(dep.Name)
		if dep.Name == "" {
			return fmt.Errorf("dependency %d has an empty name", i+1)
		}
		if seenName[dep.Name] {
			return fmt.Errorf("duplicate dependency name %q", dep.Name)
		}
		seenName[dep.Name] = true

		dep.Product = strings.TrimSpace(dep.Product)
		if dep.Product == "" {
			return fmt.Errorf("dependency %q has an empty product", dep.Name)
		}

		if len(dep.Track) == 0 {
			dep.Track = []string{phaseEOL}
		}
		seenPhase := make(map[string]bool, len(dep.Track))
		for _, phase := range dep.Track {
			if !isKnownPhase(phase) {
				return fmt.Errorf(
					"dependency %q tracks unknown phase %q, must be one of: %s",
					dep.Name, phase, strings.Join(phaseOrder, ", "),
				)
			}
			if seenPhase[phase] {
				return fmt.Errorf("dependency %q tracks phase %q more than once", dep.Name, phase)
			}
			seenPhase[phase] = true
		}
	}

	return nil
}

// tracks reports whether the dependency is configured for the given phase.
func (d Dependency) tracks(phase string) bool {
	for _, tracked := range d.Track {
		if tracked == phase {
			return true
		}
	}

	return false
}

func isKnownPhase(phase string) bool {
	for _, known := range phaseOrder {
		if phase == known {
			return true
		}
	}

	return false
}
