package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"

	"ilo-fans-controller-go/internals/config"
	"ilo-fans-controller-go/internals/console"
	"ilo-fans-controller-go/internals/models"
	"ilo-fans-controller-go/internals/services/ilo"
	"ilo-fans-controller-go/internals/services/presets"
)

type Handler struct {
	cfg           config.Config
	hub           *console.Hub
	iloService    ilo.Service
	presetService presets.Service
}

func New(cfg config.Config, hub *console.Hub, iloService ilo.Service, presetService presets.Service) *Handler {
	return &Handler{
		cfg:           cfg,
		hub:           hub,
		iloService:    iloService,
		presetService: presetService,
	}
}

func (h *Handler) GetIndex(c *fiber.Ctx) error {
	status := (*models.StatusMessage)(nil)
	fans := []models.Fan{}
	presets := []models.Preset{}

	if h.cfg.HasILOConfig() {
		loadedFans, err := h.iloService.GetFans(c.UserContext())
		if err != nil {
			log.Printf("unable to fetch fans for page render: %v", err)
			status = &models.StatusMessage{Type: "error", Message: "Unable to connect to iLO right now. The page is available, but live fan controls are temporarily unavailable."}
		} else {
			fans = loadedFans
		}
	} else {
		status = &models.StatusMessage{Type: "error", Message: "iLO credentials are not configured yet. Set ILO_HOST, ILO_USERNAME, and ILO_PASSWORD to enable live fan control."}
	}

	loadedPresets, err := h.presetService.List(c.UserContext())
	if err != nil {
		log.Printf("unable to fetch presets for page render: %v", err)
		if status == nil {
			status = &models.StatusMessage{Type: "error", Message: "Unable to load presets from Postgres."}
		}
	} else {
		presets = loadedPresets
	}

	return c.Render("index", fiber.Map{
		"InitialFansJSON":    mustJSON(fans),
		"InitialPresetsJSON": mustJSON(presets),
		"InitialStatusJSON":  mustJSON(status),
		"MinimumFanSpeed":    h.cfg.MinimumFanSpeed,
		"PageTitle":          "iLO Fans Controller",
	})
}

func (h *Handler) GetFans(c *fiber.Ctx) error {
	if !h.cfg.HasILOConfig() {
		return writeJSONError(c, fiber.StatusBadRequest, "iLO credentials are not configured")
	}

	fans, err := h.iloService.GetFans(c.UserContext())
	if err != nil {
		log.Printf("unable to fetch fans: %v", err)
		return writeJSONError(c, statusForILOError(err), "Unable to fetch fans from iLO")
	}

	return c.JSON(fans)
}

func (h *Handler) SetFans(c *fiber.Ctx) error {
	if !h.cfg.HasILOConfig() {
		return writeJSONError(c, fiber.StatusBadRequest, "iLO credentials are not configured")
	}

	var request models.SetFansRequest
	if err := c.BodyParser(&request); err != nil {
		return writeJSONError(c, fiber.StatusBadRequest, "Invalid JSON payload")
	}

	if strings.TrimSpace(request.ClientID) == "" {
		request.ClientID = strings.TrimSpace(c.Get("X-Console-Client-Id"))
	}

	fans, err := h.iloService.SetFans(c.UserContext(), request)
	if err != nil {
		log.Printf("unable to set fans: %v", err)
		h.hub.Send(request.ClientID, "error", err.Error())
		return writeJSONError(c, statusForILOError(err), err.Error())
	}

	return c.JSON(fans)
}

func (h *Handler) ConsoleWebSocket(conn *websocket.Conn) {
	clientID := strings.TrimSpace(conn.Query("client_id"))
	if clientID == "" {
		_ = conn.WriteJSON(console.Event{Type: "error", Message: "client_id is required"})
		_ = conn.Close()
		return
	}

	h.hub.Register(clientID, conn)
	defer h.hub.Unregister(clientID, conn)
	defer conn.Close()

	h.hub.Send(clientID, "connected", "Console connected")

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *Handler) GetPresets(c *fiber.Ctx) error {
	presets, err := h.presetService.List(c.UserContext())
	if err != nil {
		log.Printf("unable to fetch presets: %v", err)
		return writeJSONError(c, fiber.StatusInternalServerError, "Unable to load presets")
	}

	return c.JSON(presets)
}

func (h *Handler) SavePresets(c *fiber.Ctx) error {
	var presets []models.Preset
	if err := c.BodyParser(&presets); err != nil {
		return writeJSONError(c, fiber.StatusBadRequest, "Invalid presets payload")
	}

	for _, preset := range presets {
		if strings.TrimSpace(preset.Name) == "" {
			return writeJSONError(c, fiber.StatusBadRequest, "Preset name is required")
		}

		if len(preset.Speeds) == 0 {
			return writeJSONError(c, fiber.StatusBadRequest, "Preset speeds are required")
		}

		for _, speed := range preset.Speeds {
			if speed < h.cfg.MinimumFanSpeed || speed > 100 {
				return writeJSONError(c, fiber.StatusBadRequest, "Preset speeds must respect the configured speed range")
			}
		}
	}

	savedPresets, err := h.presetService.Save(c.UserContext(), presets)
	if err != nil {
		log.Printf("unable to save presets: %v", err)
		return writeJSONError(c, fiber.StatusInternalServerError, "Unable to save presets")
	}

	return c.JSON(savedPresets)
}

func mustJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "null"
	}

	return string(encoded)
}

func writeJSONError(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(fiber.Map{"error": message})
}

func statusForILOError(err error) int {
	if errors.Is(err, net.ErrClosed) {
		return fiber.StatusGatewayTimeout
	}

	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "timed out"):
		return fiber.StatusGatewayTimeout
	case strings.Contains(message, "unknown fan name"), strings.Contains(message, "speed must be"), strings.Contains(message, "request must include"):
		return fiber.StatusBadRequest
	default:
		return fiber.StatusBadGateway
	}
}
