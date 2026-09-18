package pgrepo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// FormSession is a form_sessions projection.
type FormSession struct {
	SessionID string
	Stage     string
}

// LatestFormSession mirrors the existingSession lookup in buildFifteenDayCheckinFeedbackUrl.
func (s *TrayaStore) LatestFormSession(ctx context.Context, caseID, formID string) (*FormSession, error) {
	var fs FormSession
	err := s.Pool.QueryRow(ctx, `SELECT session_id::text, stage::text FROM form_sessions WHERE case_id = $1 AND form_id = $2 ORDER BY created_at DESC LIMIT 1`, caseID, formID).Scan(&fs.SessionID, &fs.Stage)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &fs, nil
}

// CreateFormSession mirrors createFifteenDayCheckinFormSession's forge().save().
func (s *TrayaStore) CreateFormSession(ctx context.Context, formID, formName, userID, caseID, gender string, currentQuestionID *string, now time.Time) (string, error) {
	history := []string{}
	if currentQuestionID != nil {
		history = append(history, *currentQuestionID)
	}
	hist, _ := json.Marshal(history)
	ctxJSON, _ := json.Marshal(map[string]string{"gender": gender, "source": "app"})
	var sid string
	err := s.Pool.QueryRow(ctx, `INSERT INTO form_sessions
 (form_id, form_name, form_type, user_id, case_id, stage, current_question_id, question_history, context, started_at, last_activity_at, created_at, updated_at)
 VALUES ($1,$2,'feedback',$3,$4,'in_progress',$5,$6,$7,$8,$8,$8,$8) RETURNING session_id::text`,
		formID, formName, userID, caseID, currentQuestionID, hist, ctxJSON, now).Scan(&sid)
	return sid, err
}

// FeedbackFormExists reports a feedback_forms row for the session.
func (s *TrayaStore) FeedbackFormExists(ctx context.Context, sessionID string) (bool, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT 1 FROM feedback_forms WHERE form_session_id = $1 LIMIT 1`, sessionID).Scan(&n)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// CreateFeedbackForm mirrors the FeedbackForm + placeholder FeedbackFormsResponse creation.
func (s *TrayaStore) CreateFeedbackForm(ctx context.Context, sessionID, caseID, userID, formID, formName string, now time.Time) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var feedbackID string
	if err := tx.QueryRow(ctx, `INSERT INTO feedback_forms
 (form_session_id, case_id, user_id, form_id, form_name, component_id, component_name, component_stage, shown_at, created_at, updated_at)
 VALUES ($1,$2,$3,$4,$5,$4,$5,'in_progress',$6,$6,$6) RETURNING feedback_id::text`, sessionID, caseID, userID, formID, formName, now).Scan(&feedbackID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO feedback_forms_responses (feedback_id, created_at, updated_at) VALUES ($1,$2,$2)`, feedbackID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// LatestSyntheticID mirrors getFormStatusAndSyntheticId's synthetic_id read.
func (s *TrayaStore) LatestSyntheticID(ctx context.Context, caseID string) (string, error) {
	var sid *string
	err := s.Pool.QueryRow(ctx, `SELECT synthetic_id::text FROM form_responses WHERE case_id = $1 AND question_id NOT IN ('sdp1','sdp2','sdp3','sdp4','sdp5') ORDER BY created_at DESC LIMIT 1`, caseID).Scan(&sid)
	if err == pgx.ErrNoRows || sid == nil {
		return "", nil
	}
	return *sid, err
}
