package api

// Canvas API wrappers for Slack canvases.* endpoints.
//
// This file is intentionally additive-only: it defines methods on *Client
// that reuse the existing Post() helper in client.go. Per SPEC-050 v5
// acceptance #11, internal/api/client.go is NOT modified.
//
// Slack accepts both application/x-www-form-urlencoded and application/json
// for canvases.* methods. We use form-encoded params (matching the existing
// client) and JSON-marshal nested document_content / criteria objects when
// passed as a single string param, which Slack supports.
//
// API reference: https://docs.slack.dev/reference/methods/canvases.create/

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// === Types ===

// CanvasDocumentContent represents a Slack canvas document body.
// type=markdown is currently the only documented value.
type CanvasDocumentContent struct {
	Type     string `json:"type"`              // "markdown"
	Markdown string `json:"markdown,omitempty"`
}

// CanvasChange represents one edit operation in a canvases.edit batch.
//
// op:
//   - "insert_after"     — insert markdown after a section
//   - "insert_before"    — insert markdown before a section
//   - "insert_at_start"  — insert markdown at canvas start
//   - "insert_at_end"    — insert markdown at canvas end
//   - "replace"          — replace section content
//   - "delete"           — delete a section
//
// section_id is required for all ops except insert_at_start / insert_at_end.
// document_content is required for all ops except delete.
type CanvasChange struct {
	Operation       string                 `json:"operation"`
	SectionID       string                 `json:"section_id,omitempty"`
	DocumentContent *CanvasDocumentContent `json:"document_content,omitempty"`
}

// CanvasInfo describes a single canvas in list/get responses.
// Slack does not document a stable schema for the legacy canvases.list
// endpoint, so we keep it permissive (RawMessage for unknown sub-objects).
type CanvasInfo struct {
	ID         string          `json:"id"`
	Title      string          `json:"title,omitempty"`
	OwnerID    string          `json:"owner_id,omitempty"`
	ChannelID  string          `json:"channel_id,omitempty"`
	URL        string          `json:"url,omitempty"`
	DateCreated int64          `json:"date_created,omitempty"`
	DateUpdated int64          `json:"date_updated,omitempty"`
	Extra      json.RawMessage `json:"-"` // reserved
}

// CanvasSection describes a section returned by canvases.sections.lookup.
type CanvasSection struct {
	ID      string `json:"id"`
	Type    string `json:"type,omitempty"`
	Content string `json:"content,omitempty"`
}

// === List ===

type CanvasesListResponse struct {
	SlackResponse
	Canvases         []CanvasInfo `json:"canvases"`
	ResponseMetadata struct {
		NextCursor string `json:"next_cursor"`
	} `json:"response_metadata"`
}

// CanvasesList lists canvases visible to the caller. channelID is optional;
// when set, results are scoped to that channel.
func (c *Client) CanvasesList(channelID string, limit int) (*CanvasesListResponse, error) {
	params := url.Values{}
	if channelID != "" {
		params.Set("channel_id", channelID)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	data, err := c.Post("canvases.list", params)
	if err != nil {
		return nil, err
	}
	var resp CanvasesListResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode canvases.list: %w", err)
	}
	return &resp, nil
}

// === Get ===

type CanvasGetResponse struct {
	SlackResponse
	Canvas CanvasInfo      `json:"canvas"`
	Markdown string        `json:"markdown,omitempty"` // when format=markdown
	Raw    json.RawMessage `json:"-"`
}

// CanvasesGet fetches a single canvas. format may be "" (default), "markdown",
// or "raw" — passed through to Slack.
func (c *Client) CanvasesGet(canvasID, format string) (*CanvasGetResponse, error) {
	params := url.Values{}
	params.Set("canvas_id", canvasID)
	if format != "" {
		params.Set("format", format)
	}
	data, err := c.Post("canvases.get", params)
	if err != nil {
		return nil, err
	}
	var resp CanvasGetResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode canvases.get: %w", err)
	}
	resp.Raw = data
	return &resp, nil
}

// === Create ===

type CanvasCreateResponse struct {
	SlackResponse
	CanvasID string `json:"canvas_id"`
}

// CanvasesCreate creates a standalone canvas. If channelID is set,
// conversations.canvases.create is called instead so the canvas tabs into
// that channel automatically.
func (c *Client) CanvasesCreate(title, markdown, channelID string) (*CanvasCreateResponse, error) {
	doc := CanvasDocumentContent{Type: "markdown", Markdown: markdown}
	docJSON, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("encode document_content: %w", err)
	}

	params := url.Values{}
	params.Set("title", title)
	params.Set("document_content", string(docJSON))

	method := "canvases.create"
	if channelID != "" {
		params.Set("channel_id", channelID)
		method = "conversations.canvases.create"
	}

	data, err := c.Post(method, params)
	if err != nil {
		return nil, err
	}
	var resp CanvasCreateResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode %s: %w", method, err)
	}
	return &resp, nil
}

// === Edit ===

// CanvasesEdit applies a batch of changes (insert/replace/delete sections)
// to a canvas in a single API call.
func (c *Client) CanvasesEdit(canvasID string, changes []CanvasChange) error {
	if len(changes) == 0 {
		return fmt.Errorf("canvases.edit requires at least one change")
	}
	changesJSON, err := json.Marshal(changes)
	if err != nil {
		return fmt.Errorf("encode changes: %w", err)
	}
	params := url.Values{}
	params.Set("canvas_id", canvasID)
	params.Set("changes", string(changesJSON))

	data, err := c.Post("canvases.edit", params)
	if err != nil {
		return err
	}
	var resp SlackResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("decode canvases.edit: %w", err)
	}
	return nil
}

// === Delete ===

// CanvasesDelete deletes a canvas.
func (c *Client) CanvasesDelete(canvasID string) error {
	params := url.Values{}
	params.Set("canvas_id", canvasID)
	if _, err := c.Post("canvases.delete", params); err != nil {
		return err
	}
	return nil
}

// === Sections lookup ===

type CanvasSectionsLookupResponse struct {
	SlackResponse
	Sections []CanvasSection `json:"sections"`
}

// CanvasesSectionsLookup returns the section list for a canvas (used to
// resolve section_id values for edit operations).
func (c *Client) CanvasesSectionsLookup(canvasID string) (*CanvasSectionsLookupResponse, error) {
	params := url.Values{}
	params.Set("canvas_id", canvasID)
	data, err := c.Post("canvases.sections.lookup", params)
	if err != nil {
		return nil, err
	}
	var resp CanvasSectionsLookupResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode canvases.sections.lookup: %w", err)
	}
	return &resp, nil
}

// === Access ===

// CanvasesAccessSet grants users access to a canvas at a level
// ("read" or "write").
func (c *Client) CanvasesAccessSet(canvasID string, userIDs []string, level string) error {
	if len(userIDs) == 0 {
		return fmt.Errorf("canvases.access.set requires at least one user_id")
	}
	if level != "read" && level != "write" {
		return fmt.Errorf("canvases.access.set: level must be read|write, got %q", level)
	}
	uidsJSON, err := json.Marshal(userIDs)
	if err != nil {
		return fmt.Errorf("encode user_ids: %w", err)
	}
	params := url.Values{}
	params.Set("canvas_id", canvasID)
	params.Set("access_level", level)
	params.Set("user_ids", string(uidsJSON))
	if _, err := c.Post("canvases.access.set", params); err != nil {
		return err
	}
	return nil
}

// CanvasesAccessDelete revokes user access to a canvas.
func (c *Client) CanvasesAccessDelete(canvasID string, userIDs []string) error {
	if len(userIDs) == 0 {
		return fmt.Errorf("canvases.access.delete requires at least one user_id")
	}
	uidsJSON, err := json.Marshal(userIDs)
	if err != nil {
		return fmt.Errorf("encode user_ids: %w", err)
	}
	params := url.Values{}
	params.Set("canvas_id", canvasID)
	params.Set("user_ids", string(uidsJSON))
	if _, err := c.Post("canvases.access.delete", params); err != nil {
		return err
	}
	return nil
}
