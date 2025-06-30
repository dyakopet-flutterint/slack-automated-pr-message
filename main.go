package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/andygrunwald/go-jira"
	"github.com/google/go-github/v45/github"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"github.com/slack-go/slack"
	"golang.org/x/oauth2"
)

// Configuration struct to hold all app settings
type Config struct {
	// GitHub configuration
	GithubToken   string
	GithubOwner   string
	GithubRepo    string
	DebugMode     bool
	
	// JIRA configuration
	JiraURL       string
	JiraUsername  string
	JiraAPIToken  string
	
	// Slack configuration
	SlackToken    string
	SlackChannel  string
	TeamGroup     string
	
	// User mapping (optional) - format: "slack_user:github_user,slack_user2:github_user2"
	UserMapping   map[string]string
}

// PullRequestInfo holds the formatted information about a PR
type PullRequestInfo struct {
	Number       int
	Title        string
	URL          string
	Assignee     string
	JiraTicket   string
	JiraStatus   string
	JiraStatusID string
	Description  string
	IsDraft      bool
	IsBlocked    bool
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	config := &Config{
		GithubToken:   os.Getenv("TOKEN"),
		GithubOwner:   os.Getenv("OWNER"),
		GithubRepo:    os.Getenv("REPO"),
		JiraURL:       os.Getenv("JIRA_URL"),
		JiraUsername:  os.Getenv("JIRA_USERNAME"),
		JiraAPIToken:  os.Getenv("JIRA_API_TOKEN"),
		SlackToken:    os.Getenv("SLACK_TOKEN"),
		SlackChannel:  os.Getenv("SLACK_CHANNEL"),
		TeamGroup:     os.Getenv("TEAM_GROUP"),
		DebugMode:     strings.ToLower(os.Getenv("DEBUG")) == "true",
		UserMapping:   make(map[string]string),
	}
	
	// Parse user mapping if provided (format: "slack_user:github_user,slack_user2:github_user2")
	userMappingStr := os.Getenv("USER_MAPPING")
	if userMappingStr != "" {
		pairs := strings.Split(userMappingStr, ",")
		for _, pair := range pairs {
			parts := strings.Split(strings.TrimSpace(pair), ":")
			if len(parts) == 2 {
				slackUser := strings.TrimSpace(parts[0])
				githubUser := strings.TrimSpace(parts[1])
				config.UserMapping[slackUser] = githubUser
			}
		}
	}
	
	// Basic validation
	if config.GithubToken == "" || config.SlackToken == "" {
		return nil, fmt.Errorf("required environment variables are not set")
	}
	
	return config, nil
}

// GetSlackChannelUsers fetches the list of users from the specified Slack channel
func GetSlackChannelUsers(config *Config) ([]string, map[string]string, error) {
	api := slack.New(config.SlackToken)
	
	// Test authentication first
	if config.DebugMode {
		log.Println("Debug: Testing Slack authentication...")
		authTest, err := api.AuthTest()
		if err != nil {
			return nil, nil, fmt.Errorf("Slack authentication failed: %v", err)
		}
		log.Printf("Debug: Authenticated as: %s (Team: %s)", authTest.User, authTest.Team)
	}
	
	var channelID string
	channelName := strings.TrimPrefix(config.SlackChannel, "#")
	
	if config.DebugMode {
		log.Printf("Debug: Looking for channel: %s", channelName)
	}
	
	// Use the conversations API to find the channel
	// Try with different types to cover public and private channels
	conversationTypes := []string{"public_channel", "private_channel"}
	
	for _, convType := range conversationTypes {
		if config.DebugMode {
			log.Printf("Debug: Searching for %s channels...", convType)
		}
		
		conversations, _, err := api.GetConversations(&slack.GetConversationsParameters{
			Types: []string{convType},
			Limit: 1000,
		})
		
		if err != nil {
			if config.DebugMode {
				log.Printf("Debug: Error fetching %s channels: %v", convType, err)
			}
			continue // Try the next type
		}
		
		// Look for our channel in this type
		for _, conv := range conversations {
			if conv.Name == channelName {
				channelID = conv.ID
				if config.DebugMode {
					log.Printf("Debug: Found channel #%s with ID: %s (type: %s)", channelName, channelID, convType)
				}
				break
			}
		}
		
		if channelID != "" {
			break // Found the channel, no need to check other types
		}
	}
	
	// If still not found, try without specifying types (gets all accessible channels)
	if channelID == "" {
		if config.DebugMode {
			log.Println("Debug: Channel not found in typed search, trying all accessible channels...")
		}
		
		conversations, _, err := api.GetConversations(&slack.GetConversationsParameters{
			Limit: 1000,
		})
		
		if err != nil {
			return nil, nil, fmt.Errorf("error fetching conversations - ensure your bot has 'channels:read' and 'groups:read' scopes: %v", err)
		}
		
		for _, conv := range conversations {
			if conv.Name == channelName {
				channelID = conv.ID
				if config.DebugMode {
					log.Printf("Debug: Found channel #%s with ID: %s", channelName, channelID)
				}
				break
			}
		}
	}
	
	if channelID == "" {
		return nil, nil, fmt.Errorf("channel #%s not found. Make sure the bot is added to the channel and has proper permissions (channels:read, groups:read)", channelName)
	}
	
	// Get channel members
	if config.DebugMode {
		log.Printf("Debug: Getting members for channel ID: %s", channelID)
	}
	
	members, _, err := api.GetUsersInConversation(&slack.GetUsersInConversationParameters{
		ChannelID: channelID,
		Limit:     1000,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("error fetching channel members - ensure your bot has 'channels:read' and 'groups:read' scopes: %v", err)
	}
	
	if config.DebugMode {
		log.Printf("Debug: Found %d members in channel #%s", len(members), channelName)
	}
	
	// Get user info for each member to get their usernames and IDs
	var allowedUsers []string
	slackUserMap := make(map[string]string) // username -> user ID for proper mentions
	
	for _, memberID := range members {
		user, err := api.GetUserInfo(memberID)
		if err != nil {
			if config.DebugMode {
				log.Printf("Debug: Error getting info for user %s (ensure bot has 'users:read' scope): %v", memberID, err)
			}
			continue
		}
		
		// Skip bots and deleted users
		if user.IsBot || user.Deleted {
			if config.DebugMode {
				log.Printf("Debug: Skipping user %s (bot: %t, deleted: %t)", user.Name, user.IsBot, user.Deleted)
			}
			continue
		}
		
		// Store the mapping of Slack username to user ID for mentions
		slackUserMap[user.Name] = user.ID
		
		// Check if there's a mapping for this Slack user to a GitHub user
		githubUser := user.Name // Default to Slack username
		if mappedUser, exists := config.UserMapping[user.Name]; exists {
			githubUser = mappedUser
			if config.DebugMode {
				log.Printf("Debug: Mapped Slack user %s to GitHub user %s", user.Name, githubUser)
			}
		}
		
		allowedUsers = append(allowedUsers, githubUser)
		
		if config.DebugMode {
			log.Printf("Debug: Added user to allowed list: %s (Slack: %s, ID: %s)", githubUser, user.Name, user.ID)
		}
	}
	
	if config.DebugMode {
		log.Printf("Debug: Final allowed users list: %v", allowedUsers)
		log.Printf("Debug: Slack user map for mentions: %v", slackUserMap)
	}
	
	return allowedUsers, slackUserMap, nil
}

// FetchPullRequests fetches PRs from GitHub that match our criteria
func FetchPullRequests(config *Config) ([]*PullRequestInfo, error) {
	// First, get the list of allowed users from the Slack channel and user ID mapping
	allowedUsers, slackUserMap, err := GetSlackChannelUsers(config)
	if err != nil {
		return nil, fmt.Errorf("error getting Slack channel users: %v", err)
	}
	
	if len(allowedUsers) == 0 {
		log.Println("Warning: No users found in Slack channel, no PRs will be included")
		return []*PullRequestInfo{}, nil
	}
	
	// Create reverse mapping: GitHub username -> Slack username
	githubToSlackMap := make(map[string]string)
	for slackUser, githubUser := range config.UserMapping {
		githubToSlackMap[githubUser] = slackUser
	}
	
	if config.DebugMode {
		log.Printf("Debug: GitHub to Slack reverse mapping: %v", githubToSlackMap)
	}
	
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: config.GithubToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	
	// Verify authentication
	if config.DebugMode {
		user, _, err := client.Users.Get(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("error verifying GitHub authentication: %v", err)
		}
		log.Printf("Debug: Authenticated as GitHub user: %s", *user.Login)
	}
	
	// Set up GitHub search options
	opts := &github.PullRequestListOptions{
		State: "open",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}
	
	allPRs, _, err := client.PullRequests.List(ctx, config.GithubOwner, config.GithubRepo, opts)
	if err != nil {
		return nil, fmt.Errorf("error fetching PRs: %v", err)
	}
	
	if config.DebugMode {
		log.Printf("Debug: Found %d total open PRs in %s/%s", len(allPRs), config.GithubOwner, config.GithubRepo)
	}
	
	var filteredPRs []*PullRequestInfo
	
	// Regex to extract JIRA ticket
	jiraRegex := regexp.MustCompile(`SCRUM-\d+`)
	
	for _, pr := range allPRs {
		// Debug PR info
		if config.DebugMode {
			log.Printf("Debug: Examining PR #%d: %s", *pr.Number, *pr.Title)
			log.Printf("Debug: PR created by: %s", *pr.User.Login)
			log.Printf("Debug: PR is draft: %t", *pr.Draft)
			
			labelNames := make([]string, 0, len(pr.Labels))
			for _, label := range pr.Labels {
				labelNames = append(labelNames, *label.Name)
			}
			log.Printf("Debug: PR labels: %s", strings.Join(labelNames, ", "))
		}
		
		// Skip if not by allowed user
		if pr.User == nil || pr.User.Login == nil {
			if config.DebugMode {
				log.Printf("Debug: PR #%d skipped - no user", *pr.Number)
			}
			continue
		}
		
		userFound := false
		for _, allowedUser := range allowedUsers {
			allowedUser = strings.TrimSpace(allowedUser)
			if allowedUser == "" {
				continue
			}
			
			if strings.EqualFold(allowedUser, *pr.User.Login) {
				userFound = true
				if config.DebugMode {
					log.Printf("Debug: PR #%d matches allowed user: %s", *pr.Number, allowedUser)
				}
				break
			}
		}
		
		if !userFound {
			if config.DebugMode {
				log.Printf("Debug: PR #%d skipped - user %s not in Slack channel member list", *pr.Number, *pr.User.Login)
			}
			continue
		}
		
		// Check for "Poker" label (case insensitive)
		hasPokerLabel := false
		for _, label := range pr.Labels {
			if label.Name != nil {
				// Make the check case-insensitive
				if strings.EqualFold(*label.Name, "Poker") {
					hasPokerLabel = true
					if config.DebugMode {
						log.Printf("Debug: PR #%d has matching label: %s", *pr.Number, *label.Name)
					}
					break
				}
			}
		}
		
		if !hasPokerLabel {
			if config.DebugMode {
				log.Printf("Debug: PR #%d skipped - no 'Poker' label found", *pr.Number)
			}
			continue
		}
		
		// Extract JIRA ticket from PR title
		jiraTicket := ""
		if pr.Title != nil {
			matches := jiraRegex.FindStringSubmatch(*pr.Title)
			if len(matches) > 0 {
				jiraTicket = matches[0]
			}
			
			if config.DebugMode && jiraTicket != "" {
				log.Printf("Debug: PR #%d JIRA ticket extracted: %s", *pr.Number, jiraTicket)
			}
		}
		
		// Get assignee and convert to proper Slack mention format
		assignee := "unassigned"
		if pr.Assignee != nil && pr.Assignee.Login != nil {
			githubUsername := *pr.Assignee.Login
			
			// Check if we have a mapping from GitHub username to Slack username
			if slackUsername, exists := githubToSlackMap[githubUsername]; exists {
				// Use proper Slack mention format with user ID
				if userID, userExists := slackUserMap[slackUsername]; userExists {
					assignee = fmt.Sprintf("<@%s>", userID)
					if config.DebugMode {
						log.Printf("Debug: PR #%d assignee mapped from GitHub user %s to Slack mention <@%s> (user: %s)", *pr.Number, githubUsername, userID, slackUsername)
					}
				} else {
					// Fallback to @username if ID not found
					assignee = "@" + slackUsername
					if config.DebugMode {
						log.Printf("Debug: PR #%d assignee using @%s (user ID not found)", *pr.Number, slackUsername)
					}
				}
			} else {
				// No mapping found, check if GitHub username matches a Slack username directly
				if userID, userExists := slackUserMap[githubUsername]; userExists {
					assignee = fmt.Sprintf("<@%s>", userID)
					if config.DebugMode {
						log.Printf("Debug: PR #%d assignee using direct match <@%s> (user: %s)", *pr.Number, userID, githubUsername)
					}
				} else {
					// Fallback to GitHub username
					assignee = "@" + githubUsername
					if config.DebugMode {
						log.Printf("Debug: PR #%d assignee using GitHub username @%s (no Slack user found)", *pr.Number, githubUsername)
					}
				}
			}
		}
		
		// Create PR info
		prInfo := &PullRequestInfo{
			Number:     *pr.Number,
			Title:      *pr.Title,
			URL:        *pr.HTMLURL,
			Assignee:   assignee,
			JiraTicket: jiraTicket,
			IsDraft:    *pr.Draft,
		}
		
		if config.DebugMode {
			log.Printf("Debug: PR #%d matched all criteria and is included", *pr.Number)
			log.Printf("Debug: PR #%d draft status: %t", *pr.Number, prInfo.IsDraft)
			log.Printf("Debug: PR #%d assignee: %s", *pr.Number, prInfo.Assignee)
		}
		
		filteredPRs = append(filteredPRs, prInfo)
	}
	
	return filteredPRs, nil
}

// UpdateJiraInfo updates the PR info with JIRA ticket status
func UpdateJiraInfo(config *Config, prs []*PullRequestInfo) error {
	// Check if we have any PRs with JIRA tickets first
	hasJiraTickets := false
	for _, pr := range prs {
		if pr.JiraTicket != "" {
			hasJiraTickets = true
			break
		}
	}
	
	if !hasJiraTickets {
		if config.DebugMode {
			log.Println("Debug: No JIRA tickets found in any PRs, skipping JIRA integration")
		}
		return nil
	}
	
	// Check JIRA credentials
	if config.JiraUsername == "" || config.JiraAPIToken == "" || config.JiraURL == "" {
		log.Println("Warning: JIRA credentials not fully configured, JIRA status will show as 'Unknown'")
		return nil
	}
	
	if config.DebugMode {
		log.Printf("Debug: Initializing JIRA client for %s", config.JiraURL)
	}
	
	tp := jira.BasicAuthTransport{
		Username: config.JiraUsername,
		Password: config.JiraAPIToken,
	}
	
	jiraClient, err := jira.NewClient(tp.Client(), config.JiraURL)
	if err != nil {
		log.Printf("Error creating JIRA client: %v", err)
		return nil // Don't fail the entire process, just skip JIRA updates
	}
	
	// Test JIRA connection in debug mode
	if config.DebugMode {
		log.Printf("Debug: Testing JIRA connection to %s", config.JiraURL)
		myself, _, err := jiraClient.User.GetSelf()
		if err != nil {
			log.Printf("Debug: JIRA authentication test failed: %v", err)
			return nil // Don't fail the entire process
		}
		log.Printf("Debug: Successfully authenticated to JIRA as: %s", myself.DisplayName)
	}
	
	for _, pr := range prs {
		if pr.JiraTicket == "" {
			if config.DebugMode {
				log.Printf("Debug: Skipping JIRA lookup for PR #%d - No JIRA ticket found in title", pr.Number)
			}
			continue
		}
		
		if config.DebugMode {
			log.Printf("Debug: Fetching JIRA info for ticket %s (PR #%d)", pr.JiraTicket, pr.Number)
		}
		
		issue, resp, err := jiraClient.Issue.Get(pr.JiraTicket, nil)
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				log.Printf("Warning: JIRA ticket %s not found (PR #%d)", pr.JiraTicket, pr.Number)
				pr.JiraStatus = "Not Found"
			} else {
				log.Printf("Error fetching JIRA ticket %s: %v", pr.JiraTicket, err)
				pr.JiraStatus = "Error"
			}
			continue
		}
		
		// Extract status and description
		if issue != nil && issue.Fields != nil {
			// Extract status
			if issue.Fields.Status != nil && issue.Fields.Status.Name != "" {
				pr.JiraStatus = issue.Fields.Status.Name
				if config.DebugMode {
					log.Printf("Debug: JIRA ticket %s status set to: %s", pr.JiraTicket, pr.JiraStatus)
				}
			} else {
				pr.JiraStatus = "No Status"
				if config.DebugMode {
					log.Printf("Debug: JIRA ticket %s has no status field", pr.JiraTicket)
				}
			}
			
			// Extract description/summary
			if issue.Fields.Summary != "" {
				pr.Description = issue.Fields.Summary
				if config.DebugMode {
					log.Printf("Debug: JIRA ticket %s summary: %s", pr.JiraTicket, pr.Description)
				}
			} else {
				pr.Description = "No Description"
			}
			
			// Check if blocked by status name
			if issue.Fields.Status != nil && issue.Fields.Status.Name != "" {
				statusName := strings.ToLower(issue.Fields.Status.Name)
				if strings.Contains(statusName, "block") || 
				   strings.Contains(statusName, "impediment") ||
				   strings.Contains(statusName, "pause") {
					pr.IsBlocked = true
					if config.DebugMode {
						log.Printf("Debug: JIRA ticket %s marked as blocked due to status: %s", pr.JiraTicket, issue.Fields.Status.Name)
					}
				}
			}
			
			// Check if blocked by labels
			if issue.Fields.Labels != nil {
				for _, label := range issue.Fields.Labels {
					labelLower := strings.ToLower(label)
					if strings.Contains(labelLower, "block") || 
					   strings.Contains(labelLower, "impediment") ||
					   strings.Contains(labelLower, "pause") {
						pr.IsBlocked = true
						if config.DebugMode {
							log.Printf("Debug: JIRA ticket %s marked as blocked due to label: %s", pr.JiraTicket, label)
						}
						break
					}
				}
			}
		} else {
			pr.JiraStatus = "No Data"
			if config.DebugMode {
				log.Printf("Debug: JIRA ticket %s returned no usable data", pr.JiraTicket)
			}
		}
		
		if config.DebugMode {
			log.Printf("Debug: Final status for PR #%d (JIRA %s): %s", pr.Number, pr.JiraTicket, pr.JiraStatus)
		}
	}
	
	return nil
}

// SendSlackMessage formats and sends a message to Slack
func SendSlackMessage(config *Config, prs []*PullRequestInfo) error {
	api := slack.New(config.SlackToken)
	
	// Format message with date and total on separate lines with emojis
	currentDate := time.Now().Format("2006-01-02")
	dateText := fmt.Sprintf(":date: *%s*", currentDate)
	totalText := fmt.Sprintf(":bar_chart: *Total Open PRs: %d*", len(prs))
	
	var lines []string
	lines = append(lines, dateText)
	lines = append(lines, "") // Empty line for spacing
	lines = append(lines, totalText)
	lines = append(lines, "") // Empty line for spacing
	
	// Track blocked/draft PRs for summary at the end
	var blockedPRs []string
	var draftPRs []string
	
	for i, pr := range prs {
		statusPart := pr.JiraStatus
		if statusPart == "" {
			statusPart = "Unknown"
		}
		
		// Track blocked and draft PRs for end summary with links
		if pr.IsBlocked && pr.IsDraft {
			blockedPRs = append(blockedPRs, fmt.Sprintf("<https://github.com/%s/%s/pull/%d|PR-%d> (Blocked & Draft)", 
				config.GithubOwner, config.GithubRepo, pr.Number, pr.Number))
		} else if pr.IsBlocked {
			blockedPRs = append(blockedPRs, fmt.Sprintf("<https://github.com/%s/%s/pull/%d|PR-%d>", 
				config.GithubOwner, config.GithubRepo, pr.Number, pr.Number))
		} else if pr.IsDraft {
			draftPRs = append(draftPRs, fmt.Sprintf("<https://github.com/%s/%s/pull/%d|PR-%d>", 
				config.GithubOwner, config.GithubRepo, pr.Number, pr.Number))
		}
		
		// Format the PR line without blocked/draft status
		prLine := fmt.Sprintf("%d. *<https://github.com/%s/%s/pull/%d|PR-%d>* assigned to %s | Jira: <%s|%s> | %s | *%s*",
			i+1, 
			config.GithubOwner, 
			config.GithubRepo, 
			pr.Number, 
			pr.Number, 
			pr.Assignee, 
			fmt.Sprintf("%s/browse/%s", config.JiraURL, pr.JiraTicket), 
			pr.JiraTicket, 
			pr.Description, 
			statusPart)
		
		lines = append(lines, prLine)
	}
	
	// Add blocked/draft summary at the end
	lines = append(lines, "")
	
	if len(blockedPRs) > 0 || len(draftPRs) > 0 {
		if len(blockedPRs) > 0 {
			lines = append(lines, fmt.Sprintf("🚫 *Blocked:* %s", strings.Join(blockedPRs, ", ")))
		}
		if len(draftPRs) > 0 {
			lines = append(lines, fmt.Sprintf("📝 *Draft:* %s", strings.Join(draftPRs, ", ")))
		}
	} else {
		lines = append(lines, "✅ *Blocked/Draft:* N/A")
	}
	
	// Add team mention
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("<!subteam^%s> Please make sure to review these pull requests!", config.TeamGroup))
	
	message := strings.Join(lines, "\n")
	
	// Send message to Slack
	_, _, err := api.PostMessage(
		config.SlackChannel,
		slack.MsgOptionText(message, false),
		slack.MsgOptionAsUser(true),
	)
	
	return err
}

// RunReport executes the main report generation and sending
func RunReport(config *Config) error {
	log.Println("Starting PR report generation...")
	
	// Fetch PRs from GitHub
	prs, err := FetchPullRequests(config)
	if err != nil {
		return fmt.Errorf("error fetching pull requests: %v", err)
	}
	
	log.Printf("Found %d PRs matching criteria", len(prs))
	
	// Update with JIRA information
	err = UpdateJiraInfo(config, prs)
	if err != nil {
		log.Printf("Warning: Error updating JIRA info: %v", err)
	}
	
	// Send Slack message
	err = SendSlackMessage(config, prs)
	if err != nil {
		return fmt.Errorf("error sending Slack message: %v", err)
	}
	
	log.Println("PR report sent successfully!")
	return nil
}

func main() {
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found or could not be loaded. Using system environment variables.")
	} else {
		log.Println("Environment variables loaded from .env file successfully.")
	}
	
	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}
	
	// If no arguments are provided or run-now is specified
	if len(os.Args) <= 1 || os.Args[1] == "--run-now" {
		log.Println("Running report immediately (test mode)...")
		err = RunReport(config)
		if err != nil {
			log.Fatalf("Error running report: %v", err)
		}
		return
	}
	
	// If --print-config is specified, print the config values
	if len(os.Args) > 1 && os.Args[1] == "--print-config" {
		log.Println("Configuration:")
		log.Printf("GitHub Owner: %s", config.GithubOwner)
		log.Printf("GitHub Repo: %s", config.GithubRepo)
		log.Printf("JIRA URL: %s", config.JiraURL)
		log.Printf("JIRA Username: %s", config.JiraUsername)
		log.Printf("Slack Channel: %s", config.SlackChannel)
		log.Printf("Team Group ID: %s", config.TeamGroup)
		log.Printf("Debug Mode: %v", config.DebugMode)
		log.Printf("User Mapping: %v", config.UserMapping)
		log.Println("(Tokens hidden for security)")
		log.Println("Note: Allowed users are now dynamically fetched from Slack channel members")
		return
	}
	
	// Set up cron scheduler for regular execution
	c := cron.New()
	
	// Schedule for every weekday at 9:00 AM
	_, err = c.AddFunc("0 9 * * 1-5", func() {
		log.Println("Running scheduled PR report...")
		err := RunReport(config)
		if err != nil {
			log.Printf("Error in scheduled report: %v", err)
		}
	})
	
	if err != nil {
		log.Fatalf("Error setting up scheduler: %v", err)
	}
	
	log.Println("PR Reporter started. Will run every weekday at 9:00 AM.")
	log.Println("Press Ctrl+C to stop the application.")
	
	// Start the cron scheduler
	c.Start()
	
	// Keep the application running
	select {}
}