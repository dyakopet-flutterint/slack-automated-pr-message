package slack

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

// MessageOptions contains options for sending a PR report to Slack
type MessageOptions struct {
	Token        string // Slack bot token
	Channel      string // Slack channel to post to (e.g., "#channel-name" or "C1234567890")
	GithubOwner  string // GitHub repository owner (for PR links)
	GithubRepo   string // GitHub repository name (for PR links)
	JiraURL      string // JIRA base URL (for ticket links)
	TeamGroup    string // Slack team group ID to mention (optional)
	MentionUsers string // Comma-separated Slack user IDs to mention (alternative to TeamGroup)
	ReportTitle  string // Optional title for the report (e.g., "Frontend Report")
	DebugMode    bool   // Enable debug logging
}

// PRInfo represents PR information to be sent to Slack
type PRInfo struct {
	Number      int
	Title       string
	Assignee    string // Slack mention format (e.g., "<@U123456>") or GitHub username
	JiraTicket  string
	JiraStatus  string
	Description string
	IsDraft     bool
	IsBlocked   bool
}

// SendPRReport formats and sends a PR report message to Slack
func SendPRReport(opts MessageOptions, prs []*PRInfo) error {
	if opts.Token == "" {
		return fmt.Errorf("Slack token is required")
	}
	if opts.Channel == "" {
		return fmt.Errorf("Slack channel is required")
	}
	if opts.GithubOwner == "" || opts.GithubRepo == "" {
		return fmt.Errorf("GitHub owner and repo are required")
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
	currentDate := time.Now().Format("2006-01-02")
	dateText := fmt.Sprintf(":date: *%s*", currentDate)
	totalText := fmt.Sprintf(":bar_chart: *Total Open PRs: %d*", len(prs))

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
				opts.GithubOwner, opts.GithubRepo, pr.Number, pr.Number))
		} else if pr.IsBlocked {
			blockedPRs = append(blockedPRs, fmt.Sprintf("<https://github.com/%s/%s/pull/%d|PR-%d>",
				opts.GithubOwner, opts.GithubRepo, pr.Number, pr.Number))
		} else if pr.IsDraft {
			draftPRs = append(draftPRs, fmt.Sprintf("<https://github.com/%s/%s/pull/%d|PR-%d>",
				opts.GithubOwner, opts.GithubRepo, pr.Number, pr.Number))
		}

		// Format assignee
		assigneeText := pr.Assignee
		if assigneeText == "" {
			assigneeText = "unassigned"
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
		prLine := fmt.Sprintf("%d. *<https://github.com/%s/%s/pull/%d|PR-%d>* assigned to %s | Jira: %s | %s | *%s*",
			i+1,
			opts.GithubOwner,
			opts.GithubRepo,
			pr.Number,
			pr.Number,
			assigneeText,
			jiraLink,
			description,
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
	}

	message := strings.Join(lines, "\n")

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
// Returns Slack mention format "<@U123456>" or "@githubUsername" if no mapping found
func MapGitHubUserToMention(githubToSlackMap map[string]string, githubUsername string) string {
	if githubUsername == "" {
		return ""
	}

	if slackUserID, exists := githubToSlackMap[githubUsername]; exists {
		return fmt.Sprintf("<@%s>", slackUserID)
	}

	// Fallback to GitHub username with @ prefix
	return "@" + githubUsername
}
