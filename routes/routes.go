// Package routes mounts every BAH endpoint on the Fiber app.
package routes

import (
	"github.com/gofiber/fiber/v2"

	"traya-bah-service/controllers"
	"traya-bah-service/tenant"
)

// Setup mounts the routes. Public Traya paths keep their api-server shape (no prefix);
// the app-backend paths are mounted alongside for server-to-server callers.
func Setup(app *fiber.App, d *controllers.Deps) {
	app.Use(tenant.Middleware(d.Registry, d.Cfg.DefaultTenant))

	jwt := d.Verifier.RequireJWT()
	v2 := d.Verifier.RequireV2Token()
	internal := d.Verifier.RequireInternal()
	gateway := d.Verifier.RequireGateway()
	admin := d.Verifier.RequireAdmin()
	legacyOnly := tenant.RequireEconomy(tenant.Legacy)
	habitOnly := tenant.RequireEconomy(tenant.Habit)
	logEarnOnly := tenant.RequireEconomy(tenant.LogEarn)

	// ---- Traya legacy economy (public app paths) ----
	app.Get("/streakAndRewardBalance", legacyOnly, jwt, d.GetStreakAndRewardBalance)
	app.Get("/bahLogForGivenDate", legacyOnly, jwt, d.GetBahLogForGivenDate)
	app.Post("/activityLogForBAH", legacyOnly, jwt, d.PostActivityLog)
	app.Put("/multipleActivityLogForBAH", legacyOnly, jwt, d.PutMultipleActivityLog)
	app.Get("/coinTransaction", legacyOnly, jwt, d.GetCoinTransaction)
	app.Get("/bah/:userId/calendar", legacyOnly, jwt, d.GetCalendar)
	app.Get("/medicinesForHowToUse", legacyOnly, jwt, d.GetMedicinesForHowToUse)
	app.Get("/rewardBalance/:caseId", legacyOnly, v2, d.GetRewardBalance)
	app.Get("/latestOrderHowtoUseV2/:caseId", legacyOnly, v2, d.GetLatestOrderHowToUseV2)
	app.Get("/latestRoutineV2/:caseId", legacyOnly, v2, d.GetLatestRoutineV2)

	// ---- Traya CRM ----
	app.Get("/streakMaster", legacyOnly, jwt, d.GetStreakMaster)
	app.Get("/bahHistory/:caseId", legacyOnly, jwt, d.GetBahHistory)
	app.Post("/extraRewardsToUser/:caseId", legacyOnly, jwt, admin, d.PostExtraRewards)
	app.Put("/syncRewardBalanceWithShopFlo/:caseId", legacyOnly, jwt, admin, d.PutSyncRewardBalance)

	// ---- v85 habit tracker (public app paths) ----
	app.Post("/archiveProductForBAH", habitOnly, jwt, d.ArchiveProduct)
	app.Post("/unarchiveProductForBAH", habitOnly, jwt, d.UnarchiveProduct)
	app.Get("/bah/scratch-card", habitOnly, jwt, d.GetScratchCards)
	app.Post("/bah/scratch-card/:id/reveal", habitOnly, jwt, d.RevealScratchCard)
	app.Get("/kit-tracker-page", habitOnly, jwt, d.GetKitTrackerPage)
	app.Get("/kit-tracker-calendar", habitOnly, jwt, d.GetKitTrackerCalendar)
	app.Get("/kit-tracker-badges", habitOnly, jwt, d.GetKitTrackerBadges)

	// ---- v85 habit tracker (app-backend paths, server-to-server) ----
	app.Post("/bah/archive-product", habitOnly, internal, d.ArchiveProduct)
	app.Post("/bah/unarchive-product", habitOnly, internal, d.UnarchiveProduct)
	app.Post("/bah/scratch-card/habit-tracker-mint", habitOnly, internal, d.MintHabitTrackerCredit)
	app.Post("/bah/coin/habit-tracker-credit", habitOnly, internal, d.MintHabitTrackerCredit)
	app.Post("/bah/coin/redeem", habitOnly, internal, d.RedeemCoins)
	app.Get("/config/cms/kit-tracker-page", habitOnly, internal, d.GetKitTrackerPage)
	app.Get("/config/cms/kit-tracker-calendar", habitOnly, internal, d.GetKitTrackerCalendar)
	app.Get("/config/cms/kit-tracker-badges", habitOnly, internal, d.GetKitTrackerBadges)

	// ---- mool / acne Log & Earn ----
	le := app.Group("/consumers/logearn", logEarnOnly, gateway)
	le.Get("/state", d.GetLogEarnState)
	le.Post("/log", d.PostLogEarnLog)
	le.Post("/backfill", d.PostLogEarnBackfill)
	le.Post("/lifeline", d.PostLogEarnLifeline)
	le.Post("/redeem", d.PostLogEarnRedeem)
}
