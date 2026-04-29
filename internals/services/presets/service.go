package presets

import (
	"context"

	"gorm.io/gorm"

	"ilo-fans-controller-go/internals/models"
)

type Service interface {
	List(context.Context) ([]models.Preset, error)
	Save(context.Context, []models.Preset) ([]models.Preset, error)
	EnsureDefaults(context.Context) error
}

type service struct {
	db *gorm.DB
}

func New(db *gorm.DB) Service {
	return &service{db: db}
}

func (s *service) EnsureDefaults(ctx context.Context) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.PresetRecord{}).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	_, err := s.Save(ctx, []models.Preset{
		{Name: "Silent Mode", Speeds: []int{15}},
		{Name: "Normal Mode", Speeds: []int{50}},
		{Name: "Turbo Mode", Speeds: []int{100}},
	})

	return err
}

func (s *service) List(ctx context.Context) ([]models.Preset, error) {
	var records []models.PresetRecord
	if err := s.db.WithContext(ctx).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}

	presets := make([]models.Preset, 0, len(records))
	for _, record := range records {
		presets = append(presets, models.Preset{
			Name:   record.Name,
			Speeds: record.Speeds,
		})
	}

	return presets, nil
}

func (s *service) Save(ctx context.Context, presets []models.Preset) ([]models.Preset, error) {
	records := make([]models.PresetRecord, 0, len(presets))
	for _, preset := range presets {
		records = append(records, models.PresetRecord{
			Name:   preset.Name,
			Speeds: preset.Speeds,
		})
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.PresetRecord{}).Error; err != nil {
			return err
		}

		if len(records) == 0 {
			return nil
		}

		return tx.Create(&records).Error
	})
	if err != nil {
		return nil, err
	}

	return presets, nil
}
