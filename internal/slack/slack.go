package slack

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

// MessageOptions contains options for sending a PR report to Slack
type MessageOptions struct {
	Token               string // Slack bot token
	Channel             string // Slack channel to post to (e.g., "#channel-name" or "C1234567890")
	GithubOwner         string // GitHub repository owner (for PR links)
	GithubRepo          string // GitHub repository name (for PR links)
	JiraURL             string // JIRA base URL (for ticket links)
	TeamGroup           string // Slack team group ID to mention (optional)
	MentionUsers        string // Comma-separated Slack user IDs to mention (alternative to TeamGroup)
	PlainReviewReminder bool   // Whether to add an untagged review reminder
	ReportTitle         string // Optional title for the report (e.g., "Frontend Report")
	ShowAssignee        bool   // Whether to show assignee in PR line (default: true)
	UseCheckmark        bool   // Whether to use checkmark emoji for no blocked/draft (default: true, false = memo emoji)
	DebugMode           bool   // Enable debug logging
}

// PRInfo represents PR information to be sent to Slack
type PRInfo struct {
	Number      int
	Title       string
	Assignee    string // Slack mention format (e.g., "<@U123456>") or GitHub username
	Author      string // Slack mention format (e.g., "<@U123456>") or GitHub username
	JiraTicket  string
	JiraStatus  string
	Description string
	IsDraft     bool
	IsBlocked   bool
}

// ReportSection represents a repository section in a Slack report.
type ReportSection struct {
	Title       string
	GithubOwner string
	GithubRepo  string
	PRs         []*PRInfo
}

// SendPRReport formats and sends a PR report message to Slack
func SendPRReport(opts MessageOptions, prs []*PRInfo) error {
	if opts.GithubOwner == "" || opts.GithubRepo == "" {
		return fmt.Errorf("GitHub owner and repo are required")
	}

	return SendPRReportSections(opts, []ReportSection{{
		GithubOwner: opts.GithubOwner,
		GithubRepo:  opts.GithubRepo,
		PRs:         prs,
	}})
}

// SendPRReportSections formats and sends a multi-section PR report message to Slack.
func SendPRReportSections(opts MessageOptions, sections []ReportSection) error {
	if opts.Token == "" {
		return fmt.Errorf("Slack token is required")
	}
	if opts.Channel == "" {
		return fmt.Errorf("Slack channel is required")
	}
	if len(sections) == 0 {
		return fmt.Errorf("at least one report section is required")
	}
	for _, section := range sections {
		if section.GithubOwner == "" || section.GithubRepo == "" {
			return fmt.Errorf("GitHub owner and repo are required for each section")
		}
	}

	api := slack.New(opts.Token)

	// Test authentication in debug mode
	if opts.DebugMode {
		log.Println("Debug: Testing Slack authentication...")
		authTest, err := api.AuthTest()
		if err != nil {
			return fmt.Errorf("Slack authentication failed: %v", err)
		}
		log.Printf("Debug: Authenticated as: %s (Team: %s)", authTest.User, authTest.Team)
	}

	// Format message with date and total on separate lines with emojis
	message := buildReportMessage(opts, sections)

	if opts.DebugMode {
		log.Printf("Debug: Sending message to channel %s", opts.Channel)
		log.Printf("Debug: Message length: %d characters", len(message))
	}

	// Send message to Slack
	_, _, err := api.PostMessage(
		opts.Channel,
		slack.MsgOptionText(message, false),
		slack.MsgOptionAsUser(true),
	)

	if err != nil {
		return fmt.Errorf("error posting message to Slack: %v", err)
	}

	if opts.DebugMode {
		log.Println("Debug: Message sent successfully")
	}

	return nil
}

func buildReportMessage(opts MessageOptions, sections []ReportSection) string {
	currentDate := time.Now().Format("2006-01-02")
	dateText := fmt.Sprintf(":date: *%s*", currentDate)
	totalPRs := 0
	for _, section := range sections {
		totalPRs += len(section.PRs)
	}
	totalText := fmt.Sprintf(":bar_chart: *Total Open PRs: %d*", totalPRs)

	var lines []string

	// Add report title if provided
	if opts.ReportTitle != "" {
		lines = append(lines, fmt.Sprintf("📋 *%s*", opts.ReportTitle))
		lines = append(lines, "") // Empty line for spacing
	}

	lines = append(lines, dateText)
	lines = append(lines, "") // Empty line for spacing
	lines = append(lines, totalText)
	lines = append(lines, "") // Empty line for spacing

	for sectionIndex, section := range sections {
		if section.Title != "" {
			lines = append(lines, fmt.Sprintf("*%s*", section.Title))
			lines = append(lines, fmt.Sprintf(":bar_chart: *Open PRs: %d*", len(section.PRs)))
			lines = append(lines, "")
		}

		// Track blocked/draft PRs for each section.
		var blockedPRs []string
		var draftPRs []string

		for i, pr := range section.PRs {
			statusPart := pr.JiraStatus
			if statusPart == "" {
				statusPart = "Unknown"
			}

			// Track blocked and draft PRs for end summary with links
			if pr.IsBlocked && pr.IsDraft {
				blockedPRs = append(blockedPRs, fmt.Sprintf("<https://github.com/%s/%s/pull/%d|PR-%d> (Blocked & Draft)",
					section.GithubOwner, section.GithubRepo, pr.Number, pr.Number))
			} else if pr.IsBlocked {
				blockedPRs = append(blockedPRs, fmt.Sprintf("<https://github.com/%s/%s/pull/%d|PR-%d>",
					section.GithubOwner, section.GithubRepo, pr.Number, pr.Number))
			} else if pr.IsDraft {
				draftPRs = append(draftPRs, fmt.Sprintf("<https://github.com/%s/%s/pull/%d|PR-%d>",
					section.GithubOwner, section.GithubRepo, pr.Number, pr.Number))
			}

			// Prefer assignee when present. For unassigned PRs, show who opened it.
			userText := pr.Assignee
			userPrefix := "assigned to"
			if userText == "" {
				userText = pr.Author
				userPrefix = "opened by"
			}
			if userText == "" {
				userText = "unassigned"
				userPrefix = "assigned to"
			}

			// Format JIRA ticket link
			jiraLink := pr.JiraTicket
			if pr.JiraTicket != "" && opts.JiraURL != "" {
				jiraLink = fmt.Sprintf("<%s/browse/%s|%s>", opts.JiraURL, pr.JiraTicket, pr.JiraTicket)
			} else if pr.JiraTicket == "" {
				jiraLink = "N/A"
			}

			// Format description
			description := pr.Description
			if description == "" {
				description = "No description"
			}

			// Format the PR line
			var prLine string
			if opts.ShowAssignee {
				prLine = fmt.Sprintf("%d. *<https://github.com/%s/%s/pull/%d|PR-%d>* %s %s | Jira: %s | %s | *%s*",
					i+1,
					section.GithubOwner,
					section.GithubRepo,
					pr.Number,
					pr.Number,
					userPrefix,
					userText,
					jiraLink,
					description,
					statusPart)
			} else {
				prLine = fmt.Sprintf("%d. *<https://github.com/%s/%s/pull/%d|PR-%d>* | Jira: %s | %s | *%s*",
					i+1,
					section.GithubOwner,
					section.GithubRepo,
					pr.Number,
					pr.Number,
					jiraLink,
					description,
					statusPart)
			}

			lines = append(lines, prLine)
		}

		lines = append(lines, "")

		if len(blockedPRs) > 0 || len(draftPRs) > 0 {
			if len(blockedPRs) > 0 {
				lines = append(lines, fmt.Sprintf("🚫 *Blocked:* %s", strings.Join(blockedPRs, ", ")))
			}
			if len(draftPRs) > 0 {
				lines = append(lines, fmt.Sprintf("📝 *Draft:* %s", strings.Join(draftPRs, ", ")))
			}
		} else {
			// Use checkmark or memo emoji based on opts.UseCheckmark
			emoji := "✅"
			if !opts.UseCheckmark {
				emoji = "📝"
			}
			lines = append(lines, fmt.Sprintf("%s *Blocked/Draft:* N/A", emoji))
		}

		if sectionIndex != len(sections)-1 {
			lines = append(lines, "")
		}
	}

	// Add team mention or individual user mentions if provided
	if opts.MentionUsers != "" {
		// Mention specific users (comma-separated user IDs)
		lines = append(lines, "")
		userIDs := strings.Split(opts.MentionUsers, ",")
		var mentions []string
		for _, userID := range userIDs {
			userID = strings.TrimSpace(userID)
			if userID != "" {
				mentions = append(mentions, fmt.Sprintf("<@%s>", userID))
			}
		}
		if len(mentions) > 0 {
			lines = append(lines, fmt.Sprintf("%s Please make sure to review these pull requests!", strings.Join(mentions, " ")))
		}
	} else if opts.TeamGroup != "" {
		// Mention team group
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("<!subteam^%s> Please make sure to review these pull requests!", opts.TeamGroup))
	} else if opts.PlainReviewReminder {
		lines = append(lines, "")
		lines = append(lines, "Please make sure to review these pull requests!")
	}

	return strings.Join(lines, "\n")
}

// GetChannelUsers fetches the list of users from a specified Slack channel
func GetChannelUsers(token, channelName string, debugMode bool) ([]string, error) {
	api := slack.New(token)

	// Test authentication first
	if debugMode {
		log.Println("Debug: Testing Slack authentication...")
		authTest, err := api.AuthTest()
		if err != nil {
			return nil, fmt.Errorf("Slack authentication failed: %v", err)
		}
		log.Printf("Debug: Authenticated as: %s (Team: %s)", authTest.User, authTest.Team)
	}

	var channelID string
	channelName = strings.TrimPrefix(channelName, "#")

	if debugMode {
		log.Printf("Debug: Looking for channel: %s", channelName)
	}

	// Use the conversations API to find the channel
	conversationTypes := []string{"public_channel", "private_channel"}

	for _, convType := range conversationTypes {
		if debugMode {
			log.Printf("Debug: Searching for %s channels...", convType)
		}

		conversations, _, err := api.GetConversations(&slack.GetConversationsParameters{
			Types: []string{convType},
			Limit: 1000,
		})

		if err != nil {
			if debugMode {
				log.Printf("Debug: Error fetching %s channels: %v", convType, err)
			}
			continue
		}

		for _, conv := range conversations {
			if conv.Name == channelName {
				channelID = conv.ID
				if debugMode {
					log.Printf("Debug: Found channel #%s with ID: %s (type: %s)", channelName, channelID, convType)
				}
				break
			}
		}

		if channelID != "" {
			break
		}
	}

	// If still not found, try without specifying types
	if channelID == "" {
		if debugMode {
			log.Println("Debug: Channel not found in typed search, trying all accessible channels...")
		}

		conversations, _, err := api.GetConversations(&slack.GetConversationsParameters{
			Limit: 1000,
		})

		if err != nil {
			return nil, fmt.Errorf("error fetching conversations: %v", err)
		}

		for _, conv := range conversations {
			if conv.Name == channelName {
				channelID = conv.ID
				if debugMode {
					log.Printf("Debug: Found channel #%s with ID: %s", channelName, channelID)
				}
				break
			}
		}
	}

	if channelID == "" {
		return nil, fmt.Errorf("channel #%s not found", channelName)
	}

	// Get channel members
	if debugMode {
		log.Printf("Debug: Getting members for channel ID: %s", channelID)
	}

	members, _, err := api.GetUsersInConversation(&slack.GetUsersInConversationParameters{
		ChannelID: channelID,
		Limit:     1000,
	})
	if err != nil {
		return nil, fmt.Errorf("error fetching channel members: %v", err)
	}

	if debugMode {
		log.Printf("Debug: Found %d members in channel #%s", len(members), channelName)
	}

	return members, nil
}

// MapGitHubUserToMention converts GitHub username to Slack mention format
// githubToSlackMap: map of GitHub username -> Slack user ID
// githubUsername: the GitHub username to convert
// Returns Slack mention format "<@U123456>", a plain-text label, or "@githubUsername" if no mapping found
func MapGitHubUserToMention(githubToSlackMap map[string]string, githubUsername string) string {
	if githubUsername == "" {
		return ""
	}

	if mappedValue, exists := githubToSlackMap[githubUsername]; exists {
		mappedValue = strings.TrimSpace(mappedValue)
		if mappedValue == "" {
			return githubUsername
		}

		if strings.HasPrefix(mappedValue, "<@") && strings.HasSuffix(mappedValue, ">") {
			return mappedValue
		}

		if regexp.MustCompile(`^[UW][A-Z0-9]+$`).MatchString(mappedValue) {
			return fmt.Sprintf("<@%s>", mappedValue)
		}

		return mappedValue
	}

	for pattern, mappedValue := range githubToSlackMap {
		if !matchesGitHubUsername(pattern, githubUsername) {
			continue
		}

		mappedValue = strings.TrimSpace(mappedValue)
		if mappedValue == "" {
			return githubUsername
		}

		if strings.HasPrefix(mappedValue, "<@") && strings.HasSuffix(mappedValue, ">") {
			return mappedValue
		}

		if regexp.MustCompile(`^[UW][A-Z0-9]+$`).MatchString(mappedValue) {
			return fmt.Sprintf("<@%s>", mappedValue)
		}

		return mappedValue
	}

	// Fallback to GitHub username with @ prefix
	return "@" + githubUsername
}

func matchesGitHubUsername(pattern, githubUsername string) bool {
	pattern = strings.TrimSpace(pattern)
	githubUsername = strings.TrimSpace(githubUsername)
	if pattern == "" || githubUsername == "" {
		return false
	}

	if strings.EqualFold(pattern, githubUsername) {
		return true
	}

	return strings.EqualFold(pattern, "copilot") && strings.Contains(strings.ToLower(githubUsername), "copilot")
}
