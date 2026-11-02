package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/gin-gonic/gin"
)

type turnstileCheckResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

var turnstileHTTPClient = &http.Client{Timeout: 5 * time.Second}

func verifyTurnstile(ctx context.Context, token, remoteIP string) (bool, error) {
	if strings.TrimSpace(common.TurnstileSecretKey) == "" {
		return false, errors.New("turnstile secret key is not configured")
	}
	form := url.Values{
		"secret":   {common.TurnstileSecretKey},
		"response": {token},
	}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://challenges.cloudflare.com/turnstile/v0/siteverify",
		strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := turnstileHTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return false, errors.New("turnstile verification returned a non-success status")
	}
	var verification turnstileCheckResponse
	if err := json.NewDecoder(res.Body).Decode(&verification); err != nil {
		return false, err
	}
	return verification.Success, nil
}

func TurnstileCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		if common.TurnstileCheckEnabled {
			response := strings.TrimSpace(c.GetHeader("X-Turnstile-Token"))
			if response == "" {
				c.JSON(http.StatusForbidden, gin.H{
					"success": false,
					"code":    "TURNSTILE_TOKEN_REQUIRED",
					"message": "Turnstile verification is required",
				})
				c.Abort()
				return
			}
			verified, err := verifyTurnstile(c.Request.Context(), response, c.ClientIP())
			if err != nil || !verified {
				if err != nil {
					common.SysLog("turnstile verification failed: " + err.Error())
				}
				c.JSON(http.StatusForbidden, gin.H{
					"success": false,
					"code":    "TURNSTILE_VERIFICATION_FAILED",
					"message": "Turnstile verification failed",
				})
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
