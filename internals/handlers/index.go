package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"

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
	temperatures := []models.Temperature{}
	presets := []models.Preset{}

	if h.cfg.HasILOConfig() {
		loadedFans, err := h.iloService.GetFans(c.UserContext())
		if err != nil {
			log.Printf("unable to fetch fans for page render: %v", err)
			if isILOServerOff(err) {
				status = &models.StatusMessage{Type: "offline", Message: "iLO is unreachable (server completely off / network unreachable). Entering waiting failsafe mode: the UI will keep retrying until iLO is back."}
			} else {
				status = &models.StatusMessage{Type: "error", Message: "Unable to connect to iLO right now. The page is available, but live fan controls are temporarily unavailable."}
			}
		} else {
			fans = loadedFans
		}
	} else {
		status = &models.StatusMessage{Type: "error", Message: "iLO credentials are not configured yet. Set ILO_HOST, ILO_USERNAME, and ILO_PASSWORD to enable live fan control."}
	}

	if h.cfg.HasILOSNMPConfig() {
		loadedTemperatures, err := h.iloService.GetTemperatures(c.UserContext())
		if err != nil {
			log.Printf("unable to fetch temperatures for page render: %v", err)
			if status == nil {
				status = &models.StatusMessage{Type: "error", Message: "Unable to load SNMP temperatures right now."}
			}
		} else {
			temperatures = loadedTemperatures
		}
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
		"InitialFansJSON":         mustJSON(fans),
		"InitialTemperaturesJSON": mustJSON(temperatures),
		"InitialPresetsJSON":      mustJSON(presets),
		"InitialStatusJSON":       mustJSON(status),
		"MinimumFanSpeed":         h.cfg.MinimumFanSpeed,
		"PageTitle":               "iLO Fans Controller",
	})
}

func (h *Handler) GetFans(c *fiber.Ctx) error {
	if !h.cfg.HasILOConfig() {
		return writeJSONError(c, fiber.StatusBadRequest, "iLO credentials are not configured")
	}

	fans, err := h.iloService.GetFans(c.UserContext())
	if err != nil {
		log.Printf("unable to fetch fans: %v", err)
		if isILOServerOff(err) {
			c.Set("Retry-After", "5")
			return writeJSONError(c, fiber.StatusServiceUnavailable, "iLO is unreachable (server completely off). Waiting for it to come back.")
		}
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
		if isILOServerOff(err) {
			c.Set("Retry-After", "5")
			return writeJSONError(c, fiber.StatusServiceUnavailable, "iLO is unreachable (server completely off). Fan commands were not sent.")
		}
		return writeJSONError(c, statusForILOError(err), err.Error())
	}

	return c.JSON(fans)
}

func (h *Handler) GetTemperatures(c *fiber.Ctx) error {
	if !h.cfg.HasILOSNMPConfig() {
		return writeJSONError(c, fiber.StatusBadRequest, "iLO SNMP is not configured")
	}

	temperatures, err := h.iloService.GetTemperatures(c.UserContext())
	if err != nil {
		log.Printf("unable to fetch temperatures: %v", err)
		return writeJSONError(c, fiber.StatusBadGateway, "Unable to fetch temperatures from iLO SNMP")
	}

	return c.JSON(temperatures)
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

	if isILOServerOff(err) {
		return fiber.StatusServiceUnavailable
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

func isILOServerOff(err error) bool {
	if err == nil {
		return false
	}

	// Unwrap common wrapper errors.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		err = urlErr.Err
	}

	// net/http dial errors typically end up as net.OpError -> os.SyscallError -> syscall.Errno
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Timeout() {
			return true
		}
		if opErr.Err != nil {
			err = opErr.Err
		}
	}

	var syscallErr *os.SyscallError
	if errors.As(err, &syscallErr) {
		if errno, ok := syscallErr.Err.(syscall.Errno); ok {
			switch errno {
			case syscall.EHOSTUNREACH, syscall.ENETUNREACH, syscall.ECONNREFUSED, syscall.ETIMEDOUT:
				return true
			}
		}
	}

	// Fallback string matching (covers some platform-dependent variants).
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no route to host") ||
		strings.Contains(message, "network is unreachable") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "i/o timeout") ||
		strings.Contains(message, "timed out")
}
