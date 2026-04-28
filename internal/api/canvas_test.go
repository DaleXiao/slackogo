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

// --- list (SPEC-056: files.list?types=canvas) ---

func TestCanvasesList_RequestParams(t *testing.T) {
	var captured url.Values
	var gotPath string
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		captured = parseForm(t, r)
		_, _ = w.Write([]byte(`{"ok":true,"files":[{"id":"F123","filetype":"canvas","title":"hi","permalink":"https://x.slack.com/docs/F123","channels":["C42"],"user":"U1","created":1700000000,"updated":1700000100}],"paging":{"count":1,"total":1,"page":1,"pages":1}}`))
	})
	defer srv.Close()

	resp, err := c.CanvasesList("C42", 25)
	if err != nil {
		t.Fatalf("CanvasesList err: %v", err)
	}
	// SPEC-056 ac #1: must call files.list with types=canvas
	if gotPath != "/files.list" {
		t.Errorf("path = %q, want /files.list", gotPath)
	}
	if got := captured.Get("types"); got != "canvas" {
		t.Errorf("types = %q, want canvas", got)
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
	// SPEC-056 ac #2: response decoded from `files` array, mapped into CanvasInfo
	if len(resp.Canvases) != 1 {
		t.Fatalf("canvases = %+v", resp.Canvases)
	}
	cv := resp.Canvases[0]
	if cv.ID != "F123" || cv.Title != "hi" || cv.OwnerID != "U1" || cv.ChannelID != "C42" {
		t.Errorf("canvas mapping wrong: %+v", cv)
	}
	if cv.URL != "https://x.slack.com/docs/F123" {
		t.Errorf("URL = %q (want permalink)", cv.URL)
	}
	if cv.DateCreated != 1700000000 || cv.DateUpdated != 1700000100 {
		t.Errorf("timestamps = %d/%d", cv.DateCreated, cv.DateUpdated)
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
	// types=canvas always present
	if captured.Get("types") != "canvas" {
		t.Errorf("types must always be canvas, got %q", captured.Get("types"))
	}
}

func TestCanvasesList_FiletypeFilter(t *testing.T) {
	// Defensive client-side filetype filter: drop entries with filetype != canvas
	// (in case server-side types= returns mixed results across API versions).
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"files":[
			{"id":"F1","filetype":"canvas","title":"keep"},
			{"id":"F2","filetype":"png","title":"drop"},
			{"id":"F3","filetype":"","title":"keep_empty"},
			{"id":"F4","filetype":"quip","title":"drop"}
		]}`))
	})
	defer srv.Close()

	resp, err := c.CanvasesList("", 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(resp.Canvases) != 2 {
		t.Fatalf("want 2 canvases (F1+F3), got %d: %+v", len(resp.Canvases), resp.Canvases)
	}
	ids := []string{resp.Canvases[0].ID, resp.Canvases[1].ID}
	if strings.Join(ids, ",") != "F1,F3" {
		t.Errorf("ids = %v, want [F1 F3]", ids)
	}
}

func TestCanvasesList_TitleFallbackToName(t *testing.T) {
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"files":[{"id":"F1","filetype":"canvas","title":"","name":"fallback-name","url_private":"https://x/private"}]}`))
	})
	defer srv.Close()

	resp, err := c.CanvasesList("", 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Canvases[0].Title != "fallback-name" {
		t.Errorf("title fallback failed: %q", resp.Canvases[0].Title)
	}
	if resp.Canvases[0].URL != "https://x/private" {
		t.Errorf("url fallback to url_private failed: %q", resp.Canvases[0].URL)
	}
}

// --- get (SPEC-056 v2: files.info, F-prefix only) ---

func TestCanvasesGet_UsesFilesInfo(t *testing.T) {
	var captured url.Values
	var gotPath string
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		captured = parseForm(t, r)
		_, _ = w.Write([]byte(`{"ok":true,"file":{"id":"F0ASWF3SRST","filetype":"canvas","mimetype":"application/vnd.slack-docs","title":"hi","user":"U1","channels":["C42"],"permalink":"https://x/docs/F0ASWF3SRST","created":1700000000,"updated":1700000100}}`))
	})
	defer srv.Close()

	resp, err := c.CanvasesGet("F0ASWF3SRST", "markdown")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// SPEC-056 v2: must call files.info, not canvases.get
	if gotPath != "/files.info" {
		t.Errorf("path = %q, want /files.info", gotPath)
	}
	if captured.Get("file") != "F0ASWF3SRST" {
		t.Errorf("file param = %q", captured.Get("file"))
	}
	// canvas_id is the legacy param; must NOT be sent
	if _, ok := captured["canvas_id"]; ok {
		t.Errorf("legacy canvas_id param leaked: %v", captured)
	}
	if resp.Canvas.ID != "F0ASWF3SRST" || resp.Canvas.Title != "hi" {
		t.Errorf("canvas mapping = %+v", resp.Canvas)
	}
	if len(resp.Raw) == 0 {
		t.Errorf("raw not captured")
	}
}

func TestCanvasesGet_RejectsQPrefix(t *testing.T) {
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be called for invalid id")
	})
	defer srv.Close()

	_, err := c.CanvasesGet("Q123ABC", "")
	if err == nil {
		t.Fatal("expected validation error for Q-prefix id")
	}
	if !strings.Contains(err.Error(), "F-prefix") {
		t.Errorf("error should mention F-prefix: %v", err)
	}
}

func TestCanvasesGet_RejectsEmptyID(t *testing.T) {
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be called")
	})
	defer srv.Close()
	if _, err := c.CanvasesGet("", ""); err == nil {
		t.Error("expected validation error")
	}
}

func TestCanvasesGet_RejectsNonCanvasFiletype(t *testing.T) {
	// files.info on a non-canvas F-id (e.g. an image) should be rejected
	// defensively even though Slack returns ok=true.
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"file":{"id":"F999","filetype":"png","mimetype":"image/png","title":"img"}}`))
	})
	defer srv.Close()
	_, err := c.CanvasesGet("F999", "")
	if err == nil {
		t.Fatal("expected non-canvas filetype error")
	}
	if !strings.Contains(err.Error(), "not a canvas") {
		t.Errorf("error wording: %v", err)
	}
}

// --- F-prefix validation propagation across wrappers ---

func TestValidateCanvasID_PropagatesAcrossWrappers(t *testing.T) {
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server must not be called for invalid id, path=%s", r.URL.Path)
	})
	defer srv.Close()

	cases := []struct {
		name string
		call func() error
	}{
		{"edit", func() error { return c.CanvasesEdit("Q1", []CanvasChange{{Operation: "insert_at_end"}}) }},
		{"delete", func() error { return c.CanvasesDelete("Q1") }},
		{"sections.lookup", func() error { _, e := c.CanvasesSectionsLookup("Q1"); return e }},
		{"access.set", func() error { return c.CanvasesAccessSet("Q1", []string{"U1"}, "read") }},
		{"access.delete", func() error { return c.CanvasesAccessDelete("Q1", []string{"U1"}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal("expected validation error for Q-prefix id")
			}
			if !strings.Contains(err.Error(), "F-prefix") {
				t.Errorf("error wording: %v", err)
			}
		})
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
