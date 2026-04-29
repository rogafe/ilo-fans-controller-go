package models

import "time"

type Fan struct {
	Name          string `json:"name"`
	Speed         int    `json:"speed"`
	CommandNumber int    `json:"commandNumber"`
}

type Temperature struct {
	Chassis            int    `json:"chassis"`
	Index              int    `json:"index"`
	Locale             int    `json:"locale"`
	LocaleLabel        string `json:"localeLabel"`
	Label              string `json:"label"`
	PhysicalContext    string `json:"physicalContext"`
	Temperature        int    `json:"temperature"`
	Threshold          int    `json:"threshold"`
	ThresholdType      int    `json:"thresholdType"`
	ThresholdTypeLabel string `json:"thresholdTypeLabel"`
	Condition          int    `json:"condition"`
	ConditionLabel     string `json:"conditionLabel"`
	Health             string `json:"health"`
	State              string `json:"state"`
	CautionThreshold   int    `json:"cautionThreshold"`
	CriticalThreshold  int    `json:"criticalThreshold"`
	LocationX          int    `json:"locationX"`
	LocationY          int    `json:"locationY"`
	Present            bool   `json:"present"`
}

type Preset struct {
	Name   string `json:"name"`
	Speeds []int  `json:"speeds"`
}

type PresetRecord struct {
	ID        uint      `gorm:"primaryKey"`
	Name      string    `gorm:"not null"`
	Speeds    []int     `gorm:"type:jsonb;serializer:json;not null"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
}

type SetFansRequest struct {
	ClientID string         `json:"clientId"`
	Speed    *int           `json:"speed"`
	Fans     map[string]int `json:"fans"`
}

type StatusMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
