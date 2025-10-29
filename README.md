# PR Reporter

A comprehensive Golang-based automation tool that monitors GitHub pull requests, integrates with JIRA for ticket status tracking, and sends daily reports to Slack channels. Designed to streamline development team workflows by providing automated visibility into open pull requests and their associated JIRA task statuses.

## 🚀 Features

- **🔍 Dynamic Team Management**: Automatically fetches team members from Slack channel membership
- **📋 GitHub Integration**: Monitors open pull requests with specific labels from designated repositories
- **🎫 JIRA Integration**: Extracts JIRA ticket numbers from PR titles and fetches current status and descriptions
- **💬 Slack Notifications**: Sends formatted daily reports with proper user mentions and clickable links
- **🚫 Draft & Blocked Detection**: Identifies draft PRs and blocked JIRA tickets with clear status reporting
- **⏰ Automated Scheduling**: Runs automatically every weekday at 9:00 AM
- **🐛 Debug Mode**: Comprehensive logging for troubleshooting and verification
- **⚙️ Flexible Configuration**: Environment variable-based configuration with .env file support
- **🔗 User Mapping**: Handles cases where Slack and GitHub usernames differ
- **📊 Rich Formatting**: Emoji-enhanced messages with proper Slack mentions and links

## 📋 Prerequisites

- **Go 1.19+**: The application is built in Go and requires a recent version
- **GitHub Personal Access Token**: With `repo` scope for accessing repository data
- **JIRA API Token**: For fetching ticket status and descriptions from your JIRA instance
- **Slack Bot Token**: With appropriate permissions (see [Slack Configuration](#slack-configuration))
- **Slack User Group**: For mentioning your development team in notifications

## 🛠️ Installation

### 1. Clone the Repository

```bash
git clone <repository-url>
cd pr-reporter
```

### 2. Initialize Go Module

```bash
go mod init pr-reporter
go mod tidy
```

### 3. Install Dependencies

```bash
go get github.com/google/go-github/v45/github
go get github.com/andygrunwald/go-jira
go get github.com/slack-go/slack
go get github.com/robfig/cron/v3
go get golang.org/x/oauth2
go get github.com/joho/godotenv
```

### 4. Build the Application

```bash
go build -o pr-reporter
```

## ⚙️ Configuration

### Environment Variables

Create a `.env` file in the project root:

```env
# GitHub Configuration
GITHUB_TOKEN=your_github_personal_access_token
GITHUB_OWNER=your_github_organization_or_username
GITHUB_REPO=your_repository_name

# JIRA Configuration
JIRA_URL=https://your-company.atlassian.net
JIRA_USERNAME=your_jira_email@company.com
JIRA_API_TOKEN=your_jira_api_token

# Slack Configuration
SLACK_TOKEN=xoxb-your-slack-bot-token
SLACK_CHANNEL=your-channel-name
TEAM_GROUP=your_slack_team_group_id

# Optional: Map Slack usernames to GitHub usernames if they differ
USER_MAPPING=slack_user1:github_user1,slack_user2:github_user2

# Optional: Enable debug logging
DEBUG=true
```

### Getting Required Tokens

#### GitHub Personal Access Token
1. Go to GitHub Settings → Developer settings → Personal access tokens
2. Generate new token (classic) with `repo` scope
3. Copy the token to your `.env` file

#### JIRA API Token
1. Go to Atlassian Account Settings → Security
2. Create and manage API tokens
3. Generate a new token and copy it to your `.env` file

#### Slack Bot Token
1. Create a new Slack App at [api.slack.com/apps](https://api.slack.com/apps)
2. Add required scopes (see [Slack Configuration](#slack-configuration))
3. Install the app to your workspace
4. Copy the Bot User OAuth Token to your `.env` file

#### Slack Team Group ID
Use the built-in command to list available user groups:

```bash
go run main.go --list-groups
```

Or manually create a user group in Slack and get its ID.

## 🔧 Slack Configuration

### Required Bot Token Scopes

Your Slack bot must have these scopes:

- `channels:read` - Read public channel information
- `groups:read` - Read private channel information
- `users:read` - Read user information
- `chat:write` - Send messages to channels
- `usergroups:read` - List user groups (for --list-groups command)

### Setup Steps

1. **Add Scopes**: In your Slack app settings, go to "OAuth & Permissions" and add the required scopes
2. **Reinstall App**: After adding scopes, reinstall the app to your workspace
3. **Add to Channel**: Invite your bot to the monitoring channel: `/invite @your-bot-name`
4. **Create User Group**: Create a user group for your team and note its ID

## 🚀 Usage

### Command Line Options

```bash
# Run immediately (for testing)
go run main.go --run-now
./pr-reporter --run-now

# View current configuration
go run main.go --print-config
./pr-reporter --print-config

# List available Slack user groups
go run main.go --list-groups
./pr-reporter --list-groups

# Run in production mode (scheduled execution)
go run main.go
./pr-reporter
```

### How It Works

1. **User Discovery**: Fetches all members from your specified Slack channel
2. **PR Filtering**: Finds open GitHub PRs with "Poker" label created by channel members
3. **JIRA Integration**: Extracts JIRA ticket numbers (POKER-XXXXX format) and fetches status
4. **Report Generation**: Creates formatted Slack message with proper mentions and links
5. **Scheduled Delivery**: Sends reports every weekday at 9:00 AM

### Filtering Criteria

The application includes pull requests that meet ALL of the following criteria:

- ✅ **Status**: Open (not closed or merged)
- ✅ **Label**: Contains "Poker" label (case-insensitive)
- ✅ **Author**: Created by users who are members of the specified Slack channel
- ✅ **JIRA Integration**: Optionally extracts ticket numbers matching pattern `POKER-\d+`

## 📊 Report Format

The Slack message follows this format:

```
:date: **2024-06-06**
:bar_chart: **Total Open PRs: 3**

1. **PR-1234** assigned to @developer1 | Jira: POKER-5678 | Fix user authentication | **In Progress**
2. **PR-1235** assigned to @developer2 | Jira: POKER-5679 | Add new feature | **To Do**
3. **PR-1236** assigned to @developer3 | Jira: POKER-5680 | Bug fix | **Done**

🚫 **Blocked:** PR-1234
📝 **Draft:** PR-1235

@team-group Please make sure to review these pull requests!
```

### Features of the Report

- **📅 Date Header**: Current date with calendar emoji
- **📊 Statistics**: Total count of open PRs with chart emoji
- **🔗 Clickable Links**: Direct links to both GitHub PRs and JIRA tickets
- **👤 Proper Mentions**: Actual Slack user mentions that send notifications
- **🚫📝 Status Summary**: Clear indication of blocked and draft PRs at the bottom
- **👥 Team Mention**: Mentions the entire team user group

## 🔍 Debug Mode

Enable comprehensive logging by setting `DEBUG=true` in your `.env` file:

```bash
DEBUG=true go run main.go --run-now
```

Debug output includes:
- Slack authentication verification
- Channel and user discovery process
- PR filtering decisions with explanations
- JIRA API calls and responses
- User mapping and mention formatting
- Final report structure

## 🚨 Troubleshooting

### Common Issues

#### No PRs Found
- **Check Channel Membership**: Ensure PR authors are in your Slack channel
- **Verify Labels**: Confirm PRs have the "Poker" label (case-insensitive)
- **Enable Debug Mode**: Use `DEBUG=true` to see filtering decisions

#### JIRA Status Shows "Unknown"
- **Verify Credentials**: Check JIRA URL, username, and API token
- **Check Permissions**: Ensure API token can access the specified tickets
- **URL Format**: Use `https://company.atlassian.net` (no trailing slash)

#### Slack Mentions Not Working
- **Bot Permissions**: Verify bot has `users:read` scope
- **Channel Access**: Ensure bot is added to the target channel
- **User Mapping**: Check `USER_MAPPING` for correct Slack↔GitHub username pairs

#### Missing Scope Errors
- **Add Required Scopes**: See [Slack Configuration](#slack-configuration)
- **Reinstall App**: After adding scopes, reinstall your Slack app
- **Check Token**: Ensure you're using the latest Bot User OAuth Token

### Error Messages

| Error | Solution |
|-------|----------|
| `missing_scope` | Add required Slack scopes and reinstall app |
| `channel not found` | Check channel name and bot permissions |
| `JIRA authentication failed` | Verify JIRA credentials and URL |
| `error verifying GitHub authentication` | Check GitHub token validity |


### Scheduling Notes

The application includes built-in scheduling (weekdays at 9:00 AM).

## 🔐 Security Best Practices

- **Never commit secrets**: Add `.env` to your `.gitignore` file
- **Restrict file permissions**: Use `chmod 600 .env` on Unix systems
- **Use least privilege**: Ensure tokens have only necessary permissions
- **Regular rotation**: Periodically rotate API tokens and access keys
- **Secure deployment**: Use secrets management in production environments
- **Monitor access**: Review API token usage and permissions regularly

## 🛠️ Customization

### Modifying Filtering Criteria

- **Label Filtering**: Change label check in `FetchPullRequests()` function
- **JIRA Ticket Pattern**: Update the regex pattern for different ticket formats
- **Blocked Detection**: Adjust JIRA status/label checks in `UpdateJiraInfo()`
- **Schedule**: Modify the cron expression in the `main()` function

### Message Formatting

Customize the Slack message template in `SendSlackMessage()`:
- Add additional emojis or formatting
- Include more PR metadata
- Modify the blocked/draft summary format
- Change the team mention format

### Adding New Integrations

The modular structure allows easy addition of:
- Additional project management tools (Asana, Linear, etc.)
- Different notification channels (Teams, Discord, etc.)
- Custom filtering criteria
- Additional PR metadata sources


## 🆘 Support

If you encounter any issues:

1. **Check the troubleshooting section** above
2. **Enable debug mode** for detailed logging: `DEBUG=true`
3. **Review the configuration** requirements
4. **Open an issue** in the GitHub repository with:
   - Debug logs (excluding sensitive tokens)
   - Configuration details (excluding secrets)
   - Steps to reproduce the issue
   - Expected vs actual behavior

---

**Note**: This tool is designed for team productivity and requires appropriate API access to GitHub, JIRA, and Slack. 

This is a test for a PR.
