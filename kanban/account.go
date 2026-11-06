package kanban

import (
	"github.com/gin-gonic/gin"
	"strings"
)

func (c *Collaboration) account(ctx *gin.Context, action string) {
	token, _ := accessToken(ctx.Request.Context())
	input := map[string]interface{}{}
	if ctx.Request.Method == "POST" {
		var err error
		input, err = readBoardInput(ctx)
		if err != nil {
			ctx.JSON(400, gin.H{"error": "Invalid account request"})
			return
		}
	}
	if action == "session" {
		refresh, _ := input["refresh_token"].(string)
		if refresh == "" {
			ctx.JSON(400, gin.H{"error": "Confirmation session is missing"})
			return
		}
		session, err := c.client.RefreshSession(ctx.Request.Context(), refresh)
		if err != nil {
			ctx.JSON(401, gin.H{"error": "Confirmation link expired. Return to the original browser and try again."})
			return
		}
		ctx.JSON(200, session)
		return
	}
	if action == "login" {
		email, _ := input["email"].(string)
		password, _ := input["password"].(string)
		if len(email) > 254 || len(password) > 256 || email == "" || password == "" {
			ctx.JSON(400, gin.H{"error": "Email and password required"})
			return
		}
		session, err := c.client.PasswordSession(ctx.Request.Context(), email, password)
		if err != nil {
			ctx.JSON(401, gin.H{"error": "Unable to sign in. Check your email, password and email confirmation."})
			return
		}
		ctx.JSON(200, session)
		return
	}
	user, err := c.client.AuthUser(ctx.Request.Context(), token, nil)
	if err != nil {
		ctx.JSON(401, gin.H{"error": "Unable to verify account. Please refresh the page."})
		return
	}
	if action == "password" {
		password, _ := input["password"].(string)
		if user["email_confirmed_at"] == nil || user["email_confirmed_at"] == "" {
			ctx.JSON(400, gin.H{"error": "Confirm your email before setting a password"})
			return
		}
		if len(password) < 8 || len(password) > 256 {
			ctx.JSON(400, gin.H{"error": "Use a password of 8–256 characters"})
			return
		}
		user, err = c.client.AuthUser(ctx.Request.Context(), token, map[string]interface{}{"password": password, "data": map[string]interface{}{"password_configured": true}})
		if err != nil {
			ctx.JSON(400, gin.H{"error": "Password could not be saved. Try a stronger password."})
			return
		}
	}
	metadata, _ := user["user_metadata"].(map[string]interface{})
	email, _ := user["email"].(string)
	confirmed := user["email_confirmed_at"] != nil && user["email_confirmed_at"] != ""
	configured := metadata != nil && metadata["password_configured"] == true
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(200, gin.H{"id": user["id"], "email": strings.TrimSpace(email), "confirmed": confirmed, "password_configured": configured, "anonymous": user["is_anonymous"]})
}
