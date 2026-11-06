package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// RPCError carries safe database validation errors, never credentials or tokens.
type RPCError struct {
	Status  int
	Code    string
	Message string
}

func (e *RPCError) Error() string { return e.Message }

// RPC executes a database function with the requesting user's JWT. The API key
// identifies the project; it is deliberately not a service-role key.
func (c *Client) RPC(ctx context.Context, token, name string, args interface{}, result interface{}) error {
	payload, err := json.Marshal(args)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/rest/v1/rpc/"+name, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("apikey", c.key)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		var failure struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(response.Body).Decode(&failure)
		status := http.StatusBadGateway
		switch {
		case response.StatusCode == 401:
			status = 401
		case failure.Code == "42501":
			status = 403
		case failure.Code == "P0002":
			status = 404
		case response.StatusCode == 400 || failure.Code == "22023" || failure.Code == "23514" || failure.Code == "23505":
			status = 400
		}
		message := "The database could not complete this request"
		if status == 400 || status == 403 || status == 404 {
			message = failure.Message
		}
		return &RPCError{Status: status, Code: failure.Code, Message: message}
	}
	if err := json.NewDecoder(response.Body).Decode(result); err != nil {
		return fmt.Errorf("invalid database response: %w", err)
	}
	return nil
}
