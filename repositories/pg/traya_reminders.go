package pgrepo

import (
	"context"
	"time"
)

// ReminderRow is a user_order_reminders projection.
type ReminderRow struct {
	Tag        string
	ActualDate *time.Time
}

// FinishedReminders mirrors checkFifteenDayCheckinCallEligibility's query.
func (s *TrayaStore) FinishedReminders(ctx context.Context, userID string, since time.Time) ([]ReminderRow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT COALESCE(tag,''), actual_date FROM user_order_reminders
WHERE user_id = $1 AND is_finished = true AND (tag IN ('Week #1','Week #3') OR actual_date >= $2)`, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReminderRow
	for rows.Next() {
		var r ReminderRow
		if err := rows.Scan(&r.Tag, &r.ActualDate); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
