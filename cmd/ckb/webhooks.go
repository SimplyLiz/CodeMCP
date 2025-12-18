package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"ckb/internal/logging"
	"ckb/internal/paths"
	"ckb/internal/webhooks"
)

var webhooksCmd = &cobra.Command{
	Use:   "webhooks",
	Short: "Manage webhook configurations",
	Long: `Manage outbound webhooks for CKB event notifications.

Webhooks can send notifications to Slack, PagerDuty, Discord, or generic HTTP endpoints
when events occur (architecture refresh, job completion, health checks).`,
}

var webhooksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured webhooks",
	Long: `List all configured webhooks with their status.

Examples:
  ckb webhooks list
  ckb webhooks list --format=json`,
	RunE: runWebhooksList,
}

var webhooksTestCmd = &cobra.Command{
	Use:   "test <webhook-id>",
	Short: "Send a test event to a webhook",
	Long: `Send a test event to verify webhook configuration.

Examples:
  ckb webhooks test wh_abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runWebhooksTest,
}

var webhooksDeliveriesCmd = &cobra.Command{
	Use:   "deliveries <webhook-id>",
	Short: "Show delivery history for a webhook",
	Long: `Show recent delivery attempts, successes, and failures for a webhook.

Examples:
  ckb webhooks deliveries wh_abc123
  ckb webhooks deliveries wh_abc123 --status=failed
  ckb webhooks deliveries wh_abc123 --limit=50`,
	Args: cobra.ExactArgs(1),
	RunE: runWebhooksDeliveries,
}

// Webhook flags
var (
	webhooksFormat          string
	webhooksDeliveryStatus  string
	webhooksDeliveryLimit   int
)

func init() {
	// Add webhooks command to root
	rootCmd.AddCommand(webhooksCmd)

	// Add subcommands
	webhooksCmd.AddCommand(webhooksListCmd)
	webhooksCmd.AddCommand(webhooksTestCmd)
	webhooksCmd.AddCommand(webhooksDeliveriesCmd)

	// List flags
	webhooksListCmd.Flags().StringVar(&webhooksFormat, "format", "human", "Output format (json, human)")

	// Deliveries flags
	webhooksDeliveriesCmd.Flags().StringVar(&webhooksDeliveryStatus, "status", "", "Filter by status (queued, pending, delivered, failed, dead)")
	webhooksDeliveriesCmd.Flags().IntVar(&webhooksDeliveryLimit, "limit", 20, "Maximum deliveries to return")
	webhooksDeliveriesCmd.Flags().StringVar(&webhooksFormat, "format", "human", "Output format (json, human)")
}

func runWebhooksList(cmd *cobra.Command, args []string) error {
	// Get daemon directory
	daemonDir, err := paths.GetDaemonDir()
	if err != nil {
		return fmt.Errorf("daemon not configured: %w", err)
	}

	// Create logger
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})

	// Open webhook manager
	mgr, err := webhooks.NewManager(daemonDir, logger, webhooks.DefaultConfig())
	if err != nil {
		return fmt.Errorf("failed to access webhooks: %w", err)
	}

	// List webhooks
	hooks, err := mgr.ListWebhooks()
	if err != nil {
		return fmt.Errorf("failed to list webhooks: %w", err)
	}

	// Convert to summaries
	summaries := make([]webhooks.WebhookSummary, 0, len(hooks))
	for _, h := range hooks {
		summaries = append(summaries, h.ToSummary())
	}

	result := map[string]interface{}{
		"webhooks":   summaries,
		"totalCount": len(summaries),
	}

	// Format and output
	output, err := FormatResponse(result, OutputFormat(webhooksFormat))
	if err != nil {
		return fmt.Errorf("error formatting output: %w", err)
	}

	fmt.Println(output)
	return nil
}

func runWebhooksTest(cmd *cobra.Command, args []string) error {
	webhookID := args[0]

	// Get daemon directory
	daemonDir, err := paths.GetDaemonDir()
	if err != nil {
		return fmt.Errorf("daemon not configured: %w", err)
	}

	// Create logger
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})

	// Open webhook manager
	mgr, err := webhooks.NewManager(daemonDir, logger, webhooks.DefaultConfig())
	if err != nil {
		return fmt.Errorf("failed to access webhooks: %w", err)
	}

	// Test the webhook
	if err := mgr.TestWebhook(webhookID); err != nil {
		return fmt.Errorf("failed to test webhook: %w", err)
	}

	fmt.Printf("Test event queued for webhook %s\n", webhookID)
	return nil
}

func runWebhooksDeliveries(cmd *cobra.Command, args []string) error {
	webhookID := args[0]

	// Get daemon directory
	daemonDir, err := paths.GetDaemonDir()
	if err != nil {
		return fmt.Errorf("daemon not configured: %w", err)
	}

	// Create logger
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})

	// Open webhook manager
	mgr, err := webhooks.NewManager(daemonDir, logger, webhooks.DefaultConfig())
	if err != nil {
		return fmt.Errorf("failed to access webhooks: %w", err)
	}

	// Build filter options
	opts := webhooks.ListDeliveriesOptions{
		WebhookID: webhookID,
		Limit:     webhooksDeliveryLimit,
	}

	if webhooksDeliveryStatus != "" {
		opts.Status = []webhooks.DeliveryStatus{webhooks.DeliveryStatus(webhooksDeliveryStatus)}
	}

	result, err := mgr.ListDeliveries(opts)
	if err != nil {
		return fmt.Errorf("failed to list deliveries: %w", err)
	}

	// Format and output
	output, err := FormatResponse(result, OutputFormat(webhooksFormat))
	if err != nil {
		return fmt.Errorf("error formatting output: %w", err)
	}

	fmt.Println(output)
	return nil
}
