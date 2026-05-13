package api

import (
	"net/http"
	"net/url"
	"testing"
)

func TestConversationsList_PaginatesWithCursor(t *testing.T) {
	var requests []url.Values
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		form := parseForm(t, r)
		requests = append(requests, form)

		switch len(requests) {
		case 1:
			if got := form.Get("limit"); got != "3" {
				t.Errorf("first page limit = %q, want 3", got)
			}
			if got := form.Get("cursor"); got != "" {
				t.Errorf("first page cursor = %q, want empty", got)
			}
			_, _ = w.Write([]byte(`{"ok":true,"channels":[{"id":"C1","name":"alpha"}],"response_metadata":{"next_cursor":"cursor-2"}}`))
		case 2:
			if got := form.Get("limit"); got != "2" {
				t.Errorf("second page limit = %q, want 2", got)
			}
			if got := form.Get("cursor"); got != "cursor-2" {
				t.Errorf("second page cursor = %q, want cursor-2", got)
			}
			_, _ = w.Write([]byte(`{"ok":true,"channels":[{"id":"C2","name":"beta"},{"id":"C3","name":"gamma"}],"response_metadata":{"next_cursor":""}}`))
		default:
			t.Fatalf("unexpected extra conversations.list request")
		}
	})
	defer srv.Close()

	resp, err := c.ConversationsList("public_channel,private_channel", 3)
	if err != nil {
		t.Fatalf("ConversationsList err: %v", err)
	}
	if len(resp.Channels) != 3 {
		t.Fatalf("len(channels) = %d, want 3", len(resp.Channels))
	}
	if resp.Channels[2].ID != "C3" {
		t.Errorf("last channel = %+v, want C3", resp.Channels[2])
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
}

func TestConversationsList_StopsAtLimit(t *testing.T) {
	requests := 0
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests > 1 {
			t.Fatalf("unexpected extra conversations.list request")
		}
		_, _ = w.Write([]byte(`{"ok":true,"channels":[{"id":"C1","name":"alpha"},{"id":"C2","name":"beta"}],"response_metadata":{"next_cursor":"cursor-2"}}`))
	})
	defer srv.Close()

	resp, err := c.ConversationsList("public_channel", 2)
	if err != nil {
		t.Fatalf("ConversationsList err: %v", err)
	}
	if len(resp.Channels) != 2 {
		t.Fatalf("len(channels) = %d, want 2", len(resp.Channels))
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestResolveChannelID_PaginatesUntilMatch(t *testing.T) {
	requests := 0
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		form := parseForm(t, r)
		requests++

		switch requests {
		case 1:
			if got := form.Get("limit"); got != "1000" {
				t.Errorf("resolve first page limit = %q, want 1000", got)
			}
			_, _ = w.Write([]byte(`{"ok":true,"channels":[{"id":"C1","name":"alpha"}],"response_metadata":{"next_cursor":"cursor-2"}}`))
		case 2:
			if got := form.Get("cursor"); got != "cursor-2" {
				t.Errorf("resolve second page cursor = %q, want cursor-2", got)
			}
			_, _ = w.Write([]byte(`{"ok":true,"channels":[{"id":"C2","name":"target"}],"response_metadata":{"next_cursor":"cursor-3"}}`))
		default:
			t.Fatalf("unexpected extra conversations.list request")
		}
	})
	defer srv.Close()

	id, err := c.ResolveChannelID("#target")
	if err != nil {
		t.Fatalf("ResolveChannelID err: %v", err)
	}
	if id != "C2" {
		t.Fatalf("id = %q, want C2", id)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestUsersList_PaginatesWithCursor(t *testing.T) {
	var requests []url.Values
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		form := parseForm(t, r)
		requests = append(requests, form)

		switch len(requests) {
		case 1:
			if got := form.Get("limit"); got != "3" {
				t.Errorf("first page limit = %q, want 3", got)
			}
			_, _ = w.Write([]byte(`{"ok":true,"members":[{"id":"U1","name":"alpha"}],"response_metadata":{"next_cursor":"cursor-2"}}`))
		case 2:
			if got := form.Get("limit"); got != "2" {
				t.Errorf("second page limit = %q, want 2", got)
			}
			if got := form.Get("cursor"); got != "cursor-2" {
				t.Errorf("second page cursor = %q, want cursor-2", got)
			}
			_, _ = w.Write([]byte(`{"ok":true,"members":[{"id":"U2","name":"beta"},{"id":"U3","name":"gamma"}],"response_metadata":{"next_cursor":""}}`))
		default:
			t.Fatalf("unexpected extra users.list request")
		}
	})
	defer srv.Close()

	resp, err := c.UsersList(3)
	if err != nil {
		t.Fatalf("UsersList err: %v", err)
	}
	if len(resp.Members) != 3 {
		t.Fatalf("len(members) = %d, want 3", len(resp.Members))
	}
	if resp.Members[2].ID != "U3" {
		t.Errorf("last user = %+v, want U3", resp.Members[2])
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
}

func TestResolveUserID_PaginatesUntilDisplayNameMatch(t *testing.T) {
	requests := 0
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		form := parseForm(t, r)
		requests++

		switch requests {
		case 1:
			if got := form.Get("limit"); got != "200" {
				t.Errorf("resolve first page limit = %q, want 200", got)
			}
			_, _ = w.Write([]byte(`{"ok":true,"members":[{"id":"U1","name":"alpha","profile":{"display_name":"Alpha"}}],"response_metadata":{"next_cursor":"cursor-2"}}`))
		case 2:
			if got := form.Get("cursor"); got != "cursor-2" {
				t.Errorf("resolve second page cursor = %q, want cursor-2", got)
			}
			_, _ = w.Write([]byte(`{"ok":true,"members":[{"id":"U2","name":"beta","profile":{"display_name":"Target User"}}],"response_metadata":{"next_cursor":"cursor-3"}}`))
		default:
			t.Fatalf("unexpected extra users.list request")
		}
	})
	defer srv.Close()

	id, err := c.ResolveUserID("@Target User")
	if err != nil {
		t.Fatalf("ResolveUserID err: %v", err)
	}
	if id != "U2" {
		t.Fatalf("id = %q, want U2", id)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestCanvasesList_PaginatesFilesList(t *testing.T) {
	var requests []url.Values
	c, srv := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		form := parseForm(t, r)
		requests = append(requests, form)

		switch len(requests) {
		case 1:
			if got := form.Get("count"); got != "2" {
				t.Errorf("first page count = %q, want 2", got)
			}
			if got := form.Get("page"); got != "" {
				t.Errorf("first page page param = %q, want empty", got)
			}
			_, _ = w.Write([]byte(`{"ok":true,"files":[{"id":"F1","title":"one","filetype":"canvas","channels":["C1"]}],"paging":{"page":1,"pages":2}}`))
		case 2:
			if got := form.Get("count"); got != "1" {
				t.Errorf("second page count = %q, want 1", got)
			}
			if got := form.Get("page"); got != "2" {
				t.Errorf("second page page param = %q, want 2", got)
			}
			_, _ = w.Write([]byte(`{"ok":true,"files":[{"id":"F2","title":"two","filetype":"canvas","channels":["C2"]}],"paging":{"page":2,"pages":2}}`))
		default:
			t.Fatalf("unexpected extra files.list request")
		}
	})
	defer srv.Close()

	resp, err := c.CanvasesList("", 2)
	if err != nil {
		t.Fatalf("CanvasesList err: %v", err)
	}
	if len(resp.Canvases) != 2 {
		t.Fatalf("len(canvases) = %d, want 2", len(resp.Canvases))
	}
	if resp.Canvases[1].ID != "F2" {
		t.Errorf("second canvas = %+v, want F2", resp.Canvases[1])
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
}
