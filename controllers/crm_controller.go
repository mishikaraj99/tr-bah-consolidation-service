package controllers

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"traya-bah-service/internal/common"
)

// GetStreakMaster handles GET /streakMaster.
//
//	@Summary	Superadmin streak masters
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/streakMaster [get]
//	@Tags		CRM
func (d *Deps) GetStreakMaster(c *fiber.Ctx) error {
	out, err := d.legacySvc(c).GetMasterStreakDataForExtraBonusStreak(c.UserContext())
	if err != nil {
		return common.WriteError(c, common.FamilyLegacyErr, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// GetBahHistory handles GET /bahHistory/:caseId.
//
//	@Summary	CRM agent BAH history
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/bahHistory/{caseId} [get]
//	@Tags		CRM
func (d *Deps) GetBahHistory(c *fiber.Ctx) error {
	id := identity(c)
	out, err := d.legacySvc(c).GetBahHistoryOfUser(c.UserContext(), c.Params("caseId"),
		c.Query("isCreditTxn") != "", c.Query("isDebitTxn") != "",
		optionalIntQuery(c, "year"), optionalIntQuery(c, "month"), id.Email)
	if err != nil {
		return common.WriteError(c, common.FamilyLegacyErr, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// PostExtraRewards handles POST /extraRewardsToUser/:caseId.
//
//	@Summary	Manual coin grant
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/extraRewardsToUser/{caseId} [post]
//	@Tags		CRM
func (d *Deps) PostExtraRewards(c *fiber.Ctx) error {
	var body struct {
		StreakMasterID string `json:"streakMasterId"`
		Reason         string `json:"reason"`
		CustomAmount   *int   `json:"customAmount"`
	}
	if err := c.BodyParser(&body); err != nil {
		return common.WriteError(c, common.FamilyLegacyErr, common.BadRequest("Invalid request body"))
	}
	out, err := d.legacySvc(c).SaveExtraBonusForUsersForAnyReason(c.UserContext(), c.Params("caseId"),
		strings.TrimSpace(body.StreakMasterID), strings.TrimSpace(body.Reason), body.CustomAmount)
	if err != nil {
		return common.WriteError(c, common.FamilyLegacyErr, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// PutSyncRewardBalance handles PUT /syncRewardBalanceWithShopFlo/:caseId.
//
//	@Summary	Reconcile the local balance against the Shopflo wallet
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/syncRewardBalanceWithShopFlo/{caseId} [put]
//	@Tags		CRM
func (d *Deps) PutSyncRewardBalance(c *fiber.Ctx) error {
	out, err := d.legacySvc(c).SyncRewardBalanceWithShopflo(c.UserContext(), c.Params("caseId"))
	if err != nil {
		return common.WriteError(c, common.FamilyLegacyErr, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}
