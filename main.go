package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
)

// @title       Traya BAH Service
// @version     1.0
// @description Multi-tenant Build-A-Habit service (traya legacy + v85, mool/acne Log & Earn)
func main() {
	time.Local = time.UTC
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(recover.New())
	app.Use(requestid.New(requestid.Config{Header: "x-trace-id"}))
	app.Get("/health", func(c *fiber.Ctx) error { return c.SendString("OK") })

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	go func() {
		if err := app.Listen(":" + port); err != nil {
			slog.Error("listen failed", "error", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = app.ShutdownWithContext(ctx)
}
