package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"servicemanager/internal/models"
	"servicemanager/internal/utils"
)

type contextKey string

const UserContextKey contextKey = "user"

type UserContext struct {
	UserID int
	Email  string
	Role   models.UserRole
}

func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := ""
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				parts := strings.Split(authHeader, " ")
				if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
					tokenString = parts[1]
				}
			}

			if tokenString == "" {
				if cookie, err := r.Cookie("token"); err == nil {
					tokenString = cookie.Value
				}
			}

			if tokenString == "" {
				tokenString = r.URL.Query().Get("token")
			}

			if tokenString == "" {
				utils.ErrorResponse(w, http.StatusUnauthorized, fmt.Errorf("authorization token missing"))
				return
			}

			claims, err := utils.VerifyToken(tokenString, secret)
			if err != nil {
				utils.ErrorResponse(w, http.StatusUnauthorized, fmt.Errorf("invalid or expired token"))
				return
			}

			userCtx := UserContext{
				UserID: claims.UserID,
				Email:  claims.Email,
				Role:   models.UserRole(claims.Role),
			}

			ctx := context.WithValue(r.Context(), UserContextKey, userCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireRole(roles ...models.UserRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userCtx, ok := r.Context().Value(UserContextKey).(UserContext)
			if !ok {
				utils.ErrorResponse(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
				return
			}

			allowed := false
			for _, role := range roles {
				if userCtx.Role == role {
					allowed = true
					break
				}
			}

			if !allowed {
				utils.ErrorResponse(w, http.StatusForbidden, fmt.Errorf("forbidden: insufficient permissions"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
