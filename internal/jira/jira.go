package jira

import (
	"fmt"
	"log"
	"strings"

	"github.com/andygrunwald/go-jira"
)

// FetchOptions contains options for fetching JIRA ticket information
type FetchOptions struct {
	URL       string // JIRA base URL
	Username  string // JIRA username (for Basic auth)
	APIToken  string // JIRA API token or Personal Access Token
	UsePAT    bool   // Use Personal Access Token instead of Basic auth
	DebugMode bool   // Enable debug logging
}

// TicketInfo represents information about a JIRA ticket
type TicketInfo struct {
	TicketID  string
	Status    string
	Summary   string
	IsBlocked bool
}

// FetchTicketInfo fetches information for a single JIRA ticket
func FetchTicketInfo(opts FetchOptions, ticketID string) (*TicketInfo, error) {
	if ticketID == "" {
		return nil, fmt.Errorf("ticket ID is required")
	}

	// Check JIRA credentials
	if opts.Username == "" || opts.APIToken == "" || opts.URL == "" {
		return nil, fmt.Errorf("JIRA credentials not fully configured")
	}

	if opts.DebugMode {
		log.Printf("Debug: Initializing JIRA client for %s", opts.URL)
		log.Printf("Debug: Using PAT authentication: %v", opts.UsePAT)
	}

	// Create JIRA client with appropriate authentication
	var jiraClient *jira.Client
	if opts.UsePAT {
		if opts.DebugMode {
			log.Println("Debug: Using JIRA Personal Access Token authentication")
		}

		tp := jira.PATAuthTransport{
			Token: opts.APIToken,
		}

		var err error
		jiraClient, err = jira.NewClient(tp.Client(), opts.URL)
		if err != nil {
			return nil, fmt.Errorf("error creating JIRA client with PAT: %v", err)
		}
	} else {
		if opts.DebugMode {
			log.Println("Debug: Using JIRA Basic authentication (email + API token)")
		}

		tp := jira.BasicAuthTransport{
			Username: opts.Username,
			Password: opts.APIToken,
		}

		var err error
		jiraClient, err = jira.NewClient(tp.Client(), opts.URL)
		if err != nil {
			return nil, fmt.Errorf("error creating JIRA client with Basic auth: %v", err)
		}
	}

	// Test JIRA connection in debug mode
	if opts.DebugMode {
		log.Printf("Debug: Testing JIRA connection to %s", opts.URL)
		myself, _, err := jiraClient.User.GetSelf()
		if err != nil {
			log.Printf("Debug: JIRA authentication test failed: %v", err)
		} else {
			log.Printf("Debug: Successfully authenticated to JIRA as: %s", myself.DisplayName)
		}
	}

	if opts.DebugMode {
		log.Printf("Debug: Fetching JIRA info for ticket %s", ticketID)
	}

	issue, resp, err := jiraClient.Issue.Get(ticketID, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return &TicketInfo{
				TicketID:  ticketID,
				Status:    "Not Found",
				Summary:   "Ticket not found",
				IsBlocked: false,
			}, nil
		}
		return nil, fmt.Errorf("error fetching JIRA ticket %s: %v", ticketID, err)
	}

	ticketInfo := &TicketInfo{
		TicketID:  ticketID,
		Status:    "Unknown",
		Summary:   "",
		IsBlocked: false,
	}

	// Extract status and description
	if issue != nil && issue.Fields != nil {
		// Extract status
		if issue.Fields.Status != nil && issue.Fields.Status.Name != "" {
			ticketInfo.Status = issue.Fields.Status.Name
			if opts.DebugMode {
				log.Printf("Debug: JIRA ticket %s status: %s", ticketID, ticketInfo.Status)
			}
		} else {
			ticketInfo.Status = "No Status"
			if opts.DebugMode {
				log.Printf("Debug: JIRA ticket %s has no status field", ticketID)
			}
		}

		// Extract description/summary
		if issue.Fields.Summary != "" {
			ticketInfo.Summary = issue.Fields.Summary
			if opts.DebugMode {
				log.Printf("Debug: JIRA ticket %s summary: %s", ticketID, ticketInfo.Summary)
			}
		} else {
			ticketInfo.Summary = "No Description"
		}

		// Check if blocked by status name
		if issue.Fields.Status != nil && issue.Fields.Status.Name != "" {
			statusName := strings.ToLower(issue.Fields.Status.Name)
			if strings.Contains(statusName, "block") ||
				strings.Contains(statusName, "impediment") ||
				strings.Contains(statusName, "pause") {
				ticketInfo.IsBlocked = true
				if opts.DebugMode {
					log.Printf("Debug: JIRA ticket %s marked as blocked due to status: %s", ticketID, issue.Fields.Status.Name)
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
					ticketInfo.IsBlocked = true
					if opts.DebugMode {
						log.Printf("Debug: JIRA ticket %s marked as blocked due to label: %s", ticketID, label)
					}
					break
				}
			}
		}
	} else {
		ticketInfo.Status = "No Data"
		if opts.DebugMode {
			log.Printf("Debug: JIRA ticket %s returned no usable data", ticketID)
		}
	}

	if opts.DebugMode {
		log.Printf("Debug: Final status for JIRA %s: %s (blocked: %v)", ticketID, ticketInfo.Status, ticketInfo.IsBlocked)
	}

	return ticketInfo, nil
}

// FetchTicketsInfo fetches information for multiple JIRA tickets
func FetchTicketsInfo(opts FetchOptions, ticketIDs []string) (map[string]*TicketInfo, error) {
	results := make(map[string]*TicketInfo)

	for _, ticketID := range ticketIDs {
		if ticketID == "" {
			continue
		}

		ticketInfo, err := FetchTicketInfo(opts, ticketID)
		if err != nil {
			log.Printf("Warning: Error fetching JIRA ticket %s: %v", ticketID, err)
			// Store error info
			results[ticketID] = &TicketInfo{
				TicketID:  ticketID,
				Status:    "Error",
				Summary:   fmt.Sprintf("Error: %v", err),
				IsBlocked: false,
			}
			continue
		}

		results[ticketID] = ticketInfo
	}

	return results, nil
}
