package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DaleXiao/slackogo/internal/auth"
	"github.com/DaleXiao/slackogo/internal/transport"
)

type Client struct {
	HTTPClient *http.Client
	Creds      *auth.Credentials
	BaseURL    string // e.g. https://myteam.slack.com/api/
}

func NewClient(creds *auth.Credentials, timeout time.Duration) *Client {
	base := fmt.Sprintf("https://%s.slack.com/api/", creds.Workspace)
	return &Client{
		HTTPClient: &http.Client{
			Timeout:   timeout,
			Transport: transport.NewChromeTransport(),
		},
		Creds:   creds,
		BaseURL: base,
	}
}

type SlackResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (c *Client) Post(method string, params url.Values) (json.RawMessage, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("token", c.Creds.Token)

	req, err := http.NewRequest("POST", c.BaseURL+method, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", "d="+c.Creds.Cookie)

	// Mimic Edge browser request headers to avoid Enterprise Grid security detection
	origin := fmt.Sprintf("https://%s.slack.com", c.Creds.Workspace)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", origin+"/")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Ch-Ua", `"Microsoft Edge";v="131", "Chromium";v="131", "Not_A Brand";v="24"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	// Capture rotated d cookie from response
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "d" && cookie.Value != "" && cookie.Value != c.Creds.Cookie {
			c.Creds.Cookie = cookie.Value
			// Best-effort persist — don't fail the request if save fails
			_ = auth.AddOrUpdateCredentials(*c.Creds)
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read error: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var sr SlackResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}
	if !sr.OK {
		return nil, fmt.Errorf("slack API error: %s", sr.Error)
	}

	return json.RawMessage(body), nil
}

// AuthTest validates the current credentials
func (c *Client) AuthTest() (*AuthTestResponse, error) {
	data, err := c.Post("auth.test", nil)
	if err != nil {
		return nil, err
	}
	var resp AuthTestResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type AuthTestResponse struct {
	SlackResponse
	URL    string `json:"url"`
	Team   string `json:"team"`
	User   string `json:"user"`
	TeamID string `json:"team_id"`
	UserID string `json:"user_id"`
}

// ConversationsList lists channels
func (c *Client) ConversationsList(types string, limit int) (*ConversationsListResponse, error) {
	params := url.Values{}
	if types != "" {
		params.Set("types", types)
	}
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("exclude_archived", "true")
	data, err := c.Post("conversations.list", params)
	if err != nil {
		return nil, err
	}
	var resp ConversationsListResponse
	return &resp, json.Unmarshal(data, &resp)
}

type ConversationsListResponse struct {
	SlackResponse
	Channels []Channel `json:"channels"`
}

type Channel struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsChannel  bool   `json:"is_channel"`
	IsIM       bool   `json:"is_im"`
	IsMPIM     bool   `json:"is_mpim"`
	IsPrivate  bool   `json:"is_private"`
	NumMembers int    `json:"num_members"`
	Topic      Topic  `json:"topic"`
	Purpose    Topic  `json:"purpose"`
	User       string `json:"user"` // for IMs
}

type Topic struct {
	Value string `json:"value"`
}

// ConversationsHistory gets messages from a channel
func (c *Client) ConversationsHistory(channelID string, limit int) (*HistoryResponse, error) {
	params := url.Values{}
	params.Set("channel", channelID)
	params.Set("limit", fmt.Sprintf("%d", limit))
	data, err := c.Post("conversations.history", params)
	if err != nil {
		return nil, err
	}
	var resp HistoryResponse
	return &resp, json.Unmarshal(data, &resp)
}

type HistoryResponse struct {
	SlackResponse
	Messages []Message `json:"messages"`
}

type Message struct {
	Type    string `json:"type"`
	User    string `json:"user"`
	Text    string `json:"text"`
	TS      string `json:"ts"`
	BotID   string `json:"bot_id,omitempty"`
	SubType string `json:"subtype,omitempty"`
}

// ChatPostMessage sends a message
func (c *Client) ChatPostMessage(channelID, text string) error {
	params := url.Values{}
	params.Set("channel", channelID)
	params.Set("text", text)
	_, err := c.Post("chat.postMessage", params)
	return err
}

// SearchMessages searches messages
func (c *Client) SearchMessages(query string, limit int) (*SearchResponse, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("count", fmt.Sprintf("%d", limit))
	data, err := c.Post("search.messages", params)
	if err != nil {
		return nil, err
	}
	var resp SearchResponse
	return &resp, json.Unmarshal(data, &resp)
}

type SearchResponse struct {
	SlackResponse
	Messages SearchMessages `json:"messages"`
}

type SearchMessages struct {
	Matches []SearchMatch `json:"matches"`
	Total   int           `json:"total"`
}

type SearchMatch struct {
	Channel  SearchChannel `json:"channel"`
	Username string        `json:"username"`
	Text     string        `json:"text"`
	TS       string        `json:"ts"`
}

type SearchChannel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UsersList lists users
func (c *Client) UsersList(limit int) (*UsersListResponse, error) {
	params := url.Values{}
	params.Set("limit", fmt.Sprintf("%d", limit))
	data, err := c.Post("users.list", params)
	if err != nil {
		return nil, err
	}
	var resp UsersListResponse
	return &resp, json.Unmarshal(data, &resp)
}

type UsersListResponse struct {
	SlackResponse
	Members []User `json:"members"`
}

type User struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	RealName string      `json:"real_name"`
	Deleted  bool        `json:"deleted"`
	IsBot    bool        `json:"is_bot"`
	Profile  UserProfile `json:"profile"`
}

type UserProfile struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	StatusText  string `json:"status_text"`
	StatusEmoji string `json:"status_emoji"`
	Title       string `json:"title"`
}

// UsersInfo gets info about a user
func (c *Client) UsersInfo(userID string) (*UsersInfoResponse, error) {
	params := url.Values{}
	params.Set("user", userID)
	data, err := c.Post("users.info", params)
	if err != nil {
		return nil, err
	}
	var resp UsersInfoResponse
	return &resp, json.Unmarshal(data, &resp)
}

type UsersInfoResponse struct {
	SlackResponse
	User User `json:"user"`
}

// TeamInfo gets workspace info
func (c *Client) TeamInfo() (*TeamInfoResponse, error) {
	data, err := c.Post("team.info", nil)
	if err != nil {
		return nil, err
	}
	var resp TeamInfoResponse
	return &resp, json.Unmarshal(data, &resp)
}

type TeamInfoResponse struct {
	SlackResponse
	Team TeamDetail `json:"team"`
}

type TeamDetail struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

// UsersGetPresence gets user presence
func (c *Client) UsersGetPresence(userID string) (*PresenceResponse, error) {
	params := url.Values{}
	params.Set("user", userID)
	data, err := c.Post("users.getPresence", params)
	if err != nil {
		return nil, err
	}
	var resp PresenceResponse
	return &resp, json.Unmarshal(data, &resp)
}

type PresenceResponse struct {
	SlackResponse
	Presence string `json:"presence"`
}

// ResolveChannelID tries to resolve a channel name to ID
func (c *Client) ResolveChannelID(nameOrID string) (string, error) {
	// If it looks like an ID already, return as-is
	if strings.HasPrefix(nameOrID, "C") || strings.HasPrefix(nameOrID, "G") || strings.HasPrefix(nameOrID, "D") {
		return nameOrID, nil
	}
	// Strip # prefix
	nameOrID = strings.TrimPrefix(nameOrID, "#")

	resp, err := c.ConversationsList("public_channel,private_channel", 1000)
	if err != nil {
		return "", err
	}
	for _, ch := range resp.Channels {
		if ch.Name == nameOrID {
			return ch.ID, nil
		}
	}
	return "", fmt.Errorf("channel %q not found", nameOrID)
}

// ResolveUserID tries to resolve a username to ID
func (c *Client) ResolveUserID(nameOrID string) (string, error) {
	if strings.HasPrefix(nameOrID, "U") || strings.HasPrefix(nameOrID, "W") {
		return nameOrID, nil
	}
	nameOrID = strings.TrimPrefix(nameOrID, "@")

	resp, err := c.UsersList(1000)
	if err != nil {
		return "", err
	}
	for _, u := range resp.Members {
		if u.Name == nameOrID || u.Profile.DisplayName == nameOrID {
			return u.ID, nil
		}
	}
	return "", fmt.Errorf("user %q not found", nameOrID)
}

// OpenDM opens a DM channel with a user
func (c *Client) OpenDM(userID string) (string, error) {
	params := url.Values{}
	params.Set("users", userID)
	data, err := c.Post("conversations.open", params)
	if err != nil {
		return "", err
	}
	var resp struct {
		SlackResponse
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.Channel.ID, nil
}
