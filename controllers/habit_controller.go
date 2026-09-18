package controllers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/habit"
	"traya-bah-service/internal/legacy"
)

// bodyUserID resolves the identity for internal (server-to-server) routes.
func bodyUserID(c *fiber.Ctx, body map[string]any) string {
	if id := identity(c); id.UserID != "" {
		return id.UserID
	}
	if v, ok := body["userId"].(string); ok && v != "" {
		return v
	}
	return strings.TrimSpace(c.Query("userId"))
}

// ArchiveProduct handles POST /archiveProductForBAH and /bah/archive-product.
//
//	@Summary	Remove a product from the logging list
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/archiveProductForBAH [post]
//	@Tags		HabitTracker
func (d *Deps) ArchiveProduct(c *fiber.Ctx) error { return d.archiveToggle(c, true) }

// UnarchiveProduct handles POST /unarchiveProductForBAH and /bah/unarchive-product.
//
//	@Summary	Re-add a product to the logging list
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/unarchiveProductForBAH [post]
//	@Tags		HabitTracker
func (d *Deps) UnarchiveProduct(c *fiber.Ctx) error { return d.archiveToggle(c, false) }

func (d *Deps) archiveToggle(c *fiber.Ctx, archive bool) error {
	var body map[string]any
	_ = c.BodyParser(&body)
	userID := bodyUserID(c, body)
	if userID == "" || !isUUID(userID) {
		return common.WriteError(c, common.FamilyV85, common.BadRequest(habit.MsgInvalidUserID))
	}
	productID := ""
	if v, ok := body["productId"].(string); ok {
		productID = v
	}
	svc := d.habitSvc(c)
	var (
		out *habit.ArchiveResult
		err error
	)
	if archive {
		out, err = svc.ArchiveProduct(c.UserContext(), userID, productID)
	} else {
		out, err = svc.UnarchiveProduct(c.UserContext(), userID, productID)
	}
	if err != nil {
		return common.WriteError(c, common.FamilyV85, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// GetScratchCards handles GET /bah/scratch-card.
//
//	@Summary	Scratch card dashboard
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/bah/scratch-card [get]
//	@Tags		HabitTracker
func (d *Deps) GetScratchCards(c *fiber.Ctx) error {
	userID := identity(c).UserID
	if userID == "" {
		userID = strings.TrimSpace(c.Query("userId"))
	}
	if userID == "" || !isUUID(userID) {
		return common.WriteError(c, common.FamilyV85, common.BadRequest(habit.MsgInvalidUserID))
	}
	out, err := d.habitSvc(c).GetScratchCards(c.UserContext(), userID)
	if err != nil {
		return common.WriteError(c, common.FamilyV85, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// RevealScratchCard handles POST /bah/scratch-card/:id/reveal.
//
//	@Summary	Scratch a card and credit the coins
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/bah/scratch-card/{id}/reveal [post]
//	@Tags		HabitTracker
func (d *Deps) RevealScratchCard(c *fiber.Ctx) error {
	var body map[string]any
	_ = c.BodyParser(&body)
	userID := bodyUserID(c, body)
	if userID == "" || !isUUID(userID) {
		return common.WriteError(c, common.FamilyV85, common.BadRequest(habit.MsgInvalidUserID))
	}
	raw := strings.TrimSpace(c.Params("id"))
	if raw == "" {
		return common.WriteError(c, common.FamilyV85, common.BadRequest("Missing card id"))
	}
	oid, err := primitive.ObjectIDFromHex(raw)
	if err != nil {
		return common.WriteError(c, common.FamilyV85, common.NotFound(habit.MsgCardNotFound))
	}
	out, err := d.habitSvc(c).RevealScratchCard(c.UserContext(), userID, oid)
	if err != nil {
		return common.WriteError(c, common.FamilyV85, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// MintHabitTrackerCredit handles the server-to-server mint/credit routes.
//
//	@Summary	Credit or mint the habit-tracker reward for a streak day
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/bah/scratch-card/habit-tracker-mint [post]
//	@Tags		HabitTracker
func (d *Deps) MintHabitTrackerCredit(c *fiber.Ctx) error {
	var body struct {
		UserID      string `json:"userId"`
		StreakDay   int    `json:"streakDay"`
		PhoneNumber string `json:"phoneNumber"`
		CheckInDate string `json:"checkInDate"`
	}
	if err := c.BodyParser(&body); err != nil {
		return common.WriteError(c, common.FamilyV85, common.BadRequest("Invalid request body"))
	}
	if body.UserID == "" || !isUUID(body.UserID) {
		return common.WriteError(c, common.FamilyV85, common.BadRequest(habit.MsgInvalidUserID))
	}
	if body.StreakDay < 1 {
		return common.WriteError(c, common.FamilyV85, common.BadRequest(habit.MsgInvalidStreakDay))
	}
	if body.PhoneNumber == "" {
		return common.WriteError(c, common.FamilyV85, common.BadRequest(habit.MsgMissingPhoneNumber))
	}
	if body.CheckInDate == "" {
		return common.WriteError(c, common.FamilyV85, common.BadRequest(habit.MsgMissingCheckInDate))
	}
	checkIn, err := parseFlexibleDate(body.CheckInDate)
	if err != nil {
		return common.WriteError(c, common.FamilyV85, common.BadRequest(habit.MsgMissingCheckInDate))
	}
	out, err := d.habitSvc(c).MintOrCredit(c.UserContext(), legacy.HabitCreditInput{
		UserID: body.UserID, StreakDay: body.StreakDay, PhoneNumber: body.PhoneNumber, CheckInDate: checkIn,
	})
	if err != nil {
		return common.WriteError(c, common.FamilyV85, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// RedeemCoins handles POST /bah/coin/redeem.
//
//	@Summary	Redeem coins for a non-order transaction
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/bah/coin/redeem [post]
//	@Tags		HabitTracker
func (d *Deps) RedeemCoins(c *fiber.Ctx) error {
	var in habit.RedeemInput
	if err := c.BodyParser(&in); err != nil {
		return common.WriteError(c, common.FamilyV85, common.BadRequest("Invalid request body"))
	}
	out, err := d.habitSvc(c).RedeemCoinsNonOrder(c.UserContext(), in)
	if err != nil {
		return common.WriteError(c, common.FamilyV85, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// kitTrackerIdentity resolves userId/caseId for the public and internal kit-tracker routes.
func kitTrackerIdentity(c *fiber.Ctx) (userID, caseID string) {
	id := identity(c)
	userID, caseID = id.UserID, id.CaseID
	if userID == "" {
		userID = strings.TrimSpace(c.Query("userId"))
	}
	if caseID == "" {
		caseID = strings.TrimSpace(c.Query("caseId"))
	}
	return userID, caseID
}

// GetKitTrackerPage handles GET /kit-tracker-page and /config/cms/kit-tracker-page.
//
//	@Summary	Kit tracker home surface
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/kit-tracker-page [get]
//	@Tags		HabitTracker
func (d *Deps) GetKitTrackerPage(c *fiber.Ctx) error {
	userID, caseID := kitTrackerIdentity(c)
	if userID == "" || !isUUID(userID) {
		return common.WriteError(c, common.FamilyV85, common.BadRequest(habit.MsgInvalidUserID))
	}
	if caseID == "" || !isUUID(caseID) {
		return common.WriteError(c, common.FamilyV85, common.BadRequest(habit.MsgInvalidCaseID))
	}
	out, err := d.habitSvc(c).GetKitTrackerPage(c.UserContext(), userID, caseID, identity(c).AppVersion)
	if err != nil {
		return common.WriteError(c, common.FamilyV85, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// GetKitTrackerCalendar handles GET /kit-tracker-calendar and /config/cms/kit-tracker-calendar.
//
//	@Summary	Kit tracker month calendar
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/kit-tracker-calendar [get]
//	@Tags		HabitTracker
func (d *Deps) GetKitTrackerCalendar(c *fiber.Ctx) error {
	userID, caseID := kitTrackerIdentity(c)
	if userID == "" || !isUUID(userID) {
		return common.WriteError(c, common.FamilyV85, common.BadRequest(habit.MsgInvalidUserID))
	}
	if caseID == "" || !isUUID(caseID) {
		return common.WriteError(c, common.FamilyV85, common.BadRequest(habit.MsgInvalidCaseID))
	}
	out, err := d.habitSvc(c).GetHabitTrackerCalendar(c.UserContext(), userID)
	if err != nil {
		return common.WriteError(c, common.FamilyV85, common.Internal(habit.MsgKitTrackerCalUnavail))
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// GetKitTrackerBadges handles GET /kit-tracker-badges and /config/cms/kit-tracker-badges.
//
//	@Summary	Kit tracker badges
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/kit-tracker-badges [get]
//	@Tags		HabitTracker
func (d *Deps) GetKitTrackerBadges(c *fiber.Ctx) error {
	userID, caseID := kitTrackerIdentity(c)
	if userID == "" || !isUUID(userID) {
		return common.WriteError(c, common.FamilyV85, common.BadRequest(habit.MsgInvalidUserID))
	}
	if caseID == "" || !isUUID(caseID) {
		return common.WriteError(c, common.FamilyV85, common.BadRequest(habit.MsgInvalidCaseID))
	}
	out, err := d.habitSvc(c).GetHabitTrackerBadges(c.UserContext(), userID, caseID, nil, "")
	if err != nil {
		return common.WriteError(c, common.FamilyV85, common.Internal(habit.MsgKitTrackerBadgesUnavail))
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

var _ = time.Now
