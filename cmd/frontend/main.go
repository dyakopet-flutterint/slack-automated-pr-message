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

type repoConfig struct {
	Name         string
	SectionTitle string
}

func main() {
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found or could not be loaded. Using system environment variables.")
	}

	log.Println("Starting Frontend PR Report...")

	debugMode := strings.ToLower(os.Getenv("DEBUG")) == "true"

	// Parse labels from environment - Frontend uses "Poker" label
	labels := []string{"Poker"}
	if customLabels := os.Getenv("FRONTEND_LABELS"); customLabels != "" {
		labels = []string{}
		for _, label := range strings.Split(customLabels, ",") {
			label = strings.TrimSpace(label)
			if label != "" {
				labels = append(labels, label)
			}
		}
	}

	// Parse allowed users from environment
	var allowedUsers []string
	usersStr := os.Getenv("USER_MAPPING")
	if usersStr != "" {
		// Extract GitHub usernames from USER_MAPPING (format: slack_id:github_user,...)
		pairs := strings.Split(usersStr, ",")
		for _, pair := range pairs {
			parts := strings.Split(strings.TrimSpace(pair), ":")
			if len(parts) == 2 {
				githubUser := strings.TrimSpace(parts[1])
				if githubUser != "" {
					allowedUsers = append(allowedUsers, githubUser)
				}
			}
		}
	}

	// Frontend repositories
	owner := os.Getenv("GITHUB_OWNER")
	token := os.Getenv("GITHUB_TOKEN")
	repos := []repoConfig{
		{Name: "fips-web-client", SectionTitle: "Frontend"},
		{Name: "fips-stars-web", SectionTitle: "Starsweb"},
	}

	// Build JIRA fetch options
	jiraOpts := jira.FetchOptions{
		URL:       os.Getenv("JIRA_URL"),
		Username:  os.Getenv("JIRA_USERNAME"),
		APIToken:  os.Getenv("JIRA_API_TOKEN"),
		UsePAT:    strings.ToLower(os.Getenv("JIRA_USE_PAT")) == "true",
		DebugMode: debugMode,
	}

	// Build GitHub username to Slack user ID mapping
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

	sections := make([]slack.ReportSection, 0, len(repos))
	for _, repo := range repos {
		log.Printf("Fetching PRs from %s/%s with labels: %v", owner, repo.Name, labels)

		githubPRs, fetchErr := github.FetchPRs(github.FetchOptions{
			Token:        token,
			Owner:        owner,
			Repo:         repo.Name,
			Labels:       labels,
			AllowedUsers: allowedUsers,
			DebugMode:    debugMode,
		})
		if fetchErr != nil {
			log.Fatalf("Error fetching PRs from %s/%s: %v", owner, repo.Name, fetchErr)
		}

		log.Printf("Fetched %d PRs from %s/%s", len(githubPRs), owner, repo.Name)

		jiraInfo := fetchJiraInfo(jiraOpts, githubPRs)
		sections = append(sections, slack.ReportSection{
			Title:       repo.SectionTitle,
			GithubOwner: owner,
			GithubRepo:  repo.Name,
			PRs:         buildSlackPRs(githubPRs, jiraInfo, githubToSlackMap),
		})
	}

	// Build Slack message options
	slackOpts := slack.MessageOptions{
		Token:        os.Getenv("SLACK_TOKEN"),
		Channel:      os.Getenv("SLACK_CHANNEL"),
		GithubOwner:  owner,
		GithubRepo:   repos[0].Name,
		JiraURL:      os.Getenv("JIRA_URL"),
		TeamGroup:    os.Getenv("TEAM_GROUP"),
		ReportTitle:  "Frontend Report",
		ShowAssignee: true, // Show assignee for frontend
		UseCheckmark: true, // Use checkmark emoji
		DebugMode:    debugMode,
	}

	log.Printf("Sending Frontend report to Slack channel: %s", slackOpts.Channel)

	// Send to Slack
	err = slack.SendPRReportSections(slackOpts, sections)
	if err != nil {
		log.Fatalf("Error sending message to Slack: %v", err)
	}

	log.Println("Frontend PR report sent to Slack successfully!")
}

func fetchJiraInfo(jiraOpts jira.FetchOptions, githubPRs []*github.PRResult) map[string]*jira.TicketInfo {
	var jiraTicketIDs []string
	for _, pr := range githubPRs {
		if pr.JiraTicket != "" {
			jiraTicketIDs = append(jiraTicketIDs, pr.JiraTicket)
		}
	}

	if len(jiraTicketIDs) == 0 {
		return nil
	}

	log.Printf("Fetching JIRA info for %d tickets", len(jiraTicketIDs))
	jiraInfo, err := jira.FetchTicketsInfo(jiraOpts, jiraTicketIDs)
	if err != nil {
		log.Printf("Warning: Error fetching JIRA info: %v", err)
		return make(map[string]*jira.TicketInfo)
	}

	return jiraInfo
}

func buildSlackPRs(githubPRs []*github.PRResult, jiraInfo map[string]*jira.TicketInfo, githubToSlackMap map[string]string) []*slack.PRInfo {
	slackPRs := make([]*slack.PRInfo, len(githubPRs))
	for i, pr := range githubPRs {
		jiraStatus := ""
		jiraDescription := pr.Title
		isBlocked := false

		if pr.JiraTicket != "" && jiraInfo != nil {
			if ticket, exists := jiraInfo[pr.JiraTicket]; exists {
				jiraStatus = ticket.Status
				jiraDescription = ticket.Summary
				isBlocked = ticket.IsBlocked
			}
		}

		assignee := pr.Assignee
		if assignee != "" {
			assignee = slack.MapGitHubUserToMention(githubToSlackMap, pr.Assignee)
		}

		author := pr.Author
		if author != "" {
			author = slack.MapGitHubUserToMention(githubToSlackMap, pr.Author)
		}

		slackPRs[i] = &slack.PRInfo{
			Number:      pr.Number,
			Title:       pr.Title,
			Assignee:    assignee,
			Author:      author,
			JiraTicket:  pr.JiraTicket,
			JiraStatus:  jiraStatus,
			Description: jiraDescription,
			IsDraft:     pr.IsDraft,
			IsBlocked:   isBlocked,
		}
	}

	return slackPRs
}
