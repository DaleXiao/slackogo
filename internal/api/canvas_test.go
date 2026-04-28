package api

// Unit tests for canvas API wrappers (SPEC-050 v5).
// Covers: argument validation, request body construction, mocked response decoding.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DaleXiao/slackogo/internal/auth"
)

// --- helpers ---

// newMockClient routes every Slack API request to the given handler.
func newMockClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := &Client{
		HTTPClient: srv.Client(),
		Creds: &auth.Credentials{
			Workspace: "test",
			Token:     "xoxc-test",
			Cookie:    "d-cookie",
		},
		BaseURL: srv.URL + "/",
	}
	// httptest.Server uses a 0-timeout client, give it a sane one.
	c.HTTPClient.Timeout = 5 * time.Second
	return c, srv
}

func parseForm(t *testing.T, r *http.Request) url.Values {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	v, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatalf("parse form: %v", err)
	}
	return v
}

// --- list ---

func TestCanvasesList_RequestParams(t *testing.T) {
	var captured url.Values
	var gotPath string
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		captured = parseForm(t, r)
		_, _ = w.Write([]byte(`{"ok":true,"files":[{"id":"F123","title":"hi","filetype":"canvas","channels":["C42"],"created":1700000000}]}`))
	})
	defer srv.Close()

	resp, err := c.CanvasesList("C42", 25)
	if err != nil {
		t.Fatalf("CanvasesList err: %v", err)
	}
	if gotPath != "/files.list" {
		t.Errorf("wire method path = %q, want /files.list", gotPath)
	}
	if got := captured.Get("types"); got != "canvases" {
		t.Errorf("types = %q, want canvases", got)
	}
	if got := captured.Get("channel"); got != "C42" {
		t.Errorf("channel = %q, want C42", got)
	}
	if got := captured.Get("count"); got != "25" {
		t.Errorf("count = %q, want 25", got)
	}
	if got := captured.Get("token"); got != "xoxc-test" {
		t.Errorf("token = %q, want xoxc-test", got)
	}
	if len(resp.Canvases) != 1 || resp.Canvases[0].ID != "F123" || resp.Canvases[0].ChannelID != "C42" {
		t.Errorf("canvases = %+v", resp.Canvases)
	}
}

func TestCanvasesList_OmitsEmptyChannel(t *testing.T) {
	var captured url.Values
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		captured = parseForm(t, r)
		_, _ = w.Write([]byte(`{"ok":true,"files":[]}`))
	})
	defer srv.Close()

	if _, err := c.CanvasesList("", 0); err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, ok := captured["channel"]; ok {
		t.Errorf("channel should be omitted when empty")
	}
	if _, ok := captured["count"]; ok {
		t.Errorf("count should be omitted when zero")
	}
	if captured.Get("types") != "canvases" {
		t.Errorf("types should always be canvases")
	}
}

// --- get ---

func TestCanvasesGet_UsesFilesInfo(t *testing.T) {
	var captured url.Values
	var gotPath string
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		captured = parseForm(t, r)
		_, _ = w.Write([]byte(`{"ok":true,"file":{"id":"F1","title":"hi","filetype":"canvas","mimetype":"application/vnd.slack-docs","user":"U1","channels":["C9"],"permalink":"https://slack.example/x","created":1700000000,"updated":1700000100}}`))
	})
	defer srv.Close()

	resp, err := c.CanvasesGet("F1", "markdown")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if gotPath != "/files.info" {
		t.Errorf("wire method = %q, want /files.info", gotPath)
	}
	if captured.Get("file") != "F1" {
		t.Errorf("file param = %q, want F1", captured.Get("file"))
	}
	if resp.Canvas.ID != "F1" || resp.Canvas.Title != "hi" || resp.Canvas.ChannelID != "C9" {
		t.Errorf("canvas = %+v", resp.Canvas)
	}
	if len(resp.Raw) == 0 {
		t.Errorf("raw not captured")
	}
}

func TestCanvasesGet_RejectsNonFPrefixID(t *testing.T) {
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be called for invalid ID")
	})
	defer srv.Close()

	if _, err := c.CanvasesGet("Q123", ""); err == nil {
		t.Errorf("expected error for Q-prefix ID, got nil")
	}
	if _, err := c.CanvasesGet("", ""); err == nil {
		t.Errorf("expected error for empty ID, got nil")
	}
}

// --- create ---

func TestCanvasesCreate_StandaloneVsChannel(t *testing.T) {
	cases := []struct {
		name      string
		channelID string
		wantPath  string
	}{
		{"standalone", "", "/canvases.create"},
		{"channel", "C99", "/conversations.canvases.create"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			var captured url.Values
			c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				captured = parseForm(t, r)
				_, _ = w.Write([]byte(`{"ok":true,"canvas_id":"F-NEW"}`))
			})
			defer srv.Close()

			resp, err := c.CanvasesCreate("Hello", "# body", tc.channelID)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if gotPath != tc.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tc.wantPath)
			}
			if captured.Get("title") != "Hello" {
				t.Errorf("title = %q", captured.Get("title"))
			}
			var doc CanvasDocumentContent
			if err := json.Unmarshal([]byte(captured.Get("document_content")), &doc); err != nil {
				t.Fatalf("document_content not valid json: %v", err)
			}
			if doc.Type != "markdown" || doc.Markdown != "# body" {
				t.Errorf("document_content = %+v", doc)
			}
			if tc.channelID != "" && captured.Get("channel_id") != tc.channelID {
				t.Errorf("channel_id = %q", captured.Get("channel_id"))
			}
			if resp.CanvasID != "F-NEW" {
				t.Errorf("canvas_id = %q", resp.CanvasID)
			}
		})
	}
}

// --- edit ---

func TestCanvasesEdit_BatchEncoding(t *testing.T) {
	var captured url.Values
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		captured = parseForm(t, r)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	defer srv.Close()

	changes := []CanvasChange{
		{Operation: "replace", SectionID: "s1", DocumentContent: &CanvasDocumentContent{Type: "markdown", Markdown: "new"}},
		{Operation: "delete", SectionID: "s2"},
	}
	if err := c.CanvasesEdit("F1", changes); err != nil {
		t.Fatalf("err: %v", err)
	}
	if captured.Get("canvas_id") != "F1" {
		t.Errorf("canvas_id = %q", captured.Get("canvas_id"))
	}
	var got []CanvasChange
	if err := json.Unmarshal([]byte(captured.Get("changes")), &got); err != nil {
		t.Fatalf("changes not valid json: %v", err)
	}
	if len(got) != 2 || got[0].Operation != "replace" || got[1].Operation != "delete" {
		t.Errorf("changes = %+v", got)
	}
}

func TestCanvasesEdit_RejectsEmpty(t *testing.T) {
	c := &Client{} // no HTTP needed; should fail before calling
	if err := c.CanvasesEdit("F1", nil); err == nil {
		t.Errorf("expected error on empty changes")
	}
}

// --- delete ---

func TestCanvasesDelete(t *testing.T) {
	var captured url.Values
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		captured = parseForm(t, r)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	defer srv.Close()

	if err := c.CanvasesDelete("F1"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if captured.Get("canvas_id") != "F1" {
		t.Errorf("canvas_id = %q", captured.Get("canvas_id"))
	}
}

// --- access ---

func TestCanvasesAccessSet_Validation(t *testing.T) {
	c := &Client{}
	if err := c.CanvasesAccessSet("F1", nil, "read"); err == nil {
		t.Errorf("expected error on empty users")
	}
	if err := c.CanvasesAccessSet("F1", []string{"U1"}, "admin"); err == nil {
		t.Errorf("expected error on invalid level")
	}
}

func TestCanvasesAccessSet_RequestBody(t *testing.T) {
	var captured url.Values
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		captured = parseForm(t, r)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	defer srv.Close()

	if err := c.CanvasesAccessSet("F1", []string{"U1", "U2"}, "write"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if captured.Get("canvas_id") != "F1" {
		t.Errorf("canvas_id = %q", captured.Get("canvas_id"))
	}
	if captured.Get("access_level") != "write" {
		t.Errorf("access_level = %q", captured.Get("access_level"))
	}
	var ids []string
	if err := json.Unmarshal([]byte(captured.Get("user_ids")), &ids); err != nil {
		t.Fatalf("user_ids not valid json: %v", err)
	}
	if strings.Join(ids, ",") != "U1,U2" {
		t.Errorf("user_ids = %v", ids)
	}
}

func TestCanvasesAccessDelete_Validation(t *testing.T) {
	c := &Client{}
	if err := c.CanvasesAccessDelete("F1", nil); err == nil {
		t.Errorf("expected error on empty users")
	}
}

// --- error surfacing ---

func TestSlackError_NotInChannel(t *testing.T) {
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"error":"not_in_channel"}`))
	})
	defer srv.Close()

	_, err := c.CanvasesCreate("Hi", "body", "C1")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "not_in_channel") {
		t.Errorf("error = %v, want not_in_channel surfaced", err)
	}
}
