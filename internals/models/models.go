package models

import "time"

type Fan struct {
	Name          string `json:"name"`
	Speed         int    `json:"speed"`
	CommandNumber int    `json:"commandNumber"`
}

type Temperature struct {
	Index       int    `json:"index"`
	Locale      int    `json:"locale"`
	Label       string `json:"label"`
	Temperature int    `json:"temperature"`
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
