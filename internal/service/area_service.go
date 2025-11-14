package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/nurpe/snowops-operations/internal/model"
	"github.com/nurpe/snowops-operations/internal/repository"
)

type AreaService struct {
	repo *repository.CleaningAreaRepository
}

func NewAreaService(repo *repository.CleaningAreaRepository) *AreaService {
	return &AreaService{repo: repo}
}

type ListAreasInput struct {
	Status     []model.CleaningAreaStatus
	OnlyActive bool
}

func (s *AreaService) List(ctx context.Context, principal model.Principal, input ListAreasInput) ([]model.CleaningArea, error) {
	if principal.IsDriver() {
		return nil, ErrPermissionDenied
	}

	filter := repository.CleaningAreaFilter{
		Status:     input.Status,
		OnlyActive: input.OnlyActive,
	}

	if principal.IsKgu() || principal.IsContractor() {
		filter.ContractorID = &principal.OrganizationID
	}

	return s.repo.List(ctx, filter)
}

func (s *AreaService) Get(ctx context.Context, principal model.Principal, id uuid.UUID) (*model.CleaningArea, error) {
	if principal.IsDriver() {
		return nil, ErrPermissionDenied
	}

	area, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if principal.IsAkimat() {
		return area, nil
	}

	if principal.IsKgu() || principal.IsContractor() {
		if area.DefaultContractorID == nil || *area.DefaultContractorID != principal.OrganizationID {
			return nil, ErrPermissionDenied
		}
		return area, nil
	}

	return nil, ErrPermissionDenied
}

type CreateAreaInput struct {
	Name                string
	Description         *string
	GeometryGeoJSON     string
	City                string
	Status              *model.CleaningAreaStatus
	DefaultContractorID *uuid.UUID
}

func (s *AreaService) Create(ctx context.Context, principal model.Principal, input CreateAreaInput) (*model.CleaningArea, error) {
	if !principal.IsAkimat() && !principal.IsKgu() {
		return nil, ErrPermissionDenied
	}

	if strings.TrimSpace(input.Name) == "" {
		return nil, ErrInvalidInput
	}
	if strings.TrimSpace(input.GeometryGeoJSON) == "" {
		return nil, ErrInvalidInput
	}
	if strings.TrimSpace(input.City) == "" {
		input.City = "Petropavlovsk"
	}

	status := model.CleaningAreaStatusActive
	if input.Status != nil {
		status = *input.Status
	}

	defaultContractorID := input.DefaultContractorID

	if principal.IsKgu() {
		// КГУ может создавать участки только для себя
		if defaultContractorID != nil && *defaultContractorID != principal.OrganizationID {
			return nil, ErrPermissionDenied
		}
		if defaultContractorID == nil {
			defaultContractorID = &principal.OrganizationID
		}
	}

	params := repository.CreateCleaningAreaParams{
		Name:                strings.TrimSpace(input.Name),
		Description:         normalizeOptionalString(input.Description),
		GeometryGeoJSON:     input.GeometryGeoJSON,
		City:                input.City,
		Status:              status,
		DefaultContractorID: defaultContractorID,
		IsActive:            status == model.CleaningAreaStatusActive,
	}

	area, err := s.repo.Create(ctx, params)
	if err != nil {
		return nil, err
	}
	return area, nil
}

type UpdateAreaInput struct {
	ID                  uuid.UUID
	Name                *string
	Description         *string
	Status              *model.CleaningAreaStatus
	DefaultContractorID **uuid.UUID
	IsActive            *bool
}

func (s *AreaService) UpdateMetadata(ctx context.Context, principal model.Principal, input UpdateAreaInput) (*model.CleaningArea, error) {
	if principal.IsDriver() || principal.IsContractor() {
		return nil, ErrPermissionDenied
	}

	if principal.IsKgu() {
		if input.DefaultContractorID != nil {
			if *input.DefaultContractorID == nil || **input.DefaultContractorID != principal.OrganizationID {
				return nil, ErrPermissionDenied
			}
		} else {
			// Ensure area belongs to org
			area, err := s.repo.GetByID(ctx, input.ID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, ErrNotFound
				}
				return nil, err
			}
			if area.DefaultContractorID == nil || *area.DefaultContractorID != principal.OrganizationID {
				return nil, ErrPermissionDenied
			}
		}
	}

	params := repository.UpdateCleaningAreaParams{
		ID:                  input.ID,
		Name:                normalizeOptionalString(input.Name),
		Description:         normalizeOptionalString(input.Description),
		Status:              input.Status,
		DefaultContractorID: input.DefaultContractorID,
		IsActive:            input.IsActive,
	}

	area, err := s.repo.UpdateMetadata(ctx, params)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return area, nil
}

func (s *AreaService) UpdateGeometry(ctx context.Context, principal model.Principal, id uuid.UUID, geoJSON string) (*model.CleaningArea, error) {
	if !principal.IsAkimat() {
		return nil, ErrPermissionDenied
	}
	if strings.TrimSpace(geoJSON) == "" {
		return nil, ErrInvalidInput
	}

	area, err := s.repo.UpdateGeometry(ctx, id, geoJSON)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return area, nil
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	result := trimmed
	return &result
}
