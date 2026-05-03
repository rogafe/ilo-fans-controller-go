package advancedprofiles

import (
	"context"
	"fmt"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"ilo-fans-controller-go/internals/models"
)

func TestEnsureDefaultsInsertsProfilesOnce(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	service := New(db)

	if err := service.EnsureDefaults(context.Background()); err != nil {
		t.Fatalf("EnsureDefaults returned error: %v", err)
	}
	if err := service.EnsureDefaults(context.Background()); err != nil {
		t.Fatalf("second EnsureDefaults returned error: %v", err)
	}

	profiles, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if got, want := len(profiles), 2; got != want {
		t.Fatalf("len(profiles) = %d, want %d", got, want)
	}

	if profiles[0].Name != "Conservative" || profiles[1].Name != "Aggressive" {
		t.Fatalf("unexpected profile order/names: %#v", profiles)
	}
}

func TestSavePreservesBuiltInProfiles(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	service := New(db)

	if err := service.EnsureDefaults(context.Background()); err != nil {
		t.Fatalf("EnsureDefaults returned error: %v", err)
	}

	saved, err := service.Save(context.Background(), []models.AdvancedProfile{
		{
			Name:    "My Profile",
			Warning: "custom",
			CommandBundle: models.AdvancedCommandBundle{
				DisabledSensors: []int{1, 2, 3},
			},
		},
	})
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	if got, want := len(saved), 3; got != want {
		t.Fatalf("len(saved) = %d, want %d", got, want)
	}

	var builtInCount int
	for _, profile := range saved {
		if profile.BuiltIn {
			builtInCount++
		}
	}

	if builtInCount != 2 {
		t.Fatalf("builtInCount = %d, want 2", builtInCount)
	}
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open returned error: %v", err)
	}

	if err := db.AutoMigrate(&models.AdvancedProfileRecord{}); err != nil {
		t.Fatalf("AutoMigrate returned error: %v", err)
	}

	return db
}
