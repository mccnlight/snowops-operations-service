package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/nurpe/snowops-operations/internal/model"
)

type CleaningAreaFilter struct {
	Status       []model.CleaningAreaStatus
	ContractorID *uuid.UUID
	DriverID     *uuid.UUID
	OnlyActive   bool
}

type CleaningAreaRepository struct {
	db *gorm.DB
}

func NewCleaningAreaRepository(db *gorm.DB) *CleaningAreaRepository {
	return &CleaningAreaRepository{db: db}
}

func (r *CleaningAreaRepository) List(ctx context.Context, filter CleaningAreaFilter) ([]model.CleaningArea, error) {
	query := r.db.WithContext(ctx).
		Table("cleaning_areas").
		Select(`
			id,
			name,
			description,
			ST_AsGeoJSON(geometry) AS geometry,
			city,
			status::text AS status,
			default_contractor_id,
			is_active,
			created_at,
			updated_at
		`)

	if filter.OnlyActive {
		query = query.Where("is_active = TRUE")
	}

	if len(filter.Status) > 0 {
		query = query.Where("status IN ?", serializeStatuses(filter.Status))
	}

	if filter.ContractorID != nil {
		query = query.Where(`
			(
				default_contractor_id = ?
				OR EXISTS (
					SELECT 1
					FROM cleaning_area_access ca
					WHERE ca.cleaning_area_id = cleaning_areas.id
						AND ca.contractor_id = ?
						AND ca.revoked_at IS NULL
				)
			)
		`, *filter.ContractorID, *filter.ContractorID)
	}

	if filter.DriverID != nil {
		query = query.Where(`
			EXISTS (
				SELECT 1
				FROM ticket_assignments ta
				JOIN tickets t ON t.id = ta.ticket_id
				WHERE ta.driver_id = ?
					AND ta.is_active = TRUE
					AND t.cleaning_area_id = cleaning_areas.id
			)
		`, *filter.DriverID)
	}

	query = query.Order("name ASC")

	var areas []model.CleaningArea
	if err := query.Scan(&areas).Error; err != nil {
		return nil, err
	}

	return areas, nil
}

func (r *CleaningAreaRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.CleaningArea, error) {
	var area model.CleaningArea
	err := r.db.WithContext(ctx).
		Raw(`
			SELECT
				id,
				name,
				description,
				ST_AsGeoJSON(geometry) AS geometry,
				city,
				status::text AS status,
				default_contractor_id,
				is_active,
				created_at,
				updated_at
			FROM cleaning_areas
			WHERE id = ?
			LIMIT 1
		`, id).
		Scan(&area).Error
	if err != nil {
		return nil, err
	}
	if area.ID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &area, nil
}

type CreateCleaningAreaParams struct {
	Name                string
	Description         *string
	GeometryGeoJSON     string
	City                string
	Status              model.CleaningAreaStatus
	DefaultContractorID *uuid.UUID
	IsActive            bool
}

func (r *CleaningAreaRepository) Create(ctx context.Context, params CreateCleaningAreaParams) (*model.CleaningArea, error) {
	var area model.CleaningArea
	err := r.db.WithContext(ctx).
		Raw(`
			INSERT INTO cleaning_areas
				(name, description, geometry, city, status, default_contractor_id, is_active)
			VALUES
				(?, ?, ST_SetSRID(ST_GeomFromGeoJSON(?), 4326), ?, ?, ?, ?)
			RETURNING
				id,
				name,
				description,
				ST_AsGeoJSON(geometry) AS geometry,
				city,
				status::text AS status,
				default_contractor_id,
				is_active,
				created_at,
				updated_at
		`,
			params.Name,
			params.Description,
			params.GeometryGeoJSON,
			params.City,
			params.Status,
			params.DefaultContractorID,
			params.IsActive,
		).
		Scan(&area).Error
	if err != nil {
		return nil, err
	}
	return &area, nil
}

type UpdateCleaningAreaParams struct {
	ID                  uuid.UUID
	Name                *string
	Description         *string
	Status              *model.CleaningAreaStatus
	DefaultContractorID **uuid.UUID
	IsActive            *bool
}

func (r *CleaningAreaRepository) UpdateMetadata(ctx context.Context, params UpdateCleaningAreaParams) (*model.CleaningArea, error) {
	setClauses := []string{"updated_at = NOW()"}
	values := []interface{}{}

	if params.Name != nil {
		setClauses = append(setClauses, "name = ?")
		values = append(values, *params.Name)
	}
	if params.Description != nil {
		setClauses = append(setClauses, "description = ?")
		values = append(values, *params.Description)
	}
	if params.Status != nil {
		setClauses = append(setClauses, "status = ?")
		values = append(values, *params.Status)
	}
	if params.DefaultContractorID != nil {
		if *params.DefaultContractorID == nil {
			setClauses = append(setClauses, "default_contractor_id = NULL")
		} else {
			setClauses = append(setClauses, "default_contractor_id = ?")
			values = append(values, **params.DefaultContractorID)
		}
	}
	if params.IsActive != nil {
		setClauses = append(setClauses, "is_active = ?")
		values = append(values, *params.IsActive)
	}

	values = append(values, params.ID)

	query := `
		UPDATE cleaning_areas
		SET %s
		WHERE id = ?
		RETURNING
			id,
			name,
			description,
			ST_AsGeoJSON(geometry) AS geometry,
			city,
			status::text AS status,
			default_contractor_id,
			is_active,
			created_at,
			updated_at
	`

	var area model.CleaningArea
	err := r.db.WithContext(ctx).
		Raw(
			fmt.Sprintf(query, strings.Join(setClauses, ", ")),
			values...,
		).Scan(&area).Error
	if err != nil {
		return nil, err
	}
	if area.ID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &area, nil
}

func (r *CleaningAreaRepository) UpdateGeometry(ctx context.Context, id uuid.UUID, geoJSON string) (*model.CleaningArea, error) {
	var area model.CleaningArea
	err := r.db.WithContext(ctx).
		Raw(`
			UPDATE cleaning_areas
			SET
				geometry = ST_SetSRID(ST_GeomFromGeoJSON(?), 4326),
				updated_at = NOW()
			WHERE id = ?
			RETURNING
				id,
				name,
				description,
				ST_AsGeoJSON(geometry) AS geometry,
				city,
				status::text AS status,
				default_contractor_id,
				is_active,
				created_at,
				updated_at
		`, geoJSON, id).
		Scan(&area).Error
	if err != nil {
		return nil, err
	}
	if area.ID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &area, nil
}

func (r *CleaningAreaRepository) ContainsPoint(ctx context.Context, areaID uuid.UUID, lat, lng float64) (bool, error) {
	var contains bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT ST_Contains(
			(SELECT geometry FROM cleaning_areas WHERE id = ? AND is_active = TRUE),
			ST_SetSRID(ST_MakePoint(?, ?), 4326)
		)
	`, areaID, lng, lat).Scan(&contains).Error
	if err != nil {
		return false, err
	}
	return contains, nil
}

func (r *CleaningAreaRepository) FindAreaContainingPoint(ctx context.Context, lat, lng float64) (*model.CleaningArea, error) {
	var area model.CleaningArea
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			id,
			name,
			description,
			ST_AsGeoJSON(geometry) AS geometry,
			city,
			status::text AS status,
			default_contractor_id,
			is_active,
			created_at,
			updated_at
		FROM cleaning_areas
		WHERE is_active = TRUE
			AND ST_Contains(geometry, ST_SetSRID(ST_MakePoint(?, ?), 4326))
		LIMIT 1
	`, lng, lat).Scan(&area).Error
	if err != nil {
		return nil, err
	}
	if area.ID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &area, nil
}

func (r *CleaningAreaRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Table("cleaning_areas").
		Where("id = ?", id).
		Delete(nil)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *CleaningAreaRepository) HasRelatedTickets(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("tickets").
		Where("cleaning_area_id = ?", id).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

type CleaningAreaDependencies struct {
	TicketsCount       int64 `json:"tickets_count"`
	TripsCount         int64 `json:"trips_count"`
	AssignmentsCount   int64 `json:"assignments_count"`
	AppealsCount       int64 `json:"appeals_count"`
	ViolationsCount    int64 `json:"violations_count"`
	AccessRecordsCount int64 `json:"access_records_count"`
}

func (r *CleaningAreaRepository) GetDependencies(ctx context.Context, id uuid.UUID) (*CleaningAreaDependencies, error) {
	var deps CleaningAreaDependencies

	// Считаем тикеты
	if err := r.db.WithContext(ctx).
		Table("tickets").
		Where("cleaning_area_id = ?", id).
		Count(&deps.TicketsCount).Error; err != nil {
		return nil, err
	}

	// Считаем рейсы через тикеты
	if err := r.db.WithContext(ctx).
		Table("trips").
		Joins("JOIN tickets ON tickets.id = trips.ticket_id").
		Where("tickets.cleaning_area_id = ?", id).
		Count(&deps.TripsCount).Error; err != nil {
		return nil, err
	}

	// Считаем назначения водителей через тикеты
	if err := r.db.WithContext(ctx).
		Table("ticket_assignments").
		Joins("JOIN tickets ON tickets.id = ticket_assignments.ticket_id").
		Where("tickets.cleaning_area_id = ?", id).
		Count(&deps.AssignmentsCount).Error; err != nil {
		return nil, err
	}

	// Считаем апелляции через тикеты
	if err := r.db.WithContext(ctx).
		Table("appeals").
		Joins("JOIN tickets ON tickets.id = appeals.ticket_id").
		Where("tickets.cleaning_area_id = ? AND appeals.ticket_id IS NOT NULL", id).
		Count(&deps.AppealsCount).Error; err != nil {
		return nil, err
	}

	// Считаем нарушения через рейсы и тикеты
	if err := r.db.WithContext(ctx).
		Table("violations").
		Joins("JOIN trips ON trips.id = violations.trip_id").
		Joins("JOIN tickets ON tickets.id = trips.ticket_id").
		Where("tickets.cleaning_area_id = ?", id).
		Count(&deps.ViolationsCount).Error; err != nil {
		return nil, err
	}

	// Считаем записи доступа (удалятся автоматически через CASCADE, но показываем для информации)
	if err := r.db.WithContext(ctx).
		Table("cleaning_area_access").
		Where("cleaning_area_id = ?", id).
		Count(&deps.AccessRecordsCount).Error; err != nil {
		return nil, err
	}

	return &deps, nil
}

func (r *CleaningAreaRepository) DeleteTicketsByAreaID(ctx context.Context, areaID uuid.UUID) error {
	// Удаляем тикеты, что каскадно удалит:
	// - ticket_assignments (ON DELETE CASCADE)
	// - appeals (ON DELETE CASCADE)
	// trips.ticket_id станет NULL (ON DELETE SET NULL)
	result := r.db.WithContext(ctx).
		Table("tickets").
		Where("cleaning_area_id = ?", areaID).
		Delete(nil)

	return result.Error
}

func (r *CleaningAreaRepository) HasAccessForDriver(ctx context.Context, areaID, driverID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM ticket_assignments ta
			JOIN tickets t ON t.id = ta.ticket_id
			WHERE ta.driver_id = ?
				AND ta.is_active = TRUE
				AND t.cleaning_area_id = ?
		)
	`, driverID, areaID).Scan(&exists).Error
	return exists, err
}

func serializeStatuses(values []model.CleaningAreaStatus) []string {
	result := make([]string, 0, len(values))
	for _, s := range values {
		result = append(result, string(s))
	}
	return result
}
