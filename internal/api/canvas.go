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
	Canvases []CanvasInfo `json:"canvases"`
	Paging   struct {
		Count int `json:"count"`
		Total int `json:"total"`
		Page  int `json:"page"`
		Pages int `json:"pages"`
	} `json:"paging"`
}

// filesListEntry mirrors the relevant fields of the files.list `files[]` array.
// Slack returns a richer shape for files; we map the canvas-relevant fields
// into CanvasInfo for the public surface.
type filesListEntry struct {
	ID         string   `json:"id"`
	Filetype   string   `json:"filetype"`
	Title      string   `json:"title"`
	Name       string   `json:"name"`
	User       string   `json:"user"`
	Channels   []string `json:"channels"`
	Permalink  string   `json:"permalink"`
	URLPrivate string   `json:"url_private"`
	Created    int64    `json:"created"`
	Updated    int64    `json:"updated"`
	Timestamp  int64    `json:"timestamp"`
}

type filesListResponse struct {
	SlackResponse
	Files  []filesListEntry `json:"files"`
	Paging struct {
		Count int `json:"count"`
		Total int `json:"total"`
		Page  int `json:"page"`
		Pages int `json:"pages"`
	} `json:"paging"`
}

// CanvasesList lists canvases visible to the caller via Slack's `files.list`
// endpoint with `types=canvas`. Slack's `canvases.list` method does not
// exist; per https://docs.slack.dev/surfaces/canvases/ canvases are surfaced
// as files of type `canvas`.
//
// channelID is optional; when set, results are scoped to that channel
// (server-side filter via `channel`).
func (c *Client) CanvasesList(channelID string, limit int) (*CanvasesListResponse, error) {
	params := url.Values{}
	params.Set("types", "canvas")
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
	var raw filesListResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode files.list: %w", err)
	}

	resp := &CanvasesListResponse{SlackResponse: raw.SlackResponse, Paging: raw.Paging}
	for _, f := range raw.Files {
		// Defensive: server-side `types=canvas` should already filter, but
		// double-check filetype to avoid surprises across API versions.
		if f.Filetype != "" && f.Filetype != "canvas" {
			continue
		}
		title := f.Title
		if title == "" {
			title = f.Name
		}
		u := f.Permalink
		if u == "" {
			u = f.URLPrivate
		}
		var ch string
		if len(f.Channels) > 0 {
			ch = f.Channels[0]
		}
		resp.Canvases = append(resp.Canvases, CanvasInfo{
			ID:          f.ID,
			Title:       title,
			OwnerID:     f.User,
			ChannelID:   ch,
			URL:         u,
			DateCreated: f.Created,
			DateUpdated: f.Updated,
		})
	}
	return resp, nil
}

// === Get ===

type CanvasGetResponse struct {
	SlackResponse
	Canvas   CanvasInfo      `json:"canvas"`
	Markdown string          `json:"markdown,omitempty"` // when format=markdown
	Raw      json.RawMessage `json:"-"`
}

// filesInfoResponse mirrors the relevant subset of files.info.
type filesInfoResponse struct {
	SlackResponse
	File struct {
		ID                string   `json:"id"`
		Filetype          string   `json:"filetype"`
		Mimetype          string   `json:"mimetype"`
		Title             string   `json:"title"`
		Name              string   `json:"name"`
		User              string   `json:"user"`
		Channels          []string `json:"channels"`
		Permalink         string   `json:"permalink"`
		URLPrivate        string   `json:"url_private"`
		URLPrivateDownload string  `json:"url_private_download"`
		Created           int64    `json:"created"`
		Updated           int64    `json:"updated"`
	} `json:"file"`
}

// CanvasesGet fetches a single canvas via Slack's `files.info` endpoint.
//
// Slack has no `canvases.get` method (verified via
// https://docs.slack.dev/reference/methods/canvases.create sidebar — only
// canvases.create/edit/delete/sections.lookup/access.set/access.delete are
// listed). Canvases are surfaced as files of mimetype
// `application/vnd.slack-docs`; per https://docs.slack.dev/surfaces/canvases/
// they are retrieved through the standard files.info endpoint.
//
// canvasID must be an F-prefix file ID (e.g. F0ASWF3SRST). The legacy
// Q-prefix IDs returned by old preview surfaces are not valid here.
//
// format is accepted for CLI compatibility (markdown|json|raw) but does not
// alter the wire request — Slack returns the canvas content in the file
// object's `url_private_download` for follow-up download.
func (c *Client) CanvasesGet(canvasID, format string) (*CanvasGetResponse, error) {
	if err := validateCanvasID(canvasID); err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("file", canvasID)
	data, err := c.Post("files.info", params)
	if err != nil {
		return nil, err
	}
	var raw filesInfoResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode files.info: %w", err)
	}
	// Defensive mimetype check — Slack canvases have mimetype
	// application/vnd.slack-docs. Filetype is `canvas`.
	if raw.File.Filetype != "" && raw.File.Filetype != "canvas" {
		return nil, fmt.Errorf("file %s is not a canvas (filetype=%s, mimetype=%s)", canvasID, raw.File.Filetype, raw.File.Mimetype)
	}
	resp := &CanvasGetResponse{SlackResponse: raw.SlackResponse, Raw: data}
	title := raw.File.Title
	if title == "" {
		title = raw.File.Name
	}
	u := raw.File.Permalink
	if u == "" {
		u = raw.File.URLPrivate
	}
	var ch string
	if len(raw.File.Channels) > 0 {
		ch = raw.File.Channels[0]
	}
	resp.Canvas = CanvasInfo{
		ID:          raw.File.ID,
		Title:       title,
		OwnerID:     raw.File.User,
		ChannelID:   ch,
		URL:         u,
		DateCreated: raw.File.Created,
		DateUpdated: raw.File.Updated,
	}
	_ = format // CLI surface compatibility; format is honored at the print layer.
	return resp, nil
}

// validateCanvasID returns an error if id is not a Slack F-prefix file ID.
// Canvases use the standard file ID format (F-prefix); Q-prefix IDs from
// legacy preview surfaces are explicitly rejected per SPEC-056 v2.
func validateCanvasID(id string) error {
	if id == "" {
		return fmt.Errorf("canvas id is required")
	}
	if id[0] != 'F' {
		return fmt.Errorf("canvas id %q is not an F-prefix file ID; canvases are files (e.g. F0ASWF3SRST), Q-prefix IDs from legacy surfaces are not valid", id)
	}
	return nil
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
	if err := validateCanvasID(canvasID); err != nil {
		return err
	}
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
	if err := validateCanvasID(canvasID); err != nil {
		return err
	}
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
	if err := validateCanvasID(canvasID); err != nil {
		return nil, err
	}
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
	if err := validateCanvasID(canvasID); err != nil {
		return err
	}
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
	if err := validateCanvasID(canvasID); err != nil {
		return err
	}
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
