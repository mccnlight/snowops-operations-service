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

func serializeStatuses(values []model.CleaningAreaStatus) []string {
	result := make([]string, 0, len(values))
	for _, s := range values {
		result = append(result, string(s))
	}
	return result
}
