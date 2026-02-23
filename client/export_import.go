package client

import (
	"context"
	"fmt"

	"github.com/persistorai/persistor/internal/models"
)

// Export retrieves a full-fidelity export of the knowledge graph.
func (c *Client) Export(ctx context.Context) (*models.ExportFormat, error) {
	var result models.ExportFormat
	if err := c.get(ctx, "/api/v1/export", nil, &result); err != nil {
		return nil, fmt.Errorf("export: %w", err)
	}

	return &result, nil
}

// Import writes an export payload into the knowledge graph.
func (c *Client) Import(ctx context.Context, data *models.ExportFormat, opts models.ImportOptions) (*models.ImportResult, error) {
	query := ""
	sep := "?"

	if opts.OverwriteExisting {
		query += sep + "overwrite=true"
		sep = "&"
	}

	if opts.DryRun {
		query += sep + "dry_run=true"
		sep = "&"
	}

	if opts.RegenerateEmbeddings {
		query += sep + "regenerate_embeddings=true"
		sep = "&"
	}

	if opts.ResetUsage {
		query += sep + "reset_usage=true"
	}

	var result models.ImportResult
	if err := c.post(ctx, "/api/v1/import"+query, data, &result); err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}

	return &result, nil
}

// ValidateImport checks an export payload for consistency errors.
func (c *Client) ValidateImport(ctx context.Context, data *models.ExportFormat) ([]string, error) {
	var result struct {
		Errors []string `json:"errors"`
		Valid  bool     `json:"valid"`
	}

	if err := c.post(ctx, "/api/v1/import/validate", data, &result); err != nil {
		return nil, fmt.Errorf("validate import: %w", err)
	}

	return result.Errors, nil
}
