package database

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"ilo-fans-controller-go/internals/config"
	"ilo-fans-controller-go/internals/models"
)

func Open(cfg config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.PresetRecord{}, &models.AdvancedProfileRecord{}); err != nil {
		return nil, err
	}

	return db, nil
}
