package pgrepo

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// UserCase joins cases and users.
type UserCase struct {
	UserID, CaseID, PhoneNumber, Gender, Email, FirstName string
}

const userCaseCols = `c.user_id::text, c.id::text, COALESCE(u.phone_number,''), COALESCE(u.gender,''), COALESCE(u.email,''), COALESCE(u.first_name,'')`

func (s *TrayaStore) scanUserCase(row pgx.Row) (*UserCase, error) {
	var uc UserCase
	if err := row.Scan(&uc.UserID, &uc.CaseID, &uc.PhoneNumber, &uc.Gender, &uc.Email, &uc.FirstName); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &uc, nil
}

// UserCaseByUserID mirrors Case.where({user_id}).fetch({withRelated:['user']}).
func (s *TrayaStore) UserCaseByUserID(ctx context.Context, userID string) (*UserCase, error) {
	return s.scanUserCase(s.Pool.QueryRow(ctx, `SELECT `+userCaseCols+` FROM cases c JOIN users u ON u.id = c.user_id WHERE c.user_id = $1 ORDER BY c.created_at ASC LIMIT 1`, userID))
}

// UserCaseByCaseID resolves a case and its owner.
func (s *TrayaStore) UserCaseByCaseID(ctx context.Context, caseID string) (*UserCase, error) {
	return s.scanUserCase(s.Pool.QueryRow(ctx, `SELECT `+userCaseCols+` FROM cases c JOIN users u ON u.id = c.user_id WHERE c.id = $1 LIMIT 1`, caseID))
}

// UserIDFromCaseID returns cases.user_id ("" when the case does not exist).
func (s *TrayaStore) UserIDFromCaseID(ctx context.Context, caseID string) (string, error) {
	var uid string
	err := s.Pool.QueryRow(ctx, `SELECT user_id::text FROM cases WHERE id = $1 LIMIT 1`, caseID).Scan(&uid)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return uid, err
}

// UserExists reports whether users.id exists (used to accept a raw userId on /rewardBalance/:customerId).
func (s *TrayaStore) UserExists(ctx context.Context, userID string) (bool, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT 1 FROM users WHERE id = $1 LIMIT 1`, userID).Scan(&n)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}
