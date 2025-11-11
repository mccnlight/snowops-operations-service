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

type PolygonService struct {
	polygons *repository.PolygonRepository
	cameras  *repository.CameraRepository
}

func NewPolygonService(polygons *repository.PolygonRepository, cameras *repository.CameraRepository) *PolygonService {
	return &PolygonService{
		polygons: polygons,
		cameras:  cameras,
	}
}

type ListPolygonsInput struct {
	OnlyActive bool
}

func (s *PolygonService) List(ctx context.Context, principal model.Principal, input ListPolygonsInput) ([]model.Polygon, error) {
	if principal.IsDriver() {
		return nil, ErrPermissionDenied
	}

	filter := repository.PolygonFilter{
		OnlyActive: input.OnlyActive,
	}

	return s.polygons.List(ctx, filter)
}

func (s *PolygonService) Get(ctx context.Context, principal model.Principal, id uuid.UUID) (*model.Polygon, error) {
	if principal.IsDriver() {
		return nil, ErrPermissionDenied
	}

	polygon, err := s.polygons.GetByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return polygon, nil
}

type CreatePolygonInput struct {
	Name     string
	Address  *string
	Geometry string
	IsActive *bool
}

func (s *PolygonService) Create(ctx context.Context, principal model.Principal, input CreatePolygonInput) (*model.Polygon, error) {
	if !principal.IsAkimat() {
		return nil, ErrPermissionDenied
	}

	if strings.TrimSpace(input.Name) == "" {
		return nil, ErrInvalidInput
	}
	if strings.TrimSpace(input.Geometry) == "" {
		return nil, ErrInvalidInput
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	params := repository.CreatePolygonParams{
		Name:     strings.TrimSpace(input.Name),
		Address:  normalizeOptionalString(input.Address),
		Geometry: input.Geometry,
		IsActive: isActive,
	}

	polygon, err := s.polygons.Create(ctx, params)
	if err != nil {
		return nil, err
	}

	return polygon, nil
}

type UpdatePolygonInput struct {
	ID       uuid.UUID
	Name     *string
	Address  **string
	IsActive *bool
}

func (s *PolygonService) UpdateMetadata(ctx context.Context, principal model.Principal, input UpdatePolygonInput) (*model.Polygon, error) {
	if !principal.IsAkimat() {
		return nil, ErrPermissionDenied
	}

	params := repository.UpdatePolygonParams{
		ID:       input.ID,
		Name:     normalizeOptionalString(input.Name),
		Address:  input.Address,
		IsActive: input.IsActive,
	}

	polygon, err := s.polygons.UpdateMetadata(ctx, params)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return polygon, nil
}

func (s *PolygonService) UpdateGeometry(ctx context.Context, principal model.Principal, id uuid.UUID, geoJSON string) (*model.Polygon, error) {
	if !principal.IsAkimat() {
		return nil, ErrPermissionDenied
	}
	if strings.TrimSpace(geoJSON) == "" {
		return nil, ErrInvalidInput
	}

	polygon, err := s.polygons.UpdateGeometry(ctx, id, geoJSON)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return polygon, nil
}

func (s *PolygonService) ListCameras(ctx context.Context, principal model.Principal, polygonID uuid.UUID) ([]model.Camera, error) {
	if principal.IsDriver() {
		return nil, ErrPermissionDenied
	}

	// ensure polygon exists
	if _, err := s.polygons.GetByID(ctx, polygonID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return s.cameras.ListByPolygon(ctx, polygonID)
}

type CreateCameraInput struct {
	PolygonID uuid.UUID
	Type      model.CameraType
	Name      string
	Location  *string
	IsActive  *bool
}

func (s *PolygonService) CreateCamera(ctx context.Context, principal model.Principal, input CreateCameraInput) (*model.Camera, error) {
	if !principal.IsAkimat() {
		return nil, ErrPermissionDenied
	}

	if input.Type != model.CameraTypeLPR && input.Type != model.CameraTypeVolume {
		return nil, ErrInvalidInput
	}
	if strings.TrimSpace(input.Name) == "" {
		return nil, ErrInvalidInput
	}

	if _, err := s.polygons.GetByID(ctx, input.PolygonID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	params := repository.CreateCameraParams{
		PolygonID: input.PolygonID,
		Type:      input.Type,
		Name:      strings.TrimSpace(input.Name),
		Location:  normalizeOptionalString(input.Location),
		IsActive:  isActive,
	}

	camera, err := s.cameras.Create(ctx, params)
	if err != nil {
		return nil, err
	}
	return camera, nil
}

type UpdateCameraInput struct {
	ID       uuid.UUID
	Type     *model.CameraType
	Name     *string
	Location **string
	IsActive *bool
}

func (s *PolygonService) UpdateCamera(ctx context.Context, principal model.Principal, input UpdateCameraInput) (*model.Camera, error) {
	if !principal.IsAkimat() {
		return nil, ErrPermissionDenied
	}

	if input.Type != nil && *input.Type != model.CameraTypeLPR && *input.Type != model.CameraTypeVolume {
		return nil, ErrInvalidInput
	}

	params := repository.UpdateCameraParams{
		ID:       input.ID,
		Type:     input.Type,
		Name:     normalizeOptionalString(input.Name),
		Location: input.Location,
		IsActive: input.IsActive,
	}

	camera, err := s.cameras.Update(ctx, params)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return camera, nil
}
