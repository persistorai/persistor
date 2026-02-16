package client

import (
	"context"
	"net/url"
	"strconv"
)

// AuditService handles audit log operations.
type AuditService struct {
	c *Client
}

// auditQueryResponse wraps the paginated audit query response.
type auditQueryResponse struct {
	Data    []AuditEntry `json:"data"`
	HasMore bool         `json:"has_more"`
}

// Query returns audit log entries matching the given options.
func (s *AuditService) Query(ctx context.Context, opts *AuditQueryOptions) ([]AuditEntry, bool, error) {
	params := url.Values{}
	if opts != nil {
		if opts.EntityType != "" {
			params.Set("entity_type", opts.EntityType)
		}
		if opts.EntityID != "" {
			params.Set("entity_id", opts.EntityID)
		}
		if opts.Action != "" {
			params.Set("action", opts.Action)
		}
		if opts.Since != nil {
			params.Set("since", opts.Since.Format("2006-01-02T15:04:05Z07:00"))
		}
		if opts.Limit > 0 {
			params.Set("limit", strconv.Itoa(opts.Limit))
		}
		if opts.Offset > 0 {
			params.Set("offset", strconv.Itoa(opts.Offset))
		}
	}
	var resp auditQueryResponse
	if err := s.c.get(ctx, "/api/v1/audit", params, &resp); err != nil {
		return nil, false, err
	}
	return resp.Data, resp.HasMore, nil
}

// Purge deletes audit entries older than retentionDays. Returns count deleted.
func (s *AuditService) Purge(ctx context.Context, retentionDays int) (int, error) {
	params := url.Values{}
	if retentionDays > 0 {
		params.Set("retention_days", strconv.Itoa(retentionDays))
	}
	var resp struct {
		Deleted       int `json:"deleted"`
		RetentionDays int `json:"retention_days"`
	}
	if err := s.c.del(ctx, "/api/v1/audit", params, &resp); err != nil {
		return 0, err
	}
	return resp.Deleted, nil
}
