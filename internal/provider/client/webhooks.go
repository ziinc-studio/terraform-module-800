package client

import (
	"context"
	"fmt"
	"net/url"
)

// Webhook mirrors the API's Webhook schema:
// https://api.800.com/docs (component: Webhook).
type Webhook struct {
	ID       int64    `json:"id,omitempty"`
	URL      string   `json:"url"`
	Method   string   `json:"method"`
	Features []string `json:"features"`
}

// CreateWebhook -> POST /v2/companies/{company}/webhooks
func (c *Client) CreateWebhook(ctx context.Context, companyID int64, w Webhook) (*Webhook, error) {
	var out Webhook
	path := fmt.Sprintf("/v2/companies/%d/webhooks", companyID)
	if err := c.Do(ctx, "POST", path, nil, w, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetWebhook -> GET /v2/companies/{company}/webhooks (filtered client-side
// for the matching id). The list endpoint is the only documented read
// path; there is no GET /webhooks/{id}.
func (c *Client) GetWebhook(ctx context.Context, companyID, webhookID int64) (*Webhook, error) {
	q := url.Values{}
	// no per-id filter exposed — page until found.
	cursor := ""
	for {
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		path := fmt.Sprintf("/v2/companies/%d/webhooks", companyID)
		var page struct {
			Items []Webhook `json:"-"`
			Meta  struct {
				NextCursor *string `json:"nextCursor"`
			} `json:"meta"`
		}
		// The data envelope is an array here, not an object — decode in two passes.
		var raw struct {
			Data []Webhook `json:"data"`
			Meta struct {
				NextCursor *string `json:"nextCursor"`
			} `json:"meta"`
		}
		if err := c.Do(ctx, "GET", path, q, nil, &raw); err != nil {
			return nil, err
		}
		page.Items = raw.Data
		page.Meta.NextCursor = raw.Meta.NextCursor
		for i := range page.Items {
			if page.Items[i].ID == webhookID {
				w := page.Items[i]
				return &w, nil
			}
		}
		if page.Meta.NextCursor == nil || *page.Meta.NextCursor == "" {
			break
		}
		cursor = *page.Meta.NextCursor
	}
	return nil, &APIError{StatusCode: 404, Message: fmt.Sprintf("webhook %d not found", webhookID)}
}

// UpdateWebhook -> PUT /v2/companies/{company}/webhooks/{webhook}
func (c *Client) UpdateWebhook(ctx context.Context, companyID, webhookID int64, w Webhook) (*Webhook, error) {
	var out Webhook
	path := fmt.Sprintf("/v2/companies/%d/webhooks/%d", companyID, webhookID)
	if err := c.Do(ctx, "PUT", path, nil, w, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteWebhook -> DELETE /v2/companies/{company}/webhooks/{webhook}
// Treats 404 as success — the destroy path must be idempotent.
func (c *Client) DeleteWebhook(ctx context.Context, companyID, webhookID int64) error {
	path := fmt.Sprintf("/v2/companies/%d/webhooks/%d", companyID, webhookID)
	if err := c.Do(ctx, "DELETE", path, nil, nil, nil); err != nil && !IsNotFound(err) {
		return err
	}
	return nil
}
