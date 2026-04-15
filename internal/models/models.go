package models

import (
	"time"

	"gorm.io/datatypes"
)

type Frame struct {
	ID         string         `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FilePath   string         `gorm:"not null" json:"filePath"`
	JpegPath   *string        `json:"jpegPath"`
	CapturedAt time.Time      `gorm:"not null" json:"capturedAt"`
	ExposureMs float64        `gorm:"not null" json:"exposureMs"`
	Gain       int            `gorm:"not null" json:"gain"`
	MedianADU  *float64       `json:"medianAdu"`
	Metadata   datatypes.JSON `json:"metadata"`
	CreatedAt  time.Time      `json:"createdAt"`
}

type AnalysisResult struct {
	ID         string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FrameID    string    `gorm:"type:uuid;not null;index" json:"frameId"`
	CloudCover float64   `gorm:"not null" json:"cloudCover"`
	Brightness float64   `gorm:"not null" json:"brightness"`
	SkyQuality string    `gorm:"not null" json:"skyQuality"`
	SQM        *float64  `json:"sqm"`
	Seeing     *float64  `json:"seeing"`
	AnalyzedAt time.Time `gorm:"autoCreateTime" json:"analyzedAt"`
}

type SensorReading struct {
	ID         string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SensorType string    `gorm:"not null;index" json:"sensorType"`
	Value      float64   `gorm:"not null" json:"value"`
	Unit       string    `gorm:"not null" json:"unit"`
	Source     string    `gorm:"default:'local'" json:"source"`
	RecordedAt time.Time `gorm:"autoCreateTime" json:"recordedAt"`
}

type SafetyState struct {
	ID             string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	State          string    `gorm:"not null" json:"state"`
	ImagingQuality string    `gorm:"not null" json:"imagingQuality"`
	Reason         *string   `json:"reason"`
	EvaluatedAt    time.Time `gorm:"autoCreateTime" json:"evaluatedAt"`
}

type Alert struct {
	ID             string     `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Type           string     `gorm:"not null" json:"type"`
	Message        string     `gorm:"not null" json:"message"`
	SentAt         time.Time  `gorm:"autoCreateTime" json:"sentAt"`
	AcknowledgedAt *time.Time `json:"acknowledgedAt"`
}

type Config struct {
	Key       string         `gorm:"primaryKey" json:"key"`
	Value     datatypes.JSON `gorm:"not null" json:"value"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
}

type Detection struct {
	ID        string         `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FrameID   string         `gorm:"type:uuid;not null;index" json:"frameId"`
	Type      string         `gorm:"not null;index" json:"type"` // star, planet, satellite, plane, meteor, constellation
	Data      datatypes.JSON `gorm:"not null" json:"data"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
}

type OverlayLayout struct {
	ID        string         `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name      string         `gorm:"not null;uniqueIndex" json:"name"`
	Layout    datatypes.JSON `gorm:"not null" json:"layout"`
	IsActive  bool           `gorm:"default:false" json:"isActive"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
}

type DarkFrame struct {
	ID          string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FilePath    string    `gorm:"not null" json:"filePath"`
	Temperature float64   `gorm:"not null;index" json:"temperature"`
	ExposureMs  float64   `gorm:"not null;index" json:"exposureMs"`
	Gain        int       `gorm:"not null" json:"gain"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

type NightlySummary struct {
	ID            string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Date          string    `gorm:"not null;uniqueIndex" json:"date"` // YYYY-MM-DD
	TotalFrames   int       `json:"totalFrames"`
	ClearHours    float64   `json:"clearHours"`
	CloudCoverAvg float64   `json:"cloudCoverAvg"`
	SQMAvg        *float64  `json:"sqmAvg"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

type ServiceHealth struct {
	Name     string    `gorm:"primaryKey" json:"name"`
	LastSeen time.Time `json:"lastSeen"`
	Status   string    `gorm:"default:'unknown'" json:"status"`
	Restarts int       `gorm:"default:0" json:"restarts"`
}

