package supabase

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func NewClient(supabaseURL, publishableKey string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(supabaseURL) == "" || strings.TrimSpace(publishableKey) == "" {
		return nil, fmt.Errorf("Supabase URL and publishable key are required")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		BaseURL:    strings.TrimRight(supabaseURL, "/") + "/rest/v1",
		APIKey:     publishableKey,
		HTTPClient: httpClient,
	}, nil
}

func (c *Client) NewRequest(ctx context.Context, method string, query url.Values, body io.Reader, accessToken string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/tasks?"+query.Encode(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("apikey", c.APIKey)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Prefer", "return=representation")
	}
	return request, nil
}
