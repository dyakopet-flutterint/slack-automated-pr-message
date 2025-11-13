# PR Reporter

A comprehensive Golang-based automation tool that monitors GitHub pull requests, integrates with JIRA for ticket status tracking, and sends daily reports to Slack channels. Designed to streamline development team workflows by providing automated visibility into open pull requests and their associated JIRA task statuses.

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
go build
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

# Optional: Use JIRA Personal Access Token instead of Basic Auth
# Set to true if using PAT, false or omit for email + API token
JIRA_USE_PAT=false

# Slack Configuration
SLACK_TOKEN=xoxb-your-slack-bot-token
SLACK_CHANNEL=your-channel-name
TEAM_GROUP=your_slack_team_group_id

# Required: Map Slack user IDs to GitHub usernames
# Only users in this mapping will have their PRs included in reports
USER_MAPPING=U0559T3P67J:github_user1,U082AFK42N6:github_user2

# Optional: Enable debug logging
DEBUG=true
```


Or manually create a user group in Slack and get its ID.

## 🔧 Slack Configuration

### Required Bot Token Scopes

Your Slack bot must have these scopes:

- `channels:read` - Read public channel information
- `groups:read` - Read private channel information
- `users:read` - Read user information
- `chat:write` - Send messages to channels

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

# View current configuration
go run main.go --print-config

# List available Slack user groups
go run main.go --list-groups

# Run in production mode (scheduled execution)
go run main.go
```

## 🚨 Troubleshooting

### Common Issues

#### No PRs Found
- **Check User Mapping**: Ensure all PR authors have their Slack user ID mapped to their GitHub username in `USER_MAPPING`
  - Users without mappings will be skipped
  - Enable `DEBUG=true` to see which users are being skipped
- **Check Channel Membership**: Ensure PR authors are in your Slack channel
- **Verify Labels**: Confirm PRs have the "Poker" label (case-insensitive)
- **Enable Debug Mode**: Use `DEBUG=true` to see filtering decisions and user mappings

#### JIRA Status Shows "Unknown"
- **Verify Credentials**: Check JIRA URL, username, and API token
- **Check Authentication Type**: Ensure `JIRA_USE_PAT` is set correctly for your token type
  - Set to `true` if using Personal Access Token
  - Set to `false` or omit if using email + API token
- **Check Permissions**: Ensure API token/PAT can access the specified tickets
- **URL Format**: Use `https://company.atlassian.net` (no trailing slash)

#### Slack Mentions Not Working
- **Bot Permissions**: Verify bot has `users:read` scope
- **Channel Access**: Ensure bot is added to the target channel
- **User Mapping**: Check `USER_MAPPING` for correct Slack user ID ↔ GitHub username pairs

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