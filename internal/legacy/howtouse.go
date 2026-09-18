package legacy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"traya-bah-service/internal/common"
)

// ProxyResult is an upstream response passed through verbatim.
type ProxyResult struct {
	Status int
	Body   []byte
}

// recommendationGet calls recommendation-service with the V2 token and traya tenant header.
func (s *Service) recommendationGet(ctx context.Context, path string, query url.Values) (ProxyResult, error) {
	base := s.Cfg.RecommendationBaseURL
	if base == "" {
		return ProxyResult{}, common.Internal("RECOMMENDATION_SERVICE_BASE_URL is not configured")
	}
	u := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ProxyResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.Cfg.V2FormDataToken)
	req.Header.Set("x-tenant-id", "traya")
	resp, err := s.httpClient(s.HTTP.Recommendation).Do(req)
	if err != nil {
		return ProxyResult{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return ProxyResult{}, err
	}
	return ProxyResult{Status: resp.StatusCode, Body: body}, nil
}

// forwardedQuery keeps only the query params api-server forwards.
func forwardedQuery(src url.Values, keys ...string) url.Values {
	out := url.Values{}
	for _, k := range keys {
		if v := src.Get(k); v != "" {
			out.Set(k, v)
		}
	}
	return out
}

// LatestOrderHowToUseV2 proxies how-to-use/{caseId} and merges reminderInfo from customeractivitylogs.
func (s *Service) LatestOrderHowToUseV2(ctx context.Context, caseID string, query url.Values) (ProxyResult, error) {
	res, err := s.recommendationGet(ctx, "how-to-use/"+caseID, forwardedQuery(query, "orderId", "cxType", "lang"))
	if err != nil || res.Status < 200 || res.Status >= 300 {
		return res, err
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Body, &payload); err != nil {
		return res, nil // not an object: pass through untouched
	}
	reminder, err := s.Store.LatestReminderInfo(ctx, caseID)
	if err != nil {
		s.Log.Warn("reminder info lookup failed", "caseId", caseID, "error", err.Error())
	}
	if reminder != nil && reminder.CaseID != "" {
		payload["reminderInfo"] = map[string]any{
			"reminder_days": reminder.ReminderDays, "order_id": reminder.OrderID, "action_date": reminder.ActionDate,
		}
	} else {
		payload["reminderInfo"] = nil
	}
	merged, err := json.Marshal(payload)
	if err != nil {
		return res, nil
	}
	return ProxyResult{Status: res.Status, Body: merged}, nil
}

// LatestRoutineV2 proxies routine/{caseId}.
func (s *Service) LatestRoutineV2(ctx context.Context, caseID string, query url.Values) (ProxyResult, error) {
	return s.recommendationGet(ctx, "routine/"+caseID, forwardedQuery(query, "productId", "orderId", "cxType", "lang"))
}
