package legacy

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/orders"
	mongorepo "traya-bah-service/repositories/mongo"
	pgrepo "traya-bah-service/repositories/pg"
	"traya-bah-service/setup"
)

// fakeCCD records published events.
type fakeCCD struct{ events []map[string]any }

func (f *fakeCCD) Publish(_ context.Context, tenantID, eventType, caseID string, payload map[string]any) {
	f.events = append(f.events, map[string]any{"tenantId": tenantID, "eventType": eventType, "caseId": caseID, "payload": payload})
}

// fakeOrders is an in-memory OrderSource.
type fakeOrders struct {
	byUser     []orders.Order
	userCase   *pgrepo.UserCase
	reminders  []pgrepo.ReminderRow
	products   []pgrepo.ProductDesc
	firstDeliv *time.Time
	latest     *orders.Order
	count      int
}

func (f *fakeOrders) NonVoidOrdersByUser(context.Context, string) ([]orders.Order, error) {
	return f.byUser, nil
}
func (f *fakeOrders) NonVoidOrdersByCase(context.Context, string) ([]orders.Order, error) {
	return f.byUser, nil
}
func (f *fakeOrders) LatestNonVoidUnknownOrders(context.Context, string) ([]orders.Order, error) {
	return f.byUser, nil
}
func (f *fakeOrders) FirstDeliveredDate(context.Context, string) (*time.Time, error) {
	return f.firstDeliv, nil
}
func (f *fakeOrders) LatestOrderAndCount(context.Context, string) (*orders.Order, int, error) {
	return f.latest, f.count, nil
}
func (f *fakeOrders) UserCaseByUserID(context.Context, string) (*pgrepo.UserCase, error) {
	return f.userCase, nil
}
func (f *fakeOrders) UserCaseByCaseID(context.Context, string) (*pgrepo.UserCase, error) {
	return f.userCase, nil
}
func (f *fakeOrders) UserIDFromCaseID(_ context.Context, caseID string) (string, error) {
	if f.userCase != nil {
		return f.userCase.UserID, nil
	}
	return "", nil
}
func (f *fakeOrders) UserExists(context.Context, string) (bool, error) { return true, nil }
func (f *fakeOrders) ProductsByPrincipalIDs(context.Context, []string) ([]pgrepo.ProductDesc, error) {
	return f.products, nil
}
func (f *fakeOrders) FinishedReminders(context.Context, string, time.Time) ([]pgrepo.ReminderRow, error) {
	return f.reminders, nil
}

// fakeHabit records MintOrCredit calls.
type fakeHabit struct {
	calls  []HabitCreditInput
	result HabitCreditResult
}

func (f *fakeHabit) MintOrCredit(_ context.Context, in HabitCreditInput) (HabitCreditResult, error) {
	f.calls = append(f.calls, in)
	return f.result, nil
}

func newTestService(t *testing.T, now time.Time) (*Service, *fakeCCD, *fakeOrders) {
	t.Helper()
	uri := os.Getenv("TEST_MONGO_URI")
	if uri == "" {
		t.Skip("TEST_MONGO_URI not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err)
	db := client.Database("bah_legacy_" + primitive.NewObjectID().Hex()[18:])
	t.Cleanup(func() { _ = db.Drop(context.Background()); _ = client.Disconnect(context.Background()) })
	require.NoError(t, mongorepo.EnsureIndexes(context.Background(), db))
	clk := common.FixedClock{T: now}
	ccd := &fakeCCD{}
	po := &fakeOrders{}
	svc := New("traya", Deps{
		Store: mongorepo.NewStore(db, clk), PG: po, Clock: clk, CCD: ccd,
		Cfg:  &setup.Config{IsProduction: false, S3ImageBaseURL: "https://cdn.test/", CommunityBaseURL: "https://community.test/"},
		HTTP: &setup.HTTPClients{},
		Log:  slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	return svc, ccd, po
}
