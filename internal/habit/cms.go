package habit

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// GetFeedbackCard ports getFeedbackCard: the O1-only feedback_v3 component from CMS.
// Returns nil on any failure or when the order count is not exactly 1.
func (s *Service) GetFeedbackCard(ctx context.Context, userID, caseID string, version, orderCount int) any {
	if userID == "" || caseID == "" {
		return nil
	}
	// The O1 gate is checked BEFORE the CMS call because mintFormSessions is a persisted side effect.
	if orderCount != FeedbackOrderCount {
		return nil
	}
	base := s.Cfg.CMSServiceBaseURL
	if base == "" {
		return nil
	}
	body, err := json.Marshal(map[string]any{
		"componentNames": []string{FeedbackComponent}, "userId": userID, "caseId": caseID,
		"version": version, "mintFormSessions": true,
	})
	if err != nil {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/api/component/data", bytes.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient(s.HTTP.CMS).Do(req)
	if err != nil {
		s.Log.Warn("getFeedbackCard failed", "userId", userID, "error", err.Error())
		return nil
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return nil
	}
	card, ok := payload[FeedbackComponent].(map[string]any)
	if !ok {
		return nil
	}
	contents, ok := card["contents"].([]any)
	if !ok || len(contents) == 0 {
		return nil
	}
	return card
}
