package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"

	"ilo-fans-controller-go/internals/handlers"
)

func Register(app *fiber.App, handler *handlers.Handler) {
	app.Use("/ws/console", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})
	app.Get("/ws/console", websocket.New(handler.ConsoleWebSocket))
	app.Get("/", handler.GetIndex)
	app.Get("/api/fans", handler.GetFans)
	app.Get("/api/temperatures", handler.GetTemperatures)
	app.Post("/api/fans", handler.SetFans)
	app.Get("/api/presets", handler.GetPresets)
	app.Post("/api/presets", handler.SavePresets)
}
