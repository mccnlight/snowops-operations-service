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

type AreaFeatures struct {
	AllowAkimatWrite             bool
	AllowGeometryUpdateWhenInUse bool
}

type AreaService struct {
	repo       *repository.CleaningAreaRepository
	accessRepo *repository.CleaningAreaAccessRepository
	features   AreaFeatures
}

func NewAreaService(
	repo *repository.CleaningAreaRepository,
	accessRepo *repository.CleaningAreaAccessRepository,
	features AreaFeatures,
) *AreaService {
	return &AreaService{
		repo:       repo,
		accessRepo: accessRepo,
		features:   features,
	}
}

type ListAreasInput struct {
	Status     []model.CleaningAreaStatus
	OnlyActive bool
}

func (s *AreaService) List(ctx context.Context, principal model.Principal, input ListAreasInput) ([]model.CleaningArea, error) {
	if principal.IsTechnicalOperator() {
		return nil, ErrPermissionDenied
	}

	filter := repository.CleaningAreaFilter{
		Status:     input.Status,
		OnlyActive: input.OnlyActive,
	}

	if principal.IsContractor() {
		filter.ContractorID = &principal.OrganizationID
	}

	return s.repo.List(ctx, filter)
}

func (s *AreaService) Get(ctx context.Context, principal model.Principal, id uuid.UUID) (*model.CleaningArea, error) {
	if principal.IsTechnicalOperator() {
		return nil, ErrPermissionDenied
	}

	area, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if principal.IsAkimat() || principal.IsKgu() || principal.IsDriver() {
		return area, nil
	}

	if principal.IsContractor() {
		if area.DefaultContractorID != nil && *area.DefaultContractorID == principal.OrganizationID {
			return area, nil
		}
		hasAccess, err := s.accessRepo.HasAccessForContractor(ctx, area.ID, principal.OrganizationID)
		if err != nil {
			return nil, err
		}
		if hasAccess {
			return area, nil
		}
		return nil, ErrPermissionDenied
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
	if !s.canManageAreas(principal) {
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

	params := repository.CreateCleaningAreaParams{
		Name:                strings.TrimSpace(input.Name),
		Description:         normalizeOptionalString(input.Description),
		GeometryGeoJSON:     input.GeometryGeoJSON,
		City:                input.City,
		Status:              status,
		DefaultContractorID: input.DefaultContractorID,
		IsActive:            true,
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
	if !s.canManageAreas(principal) {
		return nil, ErrPermissionDenied
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
	if !s.canManageAreas(principal) {
		return nil, ErrPermissionDenied
	}
	if strings.TrimSpace(geoJSON) == "" {
		return nil, ErrInvalidInput
	}

	if !s.features.AllowGeometryUpdateWhenInUse {
		inUse, err := s.accessRepo.HasActiveEntries(ctx, id)
		if err != nil {
			return nil, err
		}
		if inUse {
			return nil, ErrConflict
		}
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

func (s *AreaService) ListAccess(ctx context.Context, principal model.Principal, areaID uuid.UUID) ([]repository.CleaningAreaAccessEntry, error) {
	if !s.canManageAreas(principal) {
		return nil, ErrPermissionDenied
	}
	if _, err := s.repo.GetByID(ctx, areaID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.accessRepo.ListByArea(ctx, areaID)
}

func (s *AreaService) GrantAccess(ctx context.Context, principal model.Principal, areaID, contractorID uuid.UUID, source string) error {
	if !s.canManageAreas(principal) {
		return ErrPermissionDenied
	}
	if contractorID == uuid.Nil {
		return ErrInvalidInput
	}
	if _, err := s.repo.GetByID(ctx, areaID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if strings.TrimSpace(source) == "" {
		source = "MANUAL"
	}
	source = strings.TrimSpace(source)
	return s.accessRepo.Grant(ctx, areaID, contractorID, source)
}

func (s *AreaService) RevokeAccess(ctx context.Context, principal model.Principal, areaID, contractorID uuid.UUID) error {
	if !s.canManageAreas(principal) {
		return ErrPermissionDenied
	}
	if _, err := s.repo.GetByID(ctx, areaID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	return s.accessRepo.Revoke(ctx, areaID, contractorID)
}

type AreaTicketTemplate struct {
	Area                  *model.CleaningArea
	AccessibleContractors []uuid.UUID
}

func (s *AreaService) TicketTemplate(ctx context.Context, principal model.Principal, areaID uuid.UUID) (*AreaTicketTemplate, error) {
	if !principal.IsKgu() {
		return nil, ErrPermissionDenied
	}
	area, err := s.repo.GetByID(ctx, areaID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	entries, err := s.accessRepo.ListByArea(ctx, areaID)
	if err != nil {
		return nil, err
	}
	seen := make(map[uuid.UUID]struct{})
	contractors := make([]uuid.UUID, 0, len(entries)+1)
	for _, entry := range entries {
		if entry.RevokedAt == nil {
			if _, exists := seen[entry.ContractorID]; !exists {
				contractors = append(contractors, entry.ContractorID)
				seen[entry.ContractorID] = struct{}{}
			}
		}
	}
	if area.DefaultContractorID != nil {
		if _, exists := seen[*area.DefaultContractorID]; !exists {
			contractors = append(contractors, *area.DefaultContractorID)
		}
	}
	template := &AreaTicketTemplate{
		Area:                  area,
		AccessibleContractors: contractors,
	}
	return template, nil
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

func (s *AreaService) GetDeletionInfo(ctx context.Context, principal model.Principal, id uuid.UUID) (*DeletionInfo, error) {
	if !s.canManageAreas(principal) {
		return nil, ErrPermissionDenied
	}

	// Проверяем существование участка
	area, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// Получаем информацию о зависимостях
	deps, err := s.repo.GetDependencies(ctx, id)
	if err != nil {
		return nil, err
	}

	return &DeletionInfo{
		Area:         area,
		Dependencies: deps,
	}, nil
}

func (s *AreaService) Delete(ctx context.Context, principal model.Principal, id uuid.UUID, force bool) error {
	if !s.canManageAreas(principal) {
		return ErrPermissionDenied
	}

	// Проверяем существование участка
	_, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	// Если force=false, проверяем наличие связанных тикетов
	if !force {
		hasTickets, err := s.repo.HasRelatedTickets(ctx, id)
		if err != nil {
			return err
		}
		if hasTickets {
			return ErrAreaHasTickets
		}
	}

	// Удаляем участок
	// cleaning_area_access удалится автоматически через CASCADE
	// tickets и связанные данные нужно удалить вручную, если force=true
	if force {
		// Удаляем тикеты (каскадно удалятся ticket_assignments и appeals)
		// trips.ticket_id станет NULL автоматически через ON DELETE SET NULL
		if err := s.repo.DeleteTicketsByAreaID(ctx, id); err != nil {
			return err
		}
	}

	return s.repo.Delete(ctx, id)
}

type DeletionInfo struct {
	Area         *model.CleaningArea
	Dependencies *repository.CleaningAreaDependencies
}

func (s *AreaService) canManageAreas(principal model.Principal) bool {
	if principal.IsKgu() {
		return true
	}
	if s.features.AllowAkimatWrite && principal.IsAkimat() {
		return true
	}
	return false
}
