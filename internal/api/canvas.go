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
//
// Slack canvases are exposed via the files API. There is NO `canvases.list`
// method (confirmed via https://api.slack.com/methods?filter=canvases — empty).
// Per https://docs.slack.dev/surfaces/canvases/ we use
// `files.list` with `types=canvases` (filter by canvas filetype).

type filesListFile struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Name       string   `json:"name"`
	Filetype   string   `json:"filetype"`
	Mimetype   string   `json:"mimetype"`
	User       string   `json:"user"`
	Channels   []string `json:"channels"`
	Permalink  string   `json:"permalink"`
	URLPrivate string   `json:"url_private"`
	Created    int64    `json:"created"`
	Updated    int64    `json:"updated"`
}

type filesListResponse struct {
	SlackResponse
	Files  []filesListFile `json:"files"`
	Paging struct {
		Count int `json:"count"`
		Total int `json:"total"`
		Page  int `json:"page"`
		Pages int `json:"pages"`
	} `json:"paging"`
}

type CanvasesListResponse struct {
	SlackResponse
	Canvases         []CanvasInfo `json:"canvases"`
	ResponseMetadata struct {
		NextCursor string `json:"next_cursor"`
	} `json:"response_metadata"`
}

// CanvasesList lists canvases visible to the caller. channelID is optional;
// when set, results are scoped to that channel.
//
// Underlying Slack method: files.list with types=canvases (the
// non-existent canvases.list shipped in v0.4.0 is replaced).
// Canvas IDs returned are F-prefix file IDs (e.g. F0ASWF3SRST).
func (c *Client) CanvasesList(channelID string, limit int) (*CanvasesListResponse, error) {
	params := url.Values{}
	params.Set("types", "canvases")
	if channelID != "" {
		params.Set("channel", channelID)
	}
	if limit > 0 {
		params.Set("count", fmt.Sprintf("%d", limit))
	}
	data, err := c.Post("files.list", params)
	if err != nil {
		return nil, err
	}
	var fl filesListResponse
	if err := json.Unmarshal(data, &fl); err != nil {
		return nil, fmt.Errorf("decode files.list: %w", err)
	}
	resp := &CanvasesListResponse{SlackResponse: fl.SlackResponse}
	for _, f := range fl.Files {
		// Defensive filter — types=canvases server-side filter, but be paranoid.
		if f.Filetype != "" && f.Filetype != "canvas" && f.Filetype != "quip" {
			continue
		}
		info := CanvasInfo{
			ID:          f.ID,
			Title:       f.Title,
			OwnerID:     f.User,
			URL:         f.Permalink,
			DateCreated: f.Created,
			DateUpdated: f.Updated,
		}
		if info.Title == "" {
			info.Title = f.Name
		}
		if len(f.Channels) > 0 {
			info.ChannelID = f.Channels[0]
		}
		resp.Canvases = append(resp.Canvases, info)
	}
	return resp, nil
}

// === Get ===

type CanvasGetResponse struct {
	SlackResponse
	Canvas CanvasInfo      `json:"canvas"`
	Markdown string        `json:"markdown,omitempty"` // when format=markdown
	Raw    json.RawMessage `json:"-"`
}

// CanvasesGet fetches a single canvas via files.info. canvasID must be an
// F-prefix file ID (e.g. F0ASWF3SRST). format param is accepted for API
// compatibility but ignored — the underlying files.info returns the file
// metadata only; the document content lives at url_private (auth-gated).
//
// Underlying Slack method: files.info?file=<F-id>. canvases.get does not
// exist on Slack public API (confirmed empty at
// https://api.slack.com/methods?filter=canvases).
func (c *Client) CanvasesGet(canvasID, format string) (*CanvasGetResponse, error) {
	if canvasID == "" {
		return nil, fmt.Errorf("canvas ID required (F-prefix file ID, e.g. F0ASWF3SRST)")
	}
	if canvasID[0] != 'F' {
		return nil, fmt.Errorf("canvas ID must be F-prefix file ID (got %q); use 'slackogo canvas list' to find IDs", canvasID)
	}
	_ = format // accepted for back-compat; files.info returns metadata regardless
	params := url.Values{}
	params.Set("file", canvasID)
	data, err := c.Post("files.info", params)
	if err != nil {
		return nil, err
	}
	var wire struct {
		SlackResponse
		File struct {
			ID         string   `json:"id"`
			Title      string   `json:"title"`
			Name       string   `json:"name"`
			Filetype   string   `json:"filetype"`
			Mimetype   string   `json:"mimetype"`
			User       string   `json:"user"`
			Channels   []string `json:"channels"`
			Permalink  string   `json:"permalink"`
			URLPrivate string   `json:"url_private"`
			Created    int64    `json:"created"`
			Updated    int64    `json:"updated"`
		} `json:"file"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, fmt.Errorf("decode files.info: %w", err)
	}
	resp := &CanvasGetResponse{
		SlackResponse: wire.SlackResponse,
		Canvas: CanvasInfo{
			ID:          wire.File.ID,
			Title:       wire.File.Title,
			OwnerID:     wire.File.User,
			URL:         wire.File.Permalink,
			DateCreated: wire.File.Created,
			DateUpdated: wire.File.Updated,
		},
		Raw: data,
	}
	if resp.Canvas.Title == "" {
		resp.Canvas.Title = wire.File.Name
	}
	if len(wire.File.Channels) > 0 {
		resp.Canvas.ChannelID = wire.File.Channels[0]
	}
	return resp, nil
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
