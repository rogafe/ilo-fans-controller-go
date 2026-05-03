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

type AdvancedProfile struct {
	Name          string                `json:"name"`
	CommandBundle AdvancedCommandBundle `json:"commandBundle"`
	Warning       string                `json:"warning"`
	BuiltIn       bool                  `json:"builtIn"`
}

type AdvancedCommandBundle struct {
	FanMinimums     []FanBoundCommand `json:"fanMinimums"`
	FanMaximums     []FanBoundCommand `json:"fanMaximums"`
	PIDLows         []PIDCommand      `json:"pidLows"`
	PIDHighs        []PIDCommand      `json:"pidHighs"`
	OCSD            []OCSDCommand     `json:"ocsd"`
	DisabledSensors []int             `json:"disabledSensors"`
}

type FanBoundCommand struct {
	Fan   int `json:"fan"`
	Value int `json:"value"`
}

type PIDCommand struct {
	Sensors []int `json:"sensors"`
	Value   int   `json:"value"`
}

type OCSDCommand struct {
	Sensors []int `json:"sensors"`
	Value   int   `json:"value"`
}

type PresetRecord struct {
	ID        uint      `gorm:"primaryKey"`
	Name      string    `gorm:"not null"`
	Speeds    []int     `gorm:"type:jsonb;serializer:json;not null"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
}

type AdvancedProfileRecord struct {
	ID            uint                  `gorm:"primaryKey"`
	Name          string                `gorm:"not null;uniqueIndex"`
	Warning       string                `gorm:"not null"`
	BuiltIn       bool                  `gorm:"not null;default:false"`
	CommandBundle AdvancedCommandBundle `gorm:"type:jsonb;serializer:json;not null"`
	CreatedAt     time.Time             `json:"-"`
	UpdatedAt     time.Time             `json:"-"`
}

type SetFansRequest struct {
	ClientID string         `json:"clientId"`
	Speed    *int           `json:"speed"`
	Fans     map[string]int `json:"fans"`
}

type ApplyAdvancedProfileRequest struct {
	ClientID     string `json:"clientId"`
	ProfileName  string `json:"profileName"`
	Confirmation string `json:"confirmation"`
}

type StatusMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
