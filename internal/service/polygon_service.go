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

type PolygonFeatures struct {
	AllowAkimatWrite bool
}

type PolygonService struct {
	polygons *repository.PolygonRepository
	cameras  *repository.CameraRepository
	access   *repository.PolygonAccessRepository
	features PolygonFeatures
}

func NewPolygonService(
	polygons *repository.PolygonRepository,
	cameras *repository.CameraRepository,
	access *repository.PolygonAccessRepository,
	features PolygonFeatures,
) *PolygonService {
	return &PolygonService{
		polygons: polygons,
		cameras:  cameras,
		access:   access,
		features: features,
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

	if principal.IsContractor() {
		filter.ContractorID = &principal.OrganizationID
	}

	// LANDFILL видит только свои полигоны
	if principal.IsLandfill() {
		filter.OrganizationID = &principal.OrganizationID
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

	if principal.IsContractor() {
		hasAccess, err := s.access.HasAccessForContractor(ctx, id, principal.OrganizationID)
		if err != nil {
			return nil, err
		}
		if !hasAccess {
			return nil, ErrPermissionDenied
		}
	}

	// LANDFILL может видеть только свои полигоны
	if principal.IsLandfill() {
		if polygon.OrganizationID == nil || *polygon.OrganizationID != principal.OrganizationID {
			return nil, ErrPermissionDenied
		}
	}

	return polygon, nil
}

type CreatePolygonInput struct {
	Name           string
	Address        *string
	Geometry       string
	OrganizationID *uuid.UUID // Для LANDFILL организаций
	IsActive       *bool
}

func (s *PolygonService) Create(ctx context.Context, principal model.Principal, input CreatePolygonInput) (*model.Polygon, error) {
	if !s.canManagePolygons(principal) {
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

	// Для LANDFILL автоматически устанавливаем organization_id
	organizationID := input.OrganizationID
	if principal.IsLandfill() {
		organizationID = &principal.OrganizationID
	}

	params := repository.CreatePolygonParams{
		Name:           strings.TrimSpace(input.Name),
		Address:        normalizeOptionalString(input.Address),
		Geometry:       input.Geometry,
		OrganizationID: organizationID,
		IsActive:       isActive,
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
	if !s.canManagePolygons(principal) {
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
	if !s.canManagePolygons(principal) {
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

	// ensure polygon exists and current principal may read it
	if _, err := s.Get(ctx, principal, polygonID); err != nil {
		if errors.Is(err, ErrNotFound) {
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
	if !s.canManagePolygons(principal) {
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
	if !s.canManagePolygons(principal) {
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

func (s *PolygonService) ListAccess(ctx context.Context, principal model.Principal, polygonID uuid.UUID) ([]repository.PolygonAccessEntry, error) {
	if !s.canManagePolygons(principal) {
		return nil, ErrPermissionDenied
	}
	if _, err := s.polygons.GetByID(ctx, polygonID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.access.ListByPolygon(ctx, polygonID)
}

func (s *PolygonService) GrantAccess(ctx context.Context, principal model.Principal, polygonID, contractorID uuid.UUID, source string) error {
	if !s.canManagePolygons(principal) {
		return ErrPermissionDenied
	}
	if contractorID == uuid.Nil {
		return ErrInvalidInput
	}
	if _, err := s.polygons.GetByID(ctx, polygonID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if strings.TrimSpace(source) == "" {
		source = "MANUAL"
	}
	source = strings.TrimSpace(source)
	return s.access.Grant(ctx, polygonID, contractorID, source)
}

func (s *PolygonService) RevokeAccess(ctx context.Context, principal model.Principal, polygonID, contractorID uuid.UUID) error {
	if !s.canManagePolygons(principal) {
		return ErrPermissionDenied
	}
	if _, err := s.polygons.GetByID(ctx, polygonID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	return s.access.Revoke(ctx, polygonID, contractorID)
}

func (s *PolygonService) ContainsPoint(ctx context.Context, principal model.Principal, polygonID uuid.UUID, lat, lng float64) (bool, error) {
	if !(principal.IsKgu() || principal.IsTechnicalOperator() || principal.IsAkimat()) {
		return false, ErrPermissionDenied
	}
	if _, err := s.polygons.GetByID(ctx, polygonID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrNotFound
		}
		return false, err
	}
	return s.polygons.ContainsPoint(ctx, polygonID, lat, lng)
}

func (s *PolygonService) ResolveCameraPolygon(ctx context.Context, principal model.Principal, cameraID uuid.UUID) (*model.Camera, *model.Polygon, error) {
	if !(principal.IsKgu() || principal.IsTechnicalOperator() || principal.IsAkimat()) {
		return nil, nil, ErrPermissionDenied
	}
	camera, err := s.cameras.GetByID(ctx, cameraID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	polygon, err := s.polygons.GetByID(ctx, camera.PolygonID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	return camera, polygon, nil
}

func (s *PolygonService) Delete(ctx context.Context, principal model.Principal, id uuid.UUID) error {
	if !s.canManagePolygons(principal) {
		return ErrPermissionDenied
	}

	// Проверяем существование полигона
	_, err := s.polygons.GetByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	// Проверяем наличие связанных рейсов
	hasTrips, err := s.polygons.HasRelatedTrips(ctx, id)
	if err != nil {
		return err
	}
	if hasTrips {
		return ErrPolygonHasTrips
	}

	// Удаляем полигон (cameras и polygon_access удалятся автоматически через CASCADE)
	return s.polygons.Delete(ctx, id)
}

func (s *PolygonService) canManagePolygons(principal model.Principal) bool {
	// KGU и LANDFILL роли (включая обратную совместимость с TOO_ADMIN)
	if principal.IsKgu() || principal.IsLandfill() {
		return true
	}
	if s.features.AllowAkimatWrite && principal.IsAkimat() {
		return true
	}
	return false
}
