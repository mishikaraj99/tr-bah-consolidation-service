package common

import (
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Family selects the error envelope a route family uses.
type Family int

const (
	// FamilyLegacyErr: api-server `res.status(500).json({ err: e })`.
	FamilyLegacyErr Family = iota
	// FamilyLegacyMessage: `res.status(err.status).json({ message })` (400 validation, 500 else).
	FamilyLegacyMessage
	// FamilyV85: `res.status(status).json({ message })`.
	FamilyV85
	// FamilyCalendar: `400 {error}` or `500 {error:'Internal Server Error', details}`.
	FamilyCalendar
	// FamilyCoinTxn: `500 {err: message}`.
	FamilyCoinTxn
	// FamilyProxy: upstream status with `{err: body}`.
	FamilyProxy
	// FamilyLogEarn: `{message, statusCode, timestamp, path}`.
	FamilyLogEarn
	// FamilyPlain: `{message}` with status (tenant/auth middleware).
	FamilyPlain
)

// WriteError renders err for the family.
func WriteError(c *fiber.Ctx, fam Family, err error) error {
	status := StatusOf(err)
	msg := MessageOf(err)
	he, _ := err.(*HTTPError)
	switch fam {
	case FamilyLegacyErr:
		if he != nil && he.Status != http.StatusInternalServerError {
			// api-server serialises the {status,message} factory object under err
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"err": fiber.Map{"status": he.Status, "message": he.Message}})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"err": fiber.Map{"message": msg}})
	case FamilyLegacyMessage, FamilyV85, FamilyPlain:
		return c.Status(status).JSON(fiber.Map{"message": msg})
	case FamilyCalendar:
		if status == http.StatusBadRequest {
			return c.Status(status).JSON(fiber.Map{"error": msg})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Internal Server Error", "details": msg})
	case FamilyCoinTxn:
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"err": msg})
	case FamilyProxy:
		if he != nil && he.Body != nil {
			return c.Status(status).JSON(fiber.Map{"err": he.Body})
		}
		return c.Status(status).JSON(fiber.Map{"err": msg})
	case FamilyLogEarn:
		return c.Status(status).JSON(fiber.Map{
			"message": msg, "statusCode": status, "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "path": c.OriginalURL(),
		})
	}
	return c.Status(status).JSON(fiber.Map{"message": msg})
}

// WriteJSON writes v with status (0 → 200).
func WriteJSON(c *fiber.Ctx, status int, v any) error {
	if status == 0 {
		status = http.StatusOK
	}
	return c.Status(status).JSON(v)
}
