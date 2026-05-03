package advancedprofiles

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"ilo-fans-controller-go/internals/models"
)

type Service interface {
	List(context.Context) ([]models.AdvancedProfile, error)
	Save(context.Context, []models.AdvancedProfile) ([]models.AdvancedProfile, error)
	EnsureDefaults(context.Context) error
	GetByName(context.Context, string) (models.AdvancedProfile, error)
}

type service struct {
	db *gorm.DB
}

func New(db *gorm.DB) Service {
	return &service{db: db}
}

func (s *service) EnsureDefaults(ctx context.Context) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.AdvancedProfileRecord{}).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	records := make([]models.AdvancedProfileRecord, 0, 2)
	for _, profile := range defaultProfiles() {
		records = append(records, models.AdvancedProfileRecord{
			Name:          profile.Name,
			Warning:       profile.Warning,
			BuiltIn:       profile.BuiltIn,
			CommandBundle: profile.CommandBundle,
		})
	}

	return s.db.WithContext(ctx).Create(&records).Error
}

func (s *service) List(ctx context.Context) ([]models.AdvancedProfile, error) {
	var records []models.AdvancedProfileRecord
	if err := s.db.WithContext(ctx).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}

	profiles := make([]models.AdvancedProfile, 0, len(records))
	for _, record := range records {
		profiles = append(profiles, models.AdvancedProfile{
			Name:          record.Name,
			CommandBundle: record.CommandBundle,
			Warning:       record.Warning,
			BuiltIn:       record.BuiltIn,
		})
	}

	return profiles, nil
}

func (s *service) Save(ctx context.Context, profiles []models.AdvancedProfile) ([]models.AdvancedProfile, error) {
	records := make([]models.AdvancedProfileRecord, 0, len(profiles))
	for _, profile := range profiles {
		if profile.BuiltIn {
			continue
		}

		records = append(records, models.AdvancedProfileRecord{
			Name:          profile.Name,
			Warning:       profile.Warning,
			BuiltIn:       profile.BuiltIn,
			CommandBundle: profile.CommandBundle,
		})
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("built_in = ?", false).Delete(&models.AdvancedProfileRecord{}).Error; err != nil {
			return err
		}

		for _, record := range records {
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.List(ctx)
}

func (s *service) GetByName(ctx context.Context, name string) (models.AdvancedProfile, error) {
	var record models.AdvancedProfileRecord
	if err := s.db.WithContext(ctx).Where("name = ?", name).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.AdvancedProfile{}, err
		}

		return models.AdvancedProfile{}, err
	}

	return models.AdvancedProfile{
		Name:          record.Name,
		CommandBundle: record.CommandBundle,
		Warning:       record.Warning,
		BuiltIn:       record.BuiltIn,
	}, nil
}

func defaultProfiles() []models.AdvancedProfile {
	return []models.AdvancedProfile{
		{
			Name:    "Conservative",
			Warning: "Reduces thermal safeguards by lowering fan minimums and disabling a subset of sensors. Monitor temperatures closely after applying. Settings are not persistent across iLO or server reboots.",
			BuiltIn: true,
			CommandBundle: models.AdvancedCommandBundle{
				FanMinimums: fanBounds([]int{1, 2, 3, 4, 5, 6}, 12),
				PIDLows: []models.PIDCommand{{
					Sensors: []int{33, 34, 35, 36, 37, 38, 42, 47, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63},
					Value:   3100,
				}},
				PIDHighs: []models.PIDCommand{{
					Sensors: []int{53, 55, 57, 61, 63},
					Value:   3100,
				}},
				OCSD: []models.OCSDCommand{{
					Sensors: []int{24, 26, 27, 28, 29, 30, 31, 32, 44},
					Value:   2,
				}},
				DisabledSensors: []int{32, 45, 31, 41, 37, 38, 29, 34, 35, 30, 40, 36, 28, 33, 27},
			},
		},
		{
			Name:    "Aggressive",
			Warning: "High-risk profile. Disables sensors 0-80, lowers fan minimums to 8%, and caps fans at 50%. Use only if you understand the thermal risk and will monitor the server closely. Settings are not persistent across iLO or server reboots.",
			BuiltIn: true,
			CommandBundle: models.AdvancedCommandBundle{
				FanMinimums: fanBounds([]int{1, 2, 3, 4, 5, 6}, 8),
				FanMaximums: fanBounds([]int{1, 2, 3, 4, 5, 6}, 50),
				PIDLows: []models.PIDCommand{{
					Sensors: []int{33, 34, 35, 36, 37, 38, 42, 47, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63},
					Value:   2500,
				}},
				PIDHighs: []models.PIDCommand{{
					Sensors: []int{53, 55, 57, 61, 63},
					Value:   2500,
				}},
				OCSD: []models.OCSDCommand{{
					Sensors: expandRange(0, 45),
					Value:   2,
				}},
				DisabledSensors: expandRange(0, 80),
			},
		},
	}
}

func fanBounds(fans []int, value int) []models.FanBoundCommand {
	commands := make([]models.FanBoundCommand, 0, len(fans))
	for _, fan := range fans {
		commands = append(commands, models.FanBoundCommand{
			Fan:   fan,
			Value: value,
		})
	}

	return commands
}

func expandRange(start, end int) []int {
	if end < start {
		return nil
	}

	values := make([]int, 0, end-start+1)
	for value := start; value <= end; value++ {
		values = append(values, value)
	}

	return values
}
