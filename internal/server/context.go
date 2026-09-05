package server

import (
	"context"
	"net"
	"net/http"
)

type contextKey string

const userContextKey contextKey = "user"

type AuthUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

func userFromContext(ctx context.Context) (AuthUser, bool) {
	user, ok := ctx.Value(userContextKey).(AuthUser)
	return user, ok
}

func requestIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
