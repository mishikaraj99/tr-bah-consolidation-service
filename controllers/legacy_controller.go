package controllers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/legacy"
)

// GetStreakAndRewardBalance handles GET /streakAndRewardBalance.
//
//	@Summary	Streak and reward balance
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/streakAndRewardBalance [get]
//	@Tags		BAH
func (d *Deps) GetStreakAndRewardBalance(c *fiber.Ctx) error {
	id := identity(c)
	version := id.AppVersion
	out, err := d.legacySvc(c).GetStreakAndRewardBalance(c.UserContext(), id.UserID, version, id.RawToken)
	if err != nil {
		return common.WriteError(c, common.FamilyLegacyErr, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// GetBahLogForGivenDate handles GET /bahLogForGivenDate.
//
//	@Summary	Dose list for a date
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/bahLogForGivenDate [get]
//	@Tags		BAH
func (d *Deps) GetBahLogForGivenDate(c *fiber.Ctx) error {
	id := identity(c)
	var date *time.Time
	if raw := strings.TrimSpace(c.Query("date")); raw != "" {
		if t, err := parseFlexibleDate(raw); err == nil {
			date = &t
		}
	}
	version := id.AppVersion
	out, err := d.legacySvc(c).GetBahLogForGivenDate(c.UserContext(), date, id.UserID, version >= legacy.VersionGateBahV3, id.CaseID, version)
	if err != nil {
		return common.WriteError(c, common.FamilyLegacyErr, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// PostActivityLog handles POST /activityLogForBAH.
//
//	@Summary	Log the day's routine
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/activityLogForBAH [post]
//	@Tags		BAH
func (d *Deps) PostActivityLog(c *fiber.Ctx) error {
	var body map[string]any
	if err := c.BodyParser(&body); err != nil {
		return common.WriteError(c, common.FamilyLegacyMessage, common.BadRequest("Invalid request body"))
	}
	if err := common.ValidateLogActivity(body); err != nil {
		return common.WriteError(c, common.FamilyLegacyMessage, err)
	}
	in := legacy.LogActivityInput{UserID: identity(c).UserID}
	in.IsLogForToday, _ = body["isLogForToday"].(bool)
	in.IsHabitTracker, _ = body["isHabitTracker"].(bool)
	if raw, ok := body["productPrescriptions"].([]any); ok {
		for _, item := range raw {
			if m, ok := item.(map[string]any); ok {
				in.ProductPrescriptions = append(in.ProductPrescriptions, m)
			}
		}
	}
	out, err := d.legacySvc(c).SaveActivityLogsAndCreateStreakAndGiveRewards(c.UserContext(), in)
	if err != nil {
		return common.WriteError(c, common.FamilyLegacyMessage, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// PutMultipleActivityLog handles PUT /multipleActivityLogForBAH.
//
//	@Summary	Flip check-ins for several products
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/multipleActivityLogForBAH [put]
//	@Tags		BAH
func (d *Deps) PutMultipleActivityLog(c *fiber.Ctx) error {
	var body map[string]any
	if err := c.BodyParser(&body); err != nil {
		return common.WriteError(c, common.FamilyLegacyMessage, common.BadRequest("Invalid request body"))
	}
	in := legacy.MultipleLogInput{UserID: identity(c).UserID}
	in.IsLogForToday, _ = body["isLogForToday"].(bool)
	raw, _ := body["logProductDetail"].([]any)
	if len(raw) > 0 {
		if err := common.ValidateMultipleActivityLog(body); err != nil {
			return common.WriteError(c, common.FamilyLegacyMessage, err)
		}
		for _, item := range raw {
			if m, ok := item.(map[string]any); ok {
				in.LogProductDetail = append(in.LogProductDetail, m)
			}
		}
	}
	out, err := d.legacySvc(c).UpdateMultipleMedicineLogForUser(c.UserContext(), in)
	if err != nil {
		return common.WriteError(c, common.FamilyLegacyMessage, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// GetCoinTransaction handles GET /coinTransaction.
//
//	@Summary	Paginated coin ledger
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/coinTransaction [get]
//	@Tags		BAH
func (d *Deps) GetCoinTransaction(c *fiber.Ctx) error {
	out, err := d.legacySvc(c).GetRewardCoinHistoryPaginated(c.UserContext(), identity(c).UserID,
		queryInt(c, "page", 1), queryInt(c, "limit", 6))
	if err != nil {
		return common.WriteError(c, common.FamilyCoinTxn, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// GetCalendar handles GET /bah/:userId/calendar. The path userId is ignored; the token identity wins.
//
//	@Summary	Logging calendar
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/bah/{userId}/calendar [get]
//	@Tags		BAH
func (d *Deps) GetCalendar(c *fiber.Ctx) error {
	date := strings.TrimSpace(c.Query("date"))
	if date == "" {
		return common.WriteError(c, common.FamilyCalendar, common.BadRequest("date query parameter is required"))
	}
	mode := strings.TrimSpace(c.Query("mode"))
	if mode == "" {
		mode = "calendar"
	}
	out, err := d.legacySvc(c).GetBahCalendarLogData(c.UserContext(), identity(c).UserID, date, mode)
	if err != nil {
		return common.WriteError(c, common.FamilyCalendar, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// GetRewardBalance handles GET /rewardBalance/:caseId and /rewardBalance/:customerId.
//
//	@Summary	Reward balance for a case or user
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/rewardBalance/{caseId} [get]
//	@Tags		BAH
func (d *Deps) GetRewardBalance(c *fiber.Ctx) error {
	id := strings.TrimSpace(c.Params("caseId"))
	balance, err := d.legacySvc(c).GetUserRewardBalance(c.UserContext(), id)
	if err != nil {
		return common.WriteError(c, common.FamilyV85, err)
	}
	return common.WriteJSON(c, http.StatusOK, fiber.Map{"rewardBalance": balance})
}

// GetMedicinesForHowToUse handles GET /medicinesForHowToUse.
//
//	@Summary	How-to-use medicine list
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/medicinesForHowToUse [get]
//	@Tags		BAH
func (d *Deps) GetMedicinesForHowToUse(c *fiber.Ctx) error {
	out, err := d.legacySvc(c).GetMedicinesForHowToUsePurpose(c.UserContext(), identity(c).UserID)
	if err != nil {
		return common.WriteError(c, common.FamilyLegacyErr, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

func (d *Deps) writeProxy(c *fiber.Ctx, res legacy.ProxyResult, err error) error {
	if err != nil {
		return common.WriteError(c, common.FamilyProxy, err)
	}
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	return c.Status(res.Status).Send(res.Body)
}

// GetLatestOrderHowToUseV2 handles GET /latestOrderHowtoUseV2/:caseId.
//
//	@Summary	How-to-use v2 (recommendation service)
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/latestOrderHowtoUseV2/{caseId} [get]
//	@Tags		BAH
func (d *Deps) GetLatestOrderHowToUseV2(c *fiber.Ctx) error {
	caseID := c.Params("caseId")
	if !isUUID(caseID) {
		return common.WriteError(c, common.FamilyProxy, common.BadRequest("Invalid caseId"))
	}
	res, err := d.legacySvc(c).LatestOrderHowToUseV2(c.UserContext(), caseID, queryValues(c))
	return d.writeProxy(c, res, err)
}

// GetLatestRoutineV2 handles GET /latestRoutineV2/:caseId.
//
//	@Summary	Routine v2 (recommendation service)
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/latestRoutineV2/{caseId} [get]
//	@Tags		BAH
func (d *Deps) GetLatestRoutineV2(c *fiber.Ctx) error {
	caseID := c.Params("caseId")
	if !isUUID(caseID) {
		return common.WriteError(c, common.FamilyProxy, common.BadRequest("Invalid caseId"))
	}
	res, err := d.legacySvc(c).LatestRoutineV2(c.UserContext(), caseID, queryValues(c))
	return d.writeProxy(c, res, err)
}
