package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// runTimeout bounds the whole run: a handful of small GETs plus one webhook post.
const runTimeout = 5 * time.Minute

// options carries everything a run depends on. The clock and both clients are
// fields, so tests can drive the whole flow against fake servers.
type options struct {
	configPath string
	statePath  string
	dryRun     bool
	now        time.Time
	client     *Client
	slack      *slackClient
}

func main() {
	if err := run(); err != nil {
		logError("%v", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "Path to the dependency config YAML")
	statePath := flag.String("state", "", "Path to the JSON file recording previously seen releases")
	dryRun := flag.Bool("dry-run", false, "Render the digest to stdout instead of posting it, and leave the state file untouched")
	flag.Parse()

	if strings.TrimSpace(*configPath) == "" {
		return fmt.Errorf("failed to start, --config is required")
	}
	if strings.TrimSpace(*statePath) == "" {
		return fmt.Errorf("failed to start, --state is required")
	}

	config, err := loadConfig(envconfig.Process, *dryRun)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	return execute(ctx, options{
		configPath: *configPath,
		statePath:  *statePath,
		dryRun:     *dryRun,
		now:        time.Now(),
		client:     NewClient(),
		slack:      newSlackClient(config.SlackWebhookURL),
	})
}

func execute(ctx context.Context, opts options) error {
	dependencyConfig, err := loadDependencyConfig(opts.configPath)
	if err != nil {
		return err
	}

	state, existed, err := loadState(opts.statePath)
	if err != nil {
		return err
	}
	if !existed {
		log("no state file at %s, recording a baseline without announcing new versions", opts.statePath)
	}

	products, failed := fetchProducts(ctx, opts.client, productOrder(dependencyConfig))

	report, unreadable := detectAlerts(dependencyConfig, products, state, opts.now, !existed)
	log("found %d new version(s) and %d end-of-life alert(s)", len(report.NewVersions), len(report.EOL))

	delivered, publishErr := publish(ctx, opts.slack, report, opts.dryRun)

	// Only what reached the channel is recorded, and it is recorded even when a
	// later part of the digest failed: an alert the channel never heard is due
	// again tomorrow, and one it did hear must not be repeated.
	if opts.dryRun {
		log("dry run, leaving %s untouched", opts.statePath)
	} else {
		recordRun(state, products, report, delivered)
		if err := saveState(opts.statePath, state); err != nil {
			return err
		}
	}

	return runProblems(publishErr, failed, unreadable)
}

// recordRun folds a finished run into the state: the alerts that were delivered,
// the ones a product with no history was due but never had posted, and the
// release cycles of the products that owe the channel nothing.
func recordRun(state State, products map[string]*Product, report Report, delivered []Report) {
	for product, keys := range report.BaselineKeys {
		state.markSent(product, keys)
	}

	for _, part := range delivered {
		for _, alert := range part.EOL {
			state.markSent(alert.Product, []string{alert.key})
		}
	}

	owed := undeliveredProducts(report, delivered)
	for name, product := range products {
		if owed[name] {
			continue
		}
		state.record(name, product.Releases)
	}
}

// undeliveredProducts names the products still owed a new-version alert. Their
// release cycles must stay unrecorded: a release marked as seen is never
// announced again, so recording one whose alert never landed would lose it.
func undeliveredProducts(report Report, delivered []Report) map[string]bool {
	landed := make(map[string]bool)
	for _, part := range delivered {
		for _, alert := range part.NewVersions {
			landed[alert.Product+"|"+alert.Release] = true
		}
	}

	owed := make(map[string]bool)
	for _, alert := range report.NewVersions {
		if !landed[alert.Product+"|"+alert.Release] {
			owed[alert.Product] = true
		}
	}

	return owed
}

// runProblems turns the problems a run survived into its exit status. They are
// reported only here, after the digest went out and the state was written: the
// alerts that did work should still reach the channel, but each of these delays
// a release its alert, so the job has to go red or the delay is invisible.
func runProblems(publishErr error, failed, unreadable []string) error {
	var problems []string

	if publishErr != nil {
		problems = append(problems, publishErr.Error())
	}
	if len(failed) > 0 {
		problems = append(problems, fmt.Sprintf(
			"failed to fetch %d product(s): %s", len(failed), strings.Join(failed, ", "),
		))
	}
	if len(unreadable) > 0 {
		problems = append(problems, fmt.Sprintf(
			"skipped %d release(s) with an unreadable date: %s", len(unreadable), strings.Join(unreadable, ", "),
		))
	}

	if len(problems) == 0 {
		return nil
	}

	return errors.New(strings.Join(problems, "; "))
}

// fetchProducts polls every distinct product once. A product that cannot be
// fetched is reported rather than fatal, so one broken slug does not suppress
// alerts for everything else.
func fetchProducts(ctx context.Context, client *Client, names []string) (map[string]*Product, []string) {
	products := make(map[string]*Product, len(names))
	var failed []string

	for _, name := range names {
		product, err := client.FetchProduct(ctx, name)
		if err != nil {
			logWarn("%v", err)
			failed = append(failed, name)
			continue
		}

		log("fetched %s (%d release cycles)", name, len(product.Releases))
		products[name] = product
	}

	return products, failed
}

// publish delivers the digest, in as many messages as Slack's block limit
// requires, and returns the parts that landed. A part that fails costs only
// itself: the parts before it are reported as delivered and the rest stay due.
// Nothing to report means nothing is posted at all.
func publish(ctx context.Context, slack *slackClient, report Report, dryRun bool) ([]Report, error) {
	if report.Empty() {
		log("nothing to report, no Slack message sent")
		return nil, nil
	}

	parts := splitReport(report)
	if len(parts) > 1 {
		log("digest does not fit one Slack message, posting it in %d parts", len(parts))
	}

	var delivered []Report
	for _, part := range parts {
		payload, err := encodeMessage(buildMessage(part))
		if err != nil {
			return delivered, err
		}

		if dryRun {
			log("dry run, would post to Slack:\n%s", payload)
			continue
		}

		if err := slack.Post(ctx, payload); err != nil {
			return delivered, err
		}
		delivered = append(delivered, part)
	}

	if !dryRun {
		log("posted digest to Slack in %d message(s)", len(delivered))
	}

	return delivered, nil
}

func log(msg string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stdout, "[INFO] %s\n", format(msg, args...))
}

func logWarn(msg string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stdout, "[WARN] %s\n", format(msg, args...))
}

func logError(msg string, args ...interface{}) {
	line := format(msg, args...)

	_, _ = fmt.Fprintf(os.Stdout, "[ERROR] %s\n", line)
	_, _ = fmt.Fprintln(os.Stderr, line)
}

// format expands the arguments only when there are some. Without this guard a
// message carrying a literal % - an error text passed straight in, say - would be
// read as a format string and printed as %!s(MISSING) noise, corrupting the only
// diagnostic the operator gets.
func format(msg string, args ...interface{}) string {
	if len(args) == 0 {
		return msg
	}

	return fmt.Sprintf(msg, args...)
}
