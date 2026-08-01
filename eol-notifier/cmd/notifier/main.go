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

	if err := publish(ctx, opts.slack, report, opts.dryRun); err != nil {
		return err
	}

	// State is written only after a successful publish: recording a release the
	// channel never heard about would silence it forever.
	if opts.dryRun {
		log("dry run, leaving %s untouched", opts.statePath)
	} else {
		for name, product := range products {
			state.record(name, product.Releases)
		}
		if err := saveState(opts.statePath, state); err != nil {
			return err
		}
	}

	return runProblems(failed, unreadable)
}

// runProblems turns the problems a run survived into its exit status. They are
// reported only here, after the digest went out and the state was written: the
// alerts that did work should still reach the channel, but a product that could
// not be read and a date that could not be parsed both cost a release its alert
// on the one day it was due, so the job has to go red or the loss is invisible.
func runProblems(failed, unreadable []string) error {
	var problems []string

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

// publish renders and delivers the digest. Nothing to report means nothing is
// posted at all.
func publish(ctx context.Context, slack *slackClient, report Report, dryRun bool) error {
	if report.Empty() {
		log("nothing to report, no Slack message sent")
		return nil
	}

	message, truncated := buildMessage(report)

	payload, err := encodeMessage(message)
	if err != nil {
		return err
	}

	// The truncated digest tells its readers the full list is in the run log, so
	// it has to actually be there, dry run or not.
	if truncated {
		logWarn("digest did not fit Slack's block limit, posting a cut version. full payload:\n%s", payload)
	}

	if dryRun {
		log("dry run, would post to Slack:\n%s", payload)
		return nil
	}

	if err := slack.Post(ctx, payload); err != nil {
		return err
	}

	log("posted digest to Slack")

	return nil
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
