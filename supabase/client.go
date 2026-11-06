package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/supabase-community/gotrue-go/types"
	postgrest "github.com/supabase-community/postgrest-go"
	supabasego "github.com/supabase-community/supabase-go"
)

type Client struct {
	url        string
	key        string
	httpClient *http.Client
}

func NewClient(supabaseURL, publishableKey string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(supabaseURL) == "" || strings.TrimSpace(publishableKey) == "" {
		return nil, fmt.Errorf("Supabase URL and publishable key are required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{url: supabaseURL, key: publishableKey, httpClient: httpClient}, nil
}

func (c *Client) From(table, accessToken string) (*postgrest.QueryBuilder, error) {
	client, err := supabasego.NewClient(c.url, c.key, &supabasego.ClientOptions{
		Headers: map[string]string{
			"Authorization": "Bearer " + accessToken,
		},
	})
	if err != nil {
		return nil, err
	}
	return client.From(table), nil
}

func (c *Client) CreateAnonymousSession(ctx context.Context) (types.Session, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(c.url, "/")+"/auth/v1/signup",
		bytes.NewReader([]byte(`{}`)),
	)
	if err != nil {
		return types.Session{}, err
	}
	request.Header.Set("apikey", c.key)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return types.Session{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return types.Session{}, fmt.Errorf("Supabase anonymous sign-in failed with status %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}

	var session types.Session
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		return types.Session{}, err
	}
	if session.ExpiresAt <= 0 {
		session.ExpiresAt = time.Now().Unix() + int64(session.ExpiresIn)
	}
	return session, nil
}

func (c *Client) LinkAnonymousAccount(ctx context.Context, accessToken, email, password string) (types.User, error) {
	client, err := supabasego.NewClient(c.url, c.key, &supabasego.ClientOptions{})
	if err != nil {
		return types.User{}, err
	}
	response, err := client.Auth.WithToken(accessToken).UpdateUser(types.UpdateUserRequest{
		Email:    email,
		Password: &password,
	})
	if err != nil {
		return types.User{}, err
	}
	return response.User, nil
}

var ErrInvalidRefresh = errors.New("invalid refresh token")

func (c *Client) RefreshSession(ctx context.Context, refreshToken string) (types.Session, error) {
	payload, err := json.Marshal(map[string]string{"refresh_token": refreshToken})
	if err != nil {
		return types.Session{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.url, "/")+"/auth/v1/token?grant_type=refresh_token", bytes.NewReader(payload))
	if err != nil {
		return types.Session{}, err
	}
	request.Header.Set("apikey", c.key)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return types.Session{}, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return types.Session{}, ErrInvalidRefresh
	}
	if response.StatusCode != http.StatusOK {
		return types.Session{}, fmt.Errorf("Supabase refresh failed with status %d", response.StatusCode)
	}
	var session types.Session
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		return types.Session{}, err
	}
	if session.AccessToken == "" || session.RefreshToken == "" || session.ExpiresIn <= 0 {
		return types.Session{}, fmt.Errorf("Supabase returned an incomplete session")
	}
	if session.ExpiresAt <= 0 {
		session.ExpiresAt = time.Now().Unix() + int64(session.ExpiresIn)
	}
	return session, nil
}
