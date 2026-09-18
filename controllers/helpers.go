package controllers

import (
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func queryValues(c *fiber.Ctx) url.Values {
	out := url.Values{}
	c.Context().QueryArgs().VisitAll(func(k, v []byte) { out.Set(string(k), string(v)) })
	return out
}

// parseFlexibleDate accepts the formats the app sends (RFC3339, YYYY-MM-DD, JS toISOString).
func parseFlexibleDate(raw string) (time.Time, error) {
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z", "2006-01-02"}
	var err error
	for _, l := range layouts {
		var t time.Time
		if t, err = time.Parse(l, raw); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, err
}
