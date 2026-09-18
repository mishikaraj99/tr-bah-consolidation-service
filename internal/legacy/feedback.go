package legacy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// resolveFifteenDayInitialQuestionID best-effort reads the form's first question id from CMS.
// Any failure yields nil, exactly like the Node service.
func (s *Service) resolveFifteenDayInitialQuestionID(ctx context.Context) *string {
	base := s.Cfg.CMSServiceBaseURL
	if base == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(base, "/")+"/api/component/"+FifteenDayCheckinFormID+"/form/published", nil)
	if err != nil {
		return nil
	}
	resp, err := s.httpClient(s.HTTP.CMS).Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil
	}
	var form struct {
		DisplayMode     string `json:"display_mode"`
		InitialQuestion string `json:"initial_question"`
		Questions       []struct {
			ContentID string `json:"content_id"`
			ID        string `json:"id"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(body, &form); err != nil || len(form.Questions) == 0 {
		return nil
	}
	first := form.Questions[0]
	pick := func(vals ...string) *string {
		for _, v := range vals {
			if v != "" {
				out := v
				return &out
			}
		}
		return nil
	}
	if form.DisplayMode == "linear" {
		return pick(first.ContentID, first.ID)
	}
	return pick(form.InitialQuestion, first.ContentID, first.ID)
}

// BuildFifteenDayCheckinFeedbackURL ports services/feedback/fifteenDayCheckin.js.
// Returns "" when the session is already completed or anything fails.
func (s *Service) BuildFifteenDayCheckinFeedbackURL(ctx context.Context, caseID, userID, gender, authToken string) string {
	existing, err := s.PG.LatestFormSession(ctx, caseID, FifteenDayCheckinFormID)
	if err != nil {
		s.Log.Warn("15-day checkin session lookup failed", "caseId", caseID, "error", err.Error())
		return ""
	}
	var sessionID string
	if existing != nil {
		if existing.Stage == "completed" {
			return ""
		}
		sessionID = existing.SessionID
	} else {
		sessionID, err = s.PG.CreateFormSession(ctx, FifteenDayCheckinFormID, FifteenDayCheckinFormName, userID, caseID, gender,
			s.resolveFifteenDayInitialQuestionID(ctx), s.now())
		if err != nil {
			s.Log.Warn("15-day checkin session create failed", "caseId", caseID, "error", err.Error())
			return ""
		}
	}
	exists, err := s.PG.FeedbackFormExists(ctx, sessionID)
	if err != nil {
		s.Log.Warn("15-day checkin feedback form lookup failed", "caseId", caseID, "error", err.Error())
		return ""
	}
	if !exists {
		if err := s.PG.CreateFeedbackForm(ctx, sessionID, caseID, userID, FifteenDayCheckinFormID, FifteenDayCheckinFormName, s.now()); err != nil {
			s.Log.Warn("15-day checkin feedback form create failed", "caseId", caseID, "error", err.Error())
			return ""
		}
	}
	syntheticID, err := s.PG.LatestSyntheticID(ctx, caseID)
	if err != nil {
		syntheticID = ""
	}
	return fmt.Sprintf("%s/form?session_id=%s&source=app&authToken=%s&syntheticId=%s", s.FeedbackUIDomain(), sessionID, authToken, syntheticID)
}
