package controllers

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"traya-bah-service/internal/common"
)

func customerID(c *fiber.Ctx) string { return identity(c).UserID }

func optionalOrderID(c *fiber.Ctx) *string {
	if v := strings.TrimSpace(c.Query("orderId")); v != "" {
		return &v
	}
	return nil
}

// GetLogEarnState handles GET /consumers/logearn/state.
//
//	@Summary	Log & Earn state
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/consumers/logearn/state [get]
//	@Tags		LogEarn
func (d *Deps) GetLogEarnState(c *fiber.Ctx) error {
	out, err := d.logearnSvc(c).GetState(c.UserContext(), customerID(c))
	if err != nil {
		return common.WriteError(c, common.FamilyLogEarn, err)
	}
	return common.WriteJSON(c, http.StatusOK, out)
}

// PostLogEarnLog handles POST /consumers/logearn/log.
//
//	@Summary	Log today's dose
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/consumers/logearn/log [post]
//	@Tags		LogEarn
func (d *Deps) PostLogEarnLog(c *fiber.Ctx) error {
	out, err := d.logearnSvc(c).LogToday(c.UserContext(), customerID(c), optionalOrderID(c))
	if err != nil {
		return common.WriteError(c, common.FamilyLogEarn, err)
	}
	return common.WriteJSON(c, http.StatusCreated, out)
}

// PostLogEarnBackfill handles POST /consumers/logearn/backfill.
//
//	@Summary	Backfill yesterday's dose
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/consumers/logearn/backfill [post]
//	@Tags		LogEarn
func (d *Deps) PostLogEarnBackfill(c *fiber.Ctx) error {
	out, err := d.logearnSvc(c).BackfillYesterday(c.UserContext(), customerID(c), optionalOrderID(c))
	if err != nil {
		return common.WriteError(c, common.FamilyLogEarn, err)
	}
	return common.WriteJSON(c, http.StatusCreated, out)
}

// PostLogEarnLifeline handles POST /consumers/logearn/lifeline.
//
//	@Summary	Apply the kit's lifeline
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/consumers/logearn/lifeline [post]
//	@Tags		LogEarn
func (d *Deps) PostLogEarnLifeline(c *fiber.Ctx) error {
	out, err := d.logearnSvc(c).UseLifeline(c.UserContext(), customerID(c))
	if err != nil {
		return common.WriteError(c, common.FamilyLogEarn, err)
	}
	return common.WriteJSON(c, http.StatusCreated, out)
}

// PostLogEarnRedeem handles POST /consumers/logearn/redeem.
//
//	@Summary	Redeem cash against an order subtotal
//	@Param		x-tenant-id	header	string	true	"Tenant ID"
//	@Router		/consumers/logearn/redeem [post]
//	@Tags		LogEarn
func (d *Deps) PostLogEarnRedeem(c *fiber.Ctx) error {
	var body struct {
		Subtotal int     `json:"subtotal"`
		Amount   *int    `json:"amount"`
		OrderID  *string `json:"orderId"`
	}
	if err := c.BodyParser(&body); err != nil {
		return common.WriteError(c, common.FamilyLogEarn, common.BadRequest("Invalid request body"))
	}
	if body.Subtotal < 1 {
		return common.WriteError(c, common.FamilyLogEarn, common.BadRequest("subtotal must not be less than 1"))
	}
	if body.Amount != nil && *body.Amount < 1 {
		return common.WriteError(c, common.FamilyLogEarn, common.BadRequest("amount must not be less than 1"))
	}
	if body.OrderID != nil && *body.OrderID != "" && !isUUID(*body.OrderID) {
		return common.WriteError(c, common.FamilyLogEarn, common.BadRequest("orderId must be a UUID"))
	}
	out, err := d.logearnSvc(c).Redeem(c.UserContext(), customerID(c), body.Subtotal, body.Amount, body.OrderID)
	if err != nil {
		return common.WriteError(c, common.FamilyLogEarn, err)
	}
	return common.WriteJSON(c, http.StatusCreated, out)
}
