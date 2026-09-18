package legacy

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/orders"
	mongorepo "traya-bah-service/repositories/mongo"
	pgrepo "traya-bah-service/repositories/pg"
	"traya-bah-service/setup"
)

// HabitCreditInput is the payload for the v85 credit/mint call made from the shared write path.
type HabitCreditInput struct {
	UserID      string
	StreakDay   int
	PhoneNumber string
	CheckInDate time.Time
}

// HabitCreditResult mirrors app-backend's mint/credit response, including its optional keys.
type HabitCreditResult struct {
	Success         bool   `json:"success"`
	Minted          *bool  `json:"minted,omitempty"`
	Credited        *bool  `json:"credited,omitempty"`
	Paused          *bool  `json:"paused,omitempty"`
	AlreadyCredited *bool  `json:"alreadyCredited,omitempty"`
	AlreadyExists   *bool  `json:"alreadyExists,omitempty"`
	CardID          string `json:"cardId,omitempty"`
	Coins           *int   `json:"coins,omitempty"`
	Type            string `json:"type,omitempty"`
	Card            any    `json:"card,omitempty"`
	Message         string `json:"message,omitempty"`
}

// HabitCreditor is implemented by internal/habit; declared here to avoid an import cycle.
type HabitCreditor interface {
	MintOrCredit(ctx context.Context, in HabitCreditInput) (HabitCreditResult, error)
}

// CCDPublisher emits customer-computed-data events.
type CCDPublisher interface {
	Publish(ctx context.Context, tenantID, eventType, caseID string, payload map[string]any)
}

// OrderSource supplies a user's non-void orders (Traya Postgres in production).
type OrderSource interface {
	NonVoidOrdersByUser(ctx context.Context, userID string) ([]orders.Order, error)
	NonVoidOrdersByCase(ctx context.Context, caseID string) ([]orders.Order, error)
	LatestNonVoidUnknownOrders(ctx context.Context, userID string) ([]orders.Order, error)
	FirstDeliveredDate(ctx context.Context, userID string) (*time.Time, error)
	LatestOrderAndCount(ctx context.Context, userID string) (*orders.Order, int, error)
	UserCaseByUserID(ctx context.Context, userID string) (*pgrepo.UserCase, error)
	UserCaseByCaseID(ctx context.Context, caseID string) (*pgrepo.UserCase, error)
	UserIDFromCaseID(ctx context.Context, caseID string) (string, error)
	UserExists(ctx context.Context, userID string) (bool, error)
	ProductsByPrincipalIDs(ctx context.Context, ids []string) ([]pgrepo.ProductDesc, error)
	FinishedReminders(ctx context.Context, userID string, since time.Time) ([]pgrepo.ReminderRow, error)
	LatestFormSession(ctx context.Context, caseID, formID string) (*pgrepo.FormSession, error)
	CreateFormSession(ctx context.Context, formID, formName, userID, caseID, gender string, currentQuestionID *string, now time.Time) (string, error)
	FeedbackFormExists(ctx context.Context, sessionID string) (bool, error)
	CreateFeedbackForm(ctx context.Context, sessionID, caseID, userID, formID, formName string, now time.Time) error
	LatestSyntheticID(ctx context.Context, caseID string) (string, error)
}

// Deps are the collaborators a legacy Service needs.
type Deps struct {
	Store        *mongorepo.Store
	PG           OrderSource
	Redis        *redis.Client
	Clock        common.Clock
	Cfg          *setup.Config
	HTTP         *setup.HTTPClients
	ConfigClient *orders.ConfigServiceClient
	Shopflo      *ShopfloClient
	Habit        HabitCreditor
	CCD          CCDPublisher
	Dispatcher   *Dispatcher
	Log          *slog.Logger
}

// Service is the per-tenant legacy economy.
type Service struct {
	Deps
	TenantID string
}

// New builds a Service for a tenant.
func New(tenantID string, d Deps) *Service {
	if d.Clock == nil {
		d.Clock = common.RealClock{}
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Service{Deps: d, TenantID: tenantID}
}

func (s *Service) now() time.Time { return s.Clock.Now() }

func (s *Service) httpClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return http.DefaultClient
}
