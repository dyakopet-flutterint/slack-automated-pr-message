package main

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"pr-reporter/internal/github"
	"pr-reporter/internal/jira"
	"pr-reporter/internal/slack"
)

func main() {
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found or could not be loaded. Using system environment variables.")
	}

	log.Println("Starting Middletier PR Report...")

	debugMode := strings.ToLower(os.Getenv("DEBUG")) == "true"

	// Parse labels from environment - Middletier has no label filter by default
	var labels []string
	if customLabels := os.Getenv("MIDDLETIER_LABELS"); customLabels != "" {
		for _, label := range strings.Split(customLabels, ",") {
			label = strings.TrimSpace(label)
			if label != "" {
				labels = append(labels, label)
			}
		}
	}

	// Middletier repository
	owner := os.Getenv("GITHUB_OWNER")
	repo := "fips-poker-web-mt"
	token := os.Getenv("GITHUB_TOKEN")

	if len(labels) > 0 {
		log.Printf("Fetching PRs from %s/%s with labels: %v", owner, repo, labels)
	} else {
		log.Printf("Fetching all PRs from %s/%s (no label filter)", owner, repo)
	}

	// Fetch PRs from GitHub
	githubOpts := github.FetchOptions{
		Token:     token,
		Owner:     owner,
		Repo:      repo,
		Labels:    labels,
		DebugMode: debugMode,
	}

	githubPRs, err := github.FetchPRs(githubOpts)
	if err != nil {
		log.Fatalf("Error fetching PRs from %s/%s: %v", owner, repo, err)
	}

	log.Printf("Fetched %d PRs from %s/%s", len(githubPRs), owner, repo)

	// Build JIRA fetch options
	jiraOpts := jira.FetchOptions{
		URL:       os.Getenv("JIRA_URL"),
		Username:  os.Getenv("JIRA_USERNAME"),
		APIToken:  os.Getenv("JIRA_API_TOKEN"),
		UsePAT:    strings.ToLower(os.Getenv("JIRA_USE_PAT")) == "true",
		DebugMode: debugMode,
	}

	// Collect all JIRA ticket IDs
	var jiraTicketIDs []string
	for _, pr := range githubPRs {
		if pr.JiraTicket != "" {
			jiraTicketIDs = append(jiraTicketIDs, pr.JiraTicket)
		}
	}

	// Fetch JIRA information if we have tickets
	var jiraInfo map[string]*jira.TicketInfo
	if len(jiraTicketIDs) > 0 {
		log.Printf("Fetching JIRA info for %d tickets", len(jiraTicketIDs))
		jiraInfo, err = jira.FetchTicketsInfo(jiraOpts, jiraTicketIDs)
		if err != nil {
			log.Printf("Warning: Error fetching JIRA info: %v", err)
			jiraInfo = make(map[string]*jira.TicketInfo)
		}
	}

	// Build GitHub username to Slack user ID mapping
	usersStr := os.Getenv("USER_MAPPING")
	githubToSlackMap := make(map[string]string)
	if usersStr != "" {
		pairs := strings.Split(usersStr, ",")
		for _, pair := range pairs {
			parts := strings.Split(strings.TrimSpace(pair), ":")
			if len(parts) == 2 {
				slackUserID := strings.TrimSpace(parts[0])
				githubUser := strings.TrimSpace(parts[1])
				githubToSlackMap[githubUser] = slackUserID
			}
		}
	}

	// Convert GitHub PR results to Slack PR format
	slackPRs := make([]*slack.PRInfo, len(githubPRs))
	for i, pr := range githubPRs {
		jiraStatus := ""
		jiraDescription := pr.Title
		isBlocked := false

		// Get JIRA info if available
		if pr.JiraTicket != "" && jiraInfo != nil {
			if ticket, exists := jiraInfo[pr.JiraTicket]; exists {
				jiraStatus = ticket.Status
				jiraDescription = ticket.Summary
				isBlocked = ticket.IsBlocked
			}
		}

		// Convert assignee to Slack mention format if mapping exists
		assignee := pr.Assignee
		if assignee != "" {
			assignee = slack.MapGitHubUserToMention(githubToSlackMap, pr.Assignee)
		}

		slackPRs[i] = &slack.PRInfo{
			Number:      pr.Number,
			Title:       pr.Title,
			Assignee:    assignee,
			JiraTicket:  pr.JiraTicket,
			JiraStatus:  jiraStatus,
			Description: jiraDescription,
			IsDraft:     pr.IsDraft,
			IsBlocked:   isBlocked,
		}
	}

	// Build Slack message options
	slackOpts := slack.MessageOptions{
		Token:        os.Getenv("SLACK_TOKEN"),
		Channel:      os.Getenv("MIDDLETIER_SLACK_CHANNEL"), // Use separate channel for middletier
		GithubOwner:  owner,
		GithubRepo:   repo,
		JiraURL:      os.Getenv("JIRA_URL"),
		TeamGroup:    os.Getenv("MIDDLETIER_TEAM_GROUP"), // Use separate team group for middletier
		MentionUsers: os.Getenv("MIDDLETIER_MENTION_USERS"), // Comma-separated Slack user IDs to mention
		ReportTitle:  "Middletier Report",
		ShowAssignee: false, // Don't show assignee for middletier
		UseCheckmark: false, // Use memo emoji instead of checkmark
		DebugMode:    debugMode,
	}

	// Fallback to main SLACK_CHANNEL if MIDDLETIER_SLACK_CHANNEL not set
	if slackOpts.Channel == "" {
		slackOpts.Channel = os.Getenv("SLACK_CHANNEL")
	}

	log.Printf("Sending Middletier report to Slack channel: %s", slackOpts.Channel)

	// Send to Slack
	err = slack.SendPRReport(slackOpts, slackPRs)
	if err != nil {
		log.Fatalf("Error sending message to Slack: %v", err)
	}

	log.Println("Middletier PR report sent to Slack successfully!")
}
