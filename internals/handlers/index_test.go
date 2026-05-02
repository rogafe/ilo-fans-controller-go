package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"ilo-fans-controller-go/internals/config"
	"ilo-fans-controller-go/internals/console"
	"ilo-fans-controller-go/internals/models"
	"ilo-fans-controller-go/internals/services/advancedprofiles"
	"ilo-fans-controller-go/internals/services/ilo"
	"ilo-fans-controller-go/internals/services/presets"
)

func TestApplyAdvancedProfileUnknownProfileReturnsBadRequest(t *testing.T) {
	t.Parallel()

	handler := New(
		config.Config{ILOHost: "ilo", ILOUsername: "user", ILOPassword: "pass"},
		console.NewHub(),
		stubILOService{},
		stubPresetService{},
		stubAdvancedProfilesService{getByNameErr: gorm.ErrRecordNotFound},
	)

	app := fiber.New()
	app.Post("/api/advanced-profiles/apply", handler.ApplyAdvancedProfile)

	req := httptest.NewRequest("POST", "/api/advanced-profiles/apply", strings.NewReader(`{"profileName":"Missing","confirmation":"APPLY ADVANCED PROFILE"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test returned error: %v", err)
	}

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}
}

func TestApplyAdvancedProfileOfflineReturnsServiceUnavailable(t *testing.T) {
	t.Parallel()

	handler := New(
		config.Config{ILOHost: "ilo", ILOUsername: "user", ILOPassword: "pass"},
		console.NewHub(),
		stubILOService{applyErr: errors.New("connection refused")},
		stubPresetService{},
		stubAdvancedProfilesService{
			profile: models.AdvancedProfile{Name: "Aggressive"},
		},
	)

	app := fiber.New()
	app.Post("/api/advanced-profiles/apply", handler.ApplyAdvancedProfile)

	req := httptest.NewRequest("POST", "/api/advanced-profiles/apply", strings.NewReader(`{"profileName":"Aggressive","confirmation":"APPLY ADVANCED PROFILE"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test returned error: %v", err)
	}

	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusServiceUnavailable)
	}
}

type stubILOService struct {
	applyErr error
}

func (s stubILOService) GetFans(context.Context) ([]models.Fan, error) {
	return nil, nil
}

func (s stubILOService) GetTemperatures(context.Context) ([]models.Temperature, error) {
	return nil, nil
}

func (s stubILOService) SetFans(context.Context, models.SetFansRequest) ([]models.Fan, error) {
	return nil, nil
}

func (s stubILOService) ApplyAdvancedProfile(context.Context, models.ApplyAdvancedProfileRequest, models.AdvancedProfile) error {
	return s.applyErr
}

type stubPresetService struct{}

func (stubPresetService) List(context.Context) ([]models.Preset, error) {
	return nil, nil
}

func (stubPresetService) Save(context.Context, []models.Preset) ([]models.Preset, error) {
	return nil, nil
}

func (stubPresetService) EnsureDefaults(context.Context) error {
	return nil
}

type stubAdvancedProfilesService struct {
	profile      models.AdvancedProfile
	getByNameErr error
}

func (s stubAdvancedProfilesService) List(context.Context) ([]models.AdvancedProfile, error) {
	return []models.AdvancedProfile{s.profile}, nil
}

func (s stubAdvancedProfilesService) Save(context.Context, []models.AdvancedProfile) ([]models.AdvancedProfile, error) {
	return nil, nil
}

func (s stubAdvancedProfilesService) EnsureDefaults(context.Context) error {
	return nil
}

func (s stubAdvancedProfilesService) GetByName(context.Context, string) (models.AdvancedProfile, error) {
	if s.getByNameErr != nil {
		return models.AdvancedProfile{}, s.getByNameErr
	}

	return s.profile, nil
}

var (
	_ ilo.Service              = stubILOService{}
	_ presets.Service          = stubPresetService{}
	_ advancedprofiles.Service = stubAdvancedProfilesService{}
)

func TestApplyAdvancedProfileResponseBody(t *testing.T) {
	t.Parallel()

	handler := New(
		config.Config{ILOHost: "ilo", ILOUsername: "user", ILOPassword: "pass"},
		console.NewHub(),
		stubILOService{},
		stubPresetService{},
		stubAdvancedProfilesService{profile: models.AdvancedProfile{Name: "Aggressive"}},
	)

	app := fiber.New()
	app.Post("/api/advanced-profiles/apply", handler.ApplyAdvancedProfile)

	req := httptest.NewRequest("POST", "/api/advanced-profiles/apply", strings.NewReader(`{"profileName":"Aggressive","confirmation":"APPLY ADVANCED PROFILE"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test returned error: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}

	var payload map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("json decode failed: %v", err)
	}
	if !payload["ok"] {
		t.Fatalf("payload ok = %v, want true", payload["ok"])
	}
}
