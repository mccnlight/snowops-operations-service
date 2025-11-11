package model

import (
	"time"

	"github.com/google/uuid"
)

type UserRole string

const (
	UserRoleAkimatAdmin     UserRole = "AKIMAT_ADMIN"
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
}

func (p Principal) IsAkimat() bool {
	return p.Role == UserRoleAkimatAdmin
}

func (p Principal) IsToo() bool {
	return p.Role == UserRoleTooAdmin
}

func (p Principal) IsContractor() bool {
	return p.Role == UserRoleContractorAdmin
}

func (p Principal) IsDriver() bool {
	return p.Role == UserRoleDriver
}
