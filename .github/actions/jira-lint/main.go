package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// JiraValidator handles all Jira validation logic
type JiraValidator struct {
	jiraToken           string
	jiraEmail           string
	jiraBaseURL         string
	skipBranches        string
	validateIssueStatus bool
}

// ValidationResult holds the result of Jira issue validation
type ValidationResult struct {
	Valid    bool
	IssueKey string
	Error    string
}

// PullRequest represents the GitHub PR data we need
type PullRequest struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Number int    `json:"number"`
	Head   struct {
		Ref string `json:"ref"`
	} `json:"head"`
}

// Repository represents the GitHub repository data
type Repository struct {
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
	Name string `json:"name"`
}

// GitHubEvent represents the GitHub event payload
type GitHubEvent struct {
	PullRequest *PullRequest `json:"pull_request"`
	Repository  *Repository  `json:"repository"`
}

// Jira issue patterns
var jiraPatterns = []*regexp.Regexp{
	regexp.MustCompile(`[A-Z]{2}-[0-9]{4,5}`), // TT-0000 through TT-99999
	regexp.MustCompile(`SYSE-[0-9]+`),         // SYSE-339 pattern
	regexp.MustCompile(`[A-Z]+-[0-9]+`),       // General pattern for other projects
}

// decodeJiraToken decodes the base64 encoded jira token and extracts email and token
func decodeJiraToken(encodedToken string) (email, token string, err error) {
	decoded, err := base64.StdEncoding.DecodeString(encodedToken)
	if err != nil {
		return "", "", fmt.Errorf("failed to decode base64 token: %v", err)
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid token format, expected email:token")
	}

	return parts[0], parts[1], nil
}

// NewJiraValidator creates a new JiraValidator instance
func NewJiraValidator() (*JiraValidator, error) {
	validateIssueStatus, _ := strconv.ParseBool(os.Getenv("INPUT_VALIDATE_ISSUE_STATUS"))

	// Decode the base64 encoded JIRA token
	encodedToken := os.Getenv("INPUT_JIRA_TOKEN")
	email, token, err := decodeJiraToken(encodedToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JIRA token: %v", err)
	}

	return &JiraValidator{
		jiraToken:           token,
		jiraEmail:           email,
		jiraBaseURL:         os.Getenv("INPUT_JIRA_BASE_URL"),
		skipBranches:        os.Getenv("INPUT_SKIP_BRANCHES"),
		validateIssueStatus: validateIssueStatus,
	}, nil
}

// extractJiraIssues extracts Jira issue IDs from text
func (jv *JiraValidator) extractJiraIssues(text string) []string {
	if text == "" {
		return []string{}
	}

	issuesMap := make(map[string]bool)
	for _, pattern := range jiraPatterns {
		matches := pattern.FindAllString(text, -1)
		for _, match := range matches {
			issuesMap[strings.ToUpper(match)] = true
		}
	}

	issues := make([]string, 0, len(issuesMap))
	for issue := range issuesMap {
		issues = append(issues, issue)
	}
	return issues
}

// validateJiraIssueWithCLI validates a Jira issue using the existing jira-lint tool
func (jv *JiraValidator) validateJiraIssueWithCLI(issueKey string) ValidationResult {
	// Set environment variables for jira-lint
	env := os.Environ()
	env = append(env, fmt.Sprintf("JIRA_API_TOKEN=%s", jv.jiraToken))
	env = append(env, fmt.Sprintf("JIRA_API_EMAIL=%s", jv.jiraEmail))
	env = append(env, fmt.Sprintf("JIRA_API_URL=%s", jv.jiraBaseURL))

	// Execute jira-lint command
	cmd := exec.Command("jira-lint", issueKey)
	cmd.Env = env

	output, err := cmd.CombinedOutput()

	if err != nil {
		// Check if it's a validation error (non-zero exit code)
		if _, ok := err.(*exec.ExitError); ok {
			return ValidationResult{
				Valid:    false,
				IssueKey: issueKey,
				Error:    string(output),
			}
		}
		// Other errors (command not found, etc.)
		return ValidationResult{
			Valid:    false,
			IssueKey: issueKey,
			Error:    fmt.Sprintf("Failed to execute jira-lint: %v", err),
		}
	}

	// Success - issue is valid and in allowed state
	return ValidationResult{
		Valid:    true,
		IssueKey: issueKey,
	}
}

// shouldSkipBranch checks if the branch should be skipped based on regex pattern
func (jv *JiraValidator) shouldSkipBranch(branchName string) bool {
	if jv.skipBranches == "" {
		return false
	}

	regex, err := regexp.Compile(jv.skipBranches)
	if err != nil {
		fmt.Printf("::warning::Invalid skip-branches regex: %v\n", err)
		return false
	}

	return regex.MatchString(branchName)
}

// generateValidationReport generates a markdown report of the validation results
func (jv *JiraValidator) generateValidationReport(titleIssues, branchIssues, bodyIssues []string, validationResults []ValidationResult) string {
	report := "<!-- jira-lint-comment -->\n## 🔍 Jira Issue Validation Report\n\n"

	// Title validation
	report += "### PR Title\n"
	if len(titleIssues) == 0 {
		report += "❌ **No Jira issue found in PR title** (Required)\n\n"
	} else {
		report += fmt.Sprintf("✅ Found Jira issue(s): %s\n\n", strings.Join(titleIssues, ", "))
	}

	// Branch validation
	report += "### Branch Name\n"
	if len(branchIssues) == 0 {
		report += "⚠️ No Jira issue found in branch name\n\n"
	} else {
		report += fmt.Sprintf("✅ Found Jira issue(s): %s\n\n", strings.Join(branchIssues, ", "))
	}

	// Body validation
	report += "### PR Body\n"
	if len(bodyIssues) == 0 {
		report += "⚠️ No Jira issue found in PR body\n\n"
	} else {
		report += fmt.Sprintf("✅ Found Jira issue(s): %s\n\n", strings.Join(bodyIssues, ", "))
	}

	// Consistency check
	allIssues := make(map[string]bool)
	for _, issue := range titleIssues {
		allIssues[issue] = true
	}
	for _, issue := range branchIssues {
		allIssues[issue] = true
	}
	for _, issue := range bodyIssues {
		allIssues[issue] = true
	}

	if len(allIssues) > 1 {
		report += "### ⚠️ Consistency Warning\n"
		report += "Multiple different Jira issues found across title, branch, and body. Please ensure consistency.\n\n"
	}

	// Validation results
	if len(validationResults) > 0 {
		report += "### Jira Issue Details\n"
		for _, result := range validationResults {
			if result.Valid {
				report += fmt.Sprintf("✅ **%s**: Valid issue and status\n", result.IssueKey)
			} else {
				report += fmt.Sprintf("❌ **%s**: %s\n", result.IssueKey, result.Error)
			}
		}
	}

	return report
}

// setGitHubOutput sets a GitHub Actions output
func setGitHubOutput(name, value string) {
	fmt.Printf("::set-output name=%s::%s\n", name, value)
}

// run executes the main validation logic
func (jv *JiraValidator) run() error {
	// Get GitHub event data
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	if eventPath == "" {
		return fmt.Errorf("GITHUB_EVENT_PATH not set")
	}

	eventData, err := os.ReadFile(eventPath)
	if err != nil {
		return fmt.Errorf("failed to read event file: %v", err)
	}

	var event GitHubEvent
	if err := json.Unmarshal(eventData, &event); err != nil {
		return fmt.Errorf("failed to parse event data: %v", err)
	}

	if event.PullRequest == nil {
		return fmt.Errorf("this action can only be run on pull requests")
	}

	pr := event.PullRequest

	// Check if we should skip this branch
	if jv.shouldSkipBranch(pr.Head.Ref) {
		fmt.Printf("Skipping validation for branch: %s\n", pr.Head.Ref)
		setGitHubOutput("status", "skipped")
		return nil
	}

	// Extract Jira issues from different sources
	titleIssues := jv.extractJiraIssues(pr.Title)
	branchIssues := jv.extractJiraIssues(pr.Head.Ref)
	bodyIssues := jv.extractJiraIssues(pr.Body)

	fmt.Printf("Title issues: %s\n", strings.Join(titleIssues, ", "))
	fmt.Printf("Branch issues: %s\n", strings.Join(branchIssues, ", "))
	fmt.Printf("Body issues: %s\n", strings.Join(bodyIssues, ", "))

	// Validate that PR title contains a Jira issue (mandatory)
	if len(titleIssues) == 0 {
		report := jv.generateValidationReport(titleIssues, branchIssues, bodyIssues, []ValidationResult{})
		setGitHubOutput("report", report)
		setGitHubOutput("status", "failure")
		return fmt.Errorf("PR title must contain a valid Jira issue ID")
	}

	// Collect all unique issues
	allIssuesMap := make(map[string]bool)
	for _, issue := range titleIssues {
		allIssuesMap[issue] = true
	}
	for _, issue := range branchIssues {
		allIssuesMap[issue] = true
	}
	for _, issue := range bodyIssues {
		allIssuesMap[issue] = true
	}

	uniqueIssues := make([]string, 0, len(allIssuesMap))
	for issue := range allIssuesMap {
		uniqueIssues = append(uniqueIssues, issue)
	}

	// Validate each unique Jira issue using jira-lint
	var validationResults []ValidationResult
	for _, issueKey := range uniqueIssues {
		if jv.validateIssueStatus {
			result := jv.validateJiraIssueWithCLI(issueKey)
			validationResults = append(validationResults, result)
		} else {
			// If status validation is disabled, assume all pattern-matched issues are valid
			validationResults = append(validationResults, ValidationResult{
				Valid:    true,
				IssueKey: issueKey,
			})
		}
	}

	// Check if any issues are invalid (only if status validation is enabled)
	if jv.validateIssueStatus {
		var invalidIssues []ValidationResult
		for _, result := range validationResults {
			if !result.Valid {
				invalidIssues = append(invalidIssues, result)
			}
		}

		if len(invalidIssues) > 0 {
			report := jv.generateValidationReport(titleIssues, branchIssues, bodyIssues, validationResults)
			setGitHubOutput("report", report)
			setGitHubOutput("status", "failure")

			var invalidKeys []string
			for _, result := range invalidIssues {
				invalidKeys = append(invalidKeys, result.IssueKey)
			}
			return fmt.Errorf("invalid Jira issues found: %s", strings.Join(invalidKeys, ", "))
		}
	}

	// Check for consistency issues
	if len(branchIssues) > 0 && len(titleIssues) > 0 {
		titleSet := make(map[string]bool)
		for _, issue := range titleIssues {
			titleSet[issue] = true
		}

		branchSet := make(map[string]bool)
		for _, issue := range branchIssues {
			branchSet[issue] = true
		}

		hasCommonIssue := false
		for issue := range titleSet {
			if branchSet[issue] {
				hasCommonIssue = true
				break
			}
		}

		if !hasCommonIssue {
			report := jv.generateValidationReport(titleIssues, branchIssues, bodyIssues, validationResults)
			setGitHubOutput("report", report)
			setGitHubOutput("status", "failure")
			return fmt.Errorf("mismatch between Jira issues in PR title and branch name")
		}
	}

	// All validations passed - generate success report
	report := jv.generateValidationReport(titleIssues, branchIssues, bodyIssues, validationResults)
	setGitHubOutput("report", report)
	setGitHubOutput("status", "success")

	fmt.Println("✅ All Jira validations passed successfully")
	return nil
}

func main() {
	validator, err := NewJiraValidator()
	if err != nil {
		fmt.Printf("::error::%v\n", err)
		os.Exit(1)
	}

	if err := validator.run(); err != nil {
		fmt.Printf("::error::%v\n", err)
		os.Exit(1)
	}
}
