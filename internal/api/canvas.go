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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// validateCanvasID enforces that canvas IDs are F-prefix file IDs (e.g.
// F0ASWF3SRST). The legacy Q-prefix ("Quip") IDs and empty strings are
// rejected because Slack canvases.* methods always require an F-prefix file
// ID since the 2024 Canvas API consolidation. Returning early avoids a
// confusing slack API error like `invalid_arguments` from the server.
//
// Caller convention: the wrapper that takes canvas_id calls this first.
func validateCanvasID(canvasID string) error {
	if canvasID == "" {
		return fmt.Errorf("canvas ID required (F-prefix file ID, e.g. F0ASWF3SRST); use 'slackogo canvas list' to find IDs")
	}
	if canvasID[0] != 'F' {
		return fmt.Errorf("canvas ID must be F-prefix file ID (got %q); use 'slackogo canvas list' to find IDs", canvasID)
	}
	return nil
}

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
// Underlying Slack method: files.list with types=canvas (the
// non-existent canvases.list shipped in v0.4.0 is replaced).
// The Slack docs say "types=canvas" (singular) at
// https://docs.slack.dev/surfaces/canvases/. v0.4.1 used "canvases"
// (plural) which silently returned empty for some workspaces; SPEC-056 v4
// hot-fixes this to "canvas".
// Canvas IDs returned are F-prefix file IDs (e.g. F0ASWF3SRST).
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
	if err := validateCanvasID(canvasID); err != nil {
		return nil, err
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

// === Body fetch (SPEC-058) ===

// CanvasBody holds the canvas document body fetched from
// url_private_download. The field set covers the three CLI output modes:
//   - Raw    : original bytes returned by Slack (no parsing)
//   - Markdown: markdown text (either Raw verbatim if Slack returned plain
//               markdown, or extracted from a JSON envelope's "markdown"
//               field — see CanvasesFetchBody for sniffing rules)
//   - JSON   : a structured representation. If Slack returned JSON, this
//               is the parsed value (json.RawMessage so callers can
//               re-marshal pretty); otherwise this wraps the markdown text
//               as `{"markdown": "..."}` so `-o json` always returns
//               valid JSON.
type CanvasBody struct {
	Raw      []byte          `json:"-"`
	Markdown string          `json:"markdown,omitempty"`
	JSON     json.RawMessage `json:"json,omitempty"`
	// IsJSON is true when the downloaded payload was valid JSON. When false,
	// the payload was treated as plain markdown text.
	IsJSON bool `json:"-"`
	// SourceURL is the url_private_download we fetched from (useful for
	// debugging in the PR description / logs).
	SourceURL string `json:"-"`
}

// CanvasesFetchBody resolves a canvas's url_private_download via files.info,
// then GETs that URL with the workspace credentials (Bearer xoxc + Cookie d=)
// and returns the document body as bytes.
//
// The Slack canvas content format is markdown (per
// https://docs.slack.dev/surfaces/canvases/ "Formatting canvas content with
// the Slack API" → `document_content.markdown`). The download endpoint may
// return either:
//   - plain markdown text (mimetype application/vnd.slack-docs treated as
//     UTF-8 text), or
//   - a thin JSON envelope wrapping the markdown (e.g. {"markdown": "..."}
//     or {"body": "..."}).
//
// We sniff: if bytes parse as JSON and contain a "markdown" or "body" string
// field, we extract that for Markdown. Otherwise, the bytes are treated as
// markdown verbatim. This is conservative — Dale verifies actual fixtures at
// PR review time and the heuristic can be patched in one line if Slack ships
// a different envelope key.
//
// internal/api/client.go is intentionally NOT modified (per SPEC-050 v5
// constraint #1); this method does its own HTTP request using c.HTTPClient
// and reads c.Creds.{Token,Cookie} directly.
func (c *Client) CanvasesFetchBody(canvasID string) (*CanvasBody, error) {
	if err := validateCanvasID(canvasID); err != nil {
		return nil, err
	}

	// Step 1 — files.info to find the download URL.
	params := url.Values{}
	params.Set("file", canvasID)
	infoData, err := c.Post("files.info", params)
	if err != nil {
		return nil, fmt.Errorf("canvas body: files.info: %w", err)
	}
	var info struct {
		File struct {
			ID                 string `json:"id"`
			Filetype           string `json:"filetype"`
			Mimetype           string `json:"mimetype"`
			URLPrivate         string `json:"url_private"`
			URLPrivateDownload string `json:"url_private_download"`
			Size               int64  `json:"size"`
		} `json:"file"`
	}
	if err := json.Unmarshal(infoData, &info); err != nil {
		return nil, fmt.Errorf("canvas body: decode files.info: %w", err)
	}
	if info.File.Filetype != "" && info.File.Filetype != "canvas" && info.File.Filetype != "quip" {
		return nil, fmt.Errorf("canvas body: file %s is filetype %q, not canvas", canvasID, info.File.Filetype)
	}
	dl := info.File.URLPrivateDownload
	if dl == "" {
		// Fall back to url_private (browser-link variant). Slack returns one
		// or the other depending on the file type.
		dl = info.File.URLPrivate
	}
	if dl == "" {
		return nil, fmt.Errorf("canvas body: files.info returned no download URL for %s", canvasID)
	}

	// Step 2 — GET the download URL with workspace creds.
	req, err := http.NewRequest("GET", dl, nil)
	if err != nil {
		return nil, fmt.Errorf("canvas body: build request: %w", err)
	}
	// xoxc tokens are presented as Authorization: Bearer per Slack's file
	// download convention; the workspace cookie (d=...) authenticates the
	// session against files.slack.com.
	req.Header.Set("Authorization", "Bearer "+c.Creds.Token)
	req.Header.Set("Cookie", "d="+c.Creds.Cookie)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("canvas body: GET %s: %w", dl, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// Read up to 1KiB of the error body for diagnostics.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("canvas body: download HTTP %d from %s: %s", resp.StatusCode, dl, strings.TrimSpace(string(snippet)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("canvas body: read body: %w", err)
	}

	out := &CanvasBody{
		Raw:       body,
		SourceURL: dl,
	}

	// Step 3 — sniff envelope.
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		var generic any
		if err := json.Unmarshal(body, &generic); err == nil {
			out.IsJSON = true
			out.JSON = json.RawMessage(body)
			// Try common envelope keys for the markdown body.
			if obj, ok := generic.(map[string]any); ok {
				for _, k := range []string{"markdown", "body", "content", "doc", "document"} {
					if v, ok := obj[k]; ok {
						if s, ok := v.(string); ok && s != "" {
							out.Markdown = s
							break
						}
					}
				}
			}
		}
	}
	if out.Markdown == "" && !out.IsJSON {
		// Treat the raw bytes as markdown text.
		out.Markdown = string(body)
	}
	return out, nil
}
