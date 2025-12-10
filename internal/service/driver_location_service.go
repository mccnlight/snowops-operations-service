package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/nurpe/snowops-operations/internal/model"
	"github.com/nurpe/snowops-operations/internal/repository"
)

type DriverLocationService struct {
	repo *repository.DriverLocationRepository
}

func NewDriverLocationService(repo *repository.DriverLocationRepository) *DriverLocationService {
	return &DriverLocationService{repo: repo}
}

type UpdateDriverLocationInput struct {
	Lat      float64  `json:"lat"`
	Lon      float64  `json:"lon"`
	Accuracy *float64 `json:"accuracy,omitempty"`
}

func (s *DriverLocationService) UpdateLocation(ctx context.Context, principal model.Principal, input UpdateDriverLocationInput) error {
	if !principal.IsDriver() {
		return ErrPermissionDenied
	}

	if principal.DriverID == nil {
		return errors.New("driver_id is missing in principal")
	}

	location := &model.DriverLocation{
		DriverID: *principal.DriverID,
		Lat:      input.Lat,
		Lon:      input.Lon,
		Accuracy: input.Accuracy,
	}

	return s.repo.UpsertLocation(ctx, location)
}

type DriverLocationData struct {
	DriverID  uuid.UUID `json:"driver_id"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	UpdatedAt string    `json:"updated_at"`
	Accuracy  *float64  `json:"accuracy,omitempty"`
}

func (s *DriverLocationService) GetDriverLocations(ctx context.Context, principal model.Principal) ([]DriverLocationData, error) {
	switch {
	case principal.IsAkimat() || principal.IsKgu() || principal.IsLandfill():
		return s.getAllLocations(ctx)
	case principal.IsContractor():
		return s.getContractorDriversLocations(ctx, principal)
	case principal.IsDriver():
		return s.getOwnLocation(ctx, principal)
	default:
		return nil, ErrPermissionDenied
	}
}

func (s *DriverLocationService) getAllLocations(ctx context.Context) ([]DriverLocationData, error) {
	locations, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]DriverLocationData, 0, len(locations))
	for _, loc := range locations {
		result = append(result, DriverLocationData{
			DriverID:  loc.DriverID,
			Lat:       loc.Lat,
			Lon:       loc.Lon,
			UpdatedAt: loc.UpdatedAt.Format(time.RFC3339),
			Accuracy:  loc.Accuracy,
		})
	}
	return result, nil
}

func (s *DriverLocationService) getContractorDriversLocations(ctx context.Context, principal model.Principal) ([]DriverLocationData, error) {
	locations, err := s.repo.GetByContractor(ctx, principal.OrganizationID)
	if err != nil {
		return nil, err
	}

	result := make([]DriverLocationData, 0, len(locations))
	for _, loc := range locations {
		result = append(result, DriverLocationData{
			DriverID:  loc.DriverID,
			Lat:       loc.Lat,
			Lon:       loc.Lon,
			UpdatedAt: loc.UpdatedAt.Format(time.RFC3339),
			Accuracy:  loc.Accuracy,
		})
	}
	return result, nil
}

func (s *DriverLocationService) getOwnLocation(ctx context.Context, principal model.Principal) ([]DriverLocationData, error) {
	if principal.DriverID == nil {
		return []DriverLocationData{}, nil
	}

	location, err := s.repo.GetByDriver(ctx, *principal.DriverID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []DriverLocationData{}, nil
		}
		return nil, err
	}

	return []DriverLocationData{{
		DriverID:  location.DriverID,
		Lat:       location.Lat,
		Lon:       location.Lon,
		UpdatedAt: location.UpdatedAt.Format(time.RFC3339),
		Accuracy:  location.Accuracy,
	}}, nil
}
