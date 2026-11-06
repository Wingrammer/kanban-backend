package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/supabase-community/gotrue-go/types"
	"net/http"
	"strings"
)

// AuthUser always asks Auth for current metadata; a cached JWT may still say anonymous.
func (c *Client) AuthUser(ctx context.Context, token string, changes map[string]interface{}) (map[string]interface{}, error) {
	method := http.MethodGet
	var payload []byte
	if changes != nil {
		method = http.MethodPut
		payload, _ = json.Marshal(changes)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.url, "/")+"/auth/v1/user", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", c.key)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("auth user request failed: %d", response.StatusCode)
	}
	var user map[string]interface{}
	err = json.NewDecoder(response.Body).Decode(&user)
	return user, err
}

func (c *Client) PasswordSession(ctx context.Context, email, password string) (types.Session, error) {
	payload, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.url, "/")+"/auth/v1/token?grant_type=password", bytes.NewReader(payload))
	if err != nil {
		return types.Session{}, err
	}
	req.Header.Set("apikey", c.key)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return types.Session{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return types.Session{}, fmt.Errorf("sign in rejected")
	}
	var session types.Session
	err = json.NewDecoder(response.Body).Decode(&session)
	return session, err
}
