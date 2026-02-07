package github

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/google/go-github/v45/github"
	"golang.org/x/oauth2"
)

// FetchOptions contains options for fetching PRs from GitHub
type FetchOptions struct {
	Token         string   // GitHub API token
	Owner         string   // Repository owner
	Repo          string   // Repository name
	Labels        []string // Labels to filter by (if empty, fetch all open PRs)
	AllowedUsers  []string // Users whose PRs to include
	DebugMode     bool     // Enable debug logging
}

// PRResult represents a single PR fetched from GitHub
type PRResult struct {
	Number      int
	Title       string
	URL         string
	Assignee    string  // GitHub username (not Slack format yet)
	JiraTicket  string
	IsDraft     bool
	Labels      []string
	Author      string
}

// FetchPRs fetches pull requests from a GitHub repository based on provided options
// If no labels are specified, it fetches all open PRs from the repo
// If labels are specified, it only fetches PRs with at least one matching label
func FetchPRs(opts FetchOptions) ([]*PRResult, error) {
	if opts.Token == "" {
		return nil, fmt.Errorf("GitHub token is required")
	}
	if opts.Owner == "" {
		return nil, fmt.Errorf("repository owner is required")
	}
	if opts.Repo == "" {
		return nil, fmt.Errorf("repository name is required")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: opts.Token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// Verify authentication
	if opts.DebugMode {
		user, _, err := client.Users.Get(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("error verifying GitHub authentication: %v", err)
		}
		log.Printf("Debug: Authenticated as GitHub user: %s", *user.Login)
	}

	// Set up GitHub list options
	listOpts := &github.PullRequestListOptions{
		State: "open",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	allPRs, _, err := client.PullRequests.List(ctx, opts.Owner, opts.Repo, listOpts)
	if err != nil {
		return nil, fmt.Errorf("error fetching PRs from %s/%s: %v", opts.Owner, opts.Repo, err)
	}

	if opts.DebugMode {
		log.Printf("Debug: Found %d total open PRs in %s/%s", len(allPRs), opts.Owner, opts.Repo)
	}

	var filteredPRs []*PRResult

	// Regex to extract JIRA ticket (matches POKER-#### format)
	jiraRegex := regexp.MustCompile(`POKER-\d+`)

	for _, pr := range allPRs {
		// Debug PR info
		if opts.DebugMode {
			log.Printf("Debug: Examining PR #%d: %s", *pr.Number, *pr.Title)
			log.Printf("Debug: PR created by: %s", *pr.User.Login)
			log.Printf("Debug: PR is draft: %t", *pr.Draft)

			labelNames := make([]string, 0, len(pr.Labels))
			for _, label := range pr.Labels {
				labelNames = append(labelNames, *label.Name)
			}
			log.Printf("Debug: PR labels: %s", strings.Join(labelNames, ", "))
		}

		// Skip if no user info
		if pr.User == nil || pr.User.Login == nil {
			if opts.DebugMode {
				log.Printf("Debug: PR #%d skipped - no user", *pr.Number)
			}
			continue
		}

		// Filter by allowed users if specified
		if len(opts.AllowedUsers) > 0 {
			userFound := false
			for _, allowedUser := range opts.AllowedUsers {
				allowedUser = strings.TrimSpace(allowedUser)
				if allowedUser == "" {
					continue
				}

				if strings.EqualFold(allowedUser, *pr.User.Login) {
					userFound = true
					if opts.DebugMode {
						log.Printf("Debug: PR #%d matches allowed user: %s", *pr.Number, allowedUser)
					}
					break
				}
			}

			if !userFound {
				if opts.DebugMode {
					log.Printf("Debug: PR #%d skipped - user %s not in allowed user list", *pr.Number, *pr.User.Login)
				}
				continue
			}
		}

		// Filter by labels if specified
		if len(opts.Labels) > 0 {
			hasMatchingLabel := false
			for _, label := range pr.Labels {
				if label.Name != nil {
					for _, filterLabel := range opts.Labels {
						// Case-insensitive partial match
						if strings.Contains(strings.ToLower(*label.Name), strings.ToLower(filterLabel)) {
							hasMatchingLabel = true
							if opts.DebugMode {
								log.Printf("Debug: PR #%d has matching label: %s (matches filter: %s)", 
									*pr.Number, *label.Name, filterLabel)
							}
							break
						}
					}
					if hasMatchingLabel {
						break
					}
				}
			}

			if !hasMatchingLabel {
				if opts.DebugMode {
					log.Printf("Debug: PR #%d skipped - no matching label found from: %v", 
						*pr.Number, opts.Labels)
				}
				continue
			}
		}

		// Extract JIRA ticket from PR title
		jiraTicket := ""
		if pr.Title != nil {
			matches := jiraRegex.FindStringSubmatch(*pr.Title)
			if len(matches) > 0 {
				jiraTicket = matches[0]
			}

			if opts.DebugMode && jiraTicket != "" {
				log.Printf("Debug: PR #%d JIRA ticket extracted: %s", *pr.Number, jiraTicket)
			}
		}

		// Extract labels
		prLabels := make([]string, 0, len(pr.Labels))
		for _, label := range pr.Labels {
			if label.Name != nil {
				prLabels = append(prLabels, *label.Name)
			}
		}

		// Get assignee (just GitHub username, no Slack formatting yet)
		assignee := ""
		if pr.Assignee != nil && pr.Assignee.Login != nil {
			assignee = *pr.Assignee.Login
		}

		// Create PR result
		prResult := &PRResult{
			Number:     *pr.Number,
			Title:      *pr.Title,
			URL:        *pr.HTMLURL,
			Assignee:   assignee,
			JiraTicket: jiraTicket,
			IsDraft:    *pr.Draft,
			Labels:     prLabels,
			Author:     *pr.User.Login,
		}

		if opts.DebugMode {
			log.Printf("Debug: PR #%d matched all criteria and is included", *pr.Number)
			log.Printf("Debug: PR #%d draft status: %t", *pr.Number, prResult.IsDraft)
			log.Printf("Debug: PR #%d assignee: %s", *pr.Number, prResult.Assignee)
		}

		filteredPRs = append(filteredPRs, prResult)
	}

	if opts.DebugMode {
		log.Printf("Debug: Filtered to %d PRs matching criteria", len(filteredPRs))
	}

	return filteredPRs, nil
}
