package model

import (
	"time"

	"github.com/google/uuid"
)

type UserRole string

const (
	UserRoleAkimatAdmin     UserRole = "AKIMAT_ADMIN"
	UserRoleKguZkhAdmin     UserRole = "KGU_ZKH_ADMIN"
	UserRoleTooAdmin        UserRole = "TOO_ADMIN"
	UserRoleContractorAdmin UserRole = "CONTRACTOR_ADMIN"
	UserRoleDriver          UserRole = "DRIVER"
)

type CleaningAreaStatus string

const (
	CleaningAreaStatusActive   CleaningAreaStatus = "ACTIVE"
	CleaningAreaStatusInactive CleaningAreaStatus = "INACTIVE"
)

type CameraType string

const (
	CameraTypeLPR    CameraType = "LPR"
	CameraTypeVolume CameraType = "VOLUME"
)

type CleaningArea struct {
	ID                   uuid.UUID           `json:"id"`
	Name                 string              `json:"name"`
	Description          *string             `json:"description,omitempty"`
	Geometry             string              `json:"geometry"` // GeoJSON
	City                 string              `json:"city"`
	Status               CleaningAreaStatus  `json:"status"`
	DefaultContractorID  *uuid.UUID          `json:"default_contractor_id,omitempty"`
	IsActive             bool                `json:"is_active"`
	CreatedAt            time.Time           `json:"created_at"`
	UpdatedAt            time.Time           `json:"updated_at"`
	ActiveTicketCount    *int                `json:"active_ticket_count,omitempty"`
	DefaultContractorOrg *OrganizationLookup `json:"default_contractor,omitempty"`
}

type Polygon struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Address     *string   `json:"address,omitempty"`
	Geometry    string    `json:"geometry"` // GeoJSON
	CameraCount *int      `json:"camera_count,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Camera struct {
	ID        uuid.UUID  `json:"id"`
	PolygonID uuid.UUID  `json:"polygon_id"`
	Type      CameraType `json:"type"`
	Name      string     `json:"name"`
	Location  *string    `json:"location,omitempty"` // GeoJSON point
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type OrganizationLookup struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Principal struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	Role           UserRole
	DriverID       *uuid.UUID
}

func (p Principal) IsAkimat() bool {
	return p.Role == UserRoleAkimatAdmin
}

func (p Principal) IsKgu() bool {
	return p.Role == UserRoleKguZkhAdmin
}

func (p Principal) IsTechnicalOperator() bool {
	return p.Role == UserRoleTooAdmin
}

func (p Principal) IsContractor() bool {
	return p.Role == UserRoleContractorAdmin
}

func (p Principal) IsDriver() bool {
	return p.Role == UserRoleDriver
}

type VehicleStatus string

const (
	VehicleStatusInTrip  VehicleStatus = "IN_TRIP"
	VehicleStatusIdle    VehicleStatus = "IDLE"
	VehicleStatusOffline VehicleStatus = "OFFLINE"
)

type Vehicle struct {
	ID           uuid.UUID  `json:"id"`
	PlateNumber  string     `json:"plate_number"`
	ContractorID *uuid.UUID `json:"contractor_id,omitempty"`
	IsActive     bool       `json:"is_active"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type GPSDevice struct {
	ID        uuid.UUID `json:"id"`
	VehicleID uuid.UUID `json:"vehicle_id"`
	IMEI      *string   `json:"imei,omitempty"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GPSPoint struct {
	ID          uuid.UUID  `json:"id"`
	GPSDeviceID *uuid.UUID `json:"gps_device_id,omitempty"`
	VehicleID   uuid.UUID  `json:"vehicle_id"`
	CapturedAt  time.Time  `json:"captured_at"`
	Lat         float64    `json:"lat"`
	Lon         float64    `json:"lon"`
	SpeedKmh    float64    `json:"speed_kmh"`
	HeadingDeg  float64    `json:"heading_deg"`
	RawPayload  *string    `json:"raw_payload,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}
