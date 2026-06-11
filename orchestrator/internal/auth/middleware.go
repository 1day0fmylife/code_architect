package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

func Bearer(token string, disabled bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if disabled || token == "" {
				return next(c)
			}
			authHeader := c.Request().Header.Get("Authorization")
			if !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing bearer token")
			}
			got := strings.TrimSpace(authHeader[len("Bearer "):])
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid bearer token")
			}
			return next(c)
		}
	}
}
