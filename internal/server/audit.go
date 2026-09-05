package server

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (s *Server) audit(ctx context.Context, user *AuthUser, action, entityType, entityID, summary, beforeJSON, afterJSON string, request *http.Request) {
	var userID any
	if user != nil {
		userID = user.ID
	}
	var ip, agent string
	if request != nil {
		ip = requestIP(request)
		agent = request.UserAgent()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_logs(id, user_id, action, entity_type, entity_id, summary, before_json, after_json, ip_address, user_agent, created_at)
		VALUES(?, ?, ?, ?, ?, ?, NULLIF(?,''), NULLIF(?,''), ?, ?, ?)`, uuid.NewString(), userID, action, entityType, entityID, summary, beforeJSON, afterJSON, ip, agent, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		s.logger.Error("write audit log", "error", err, "action", action, "entity", entityType, "entity_id", entityID)
	}
}
