package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/nurpe/snowops-operations/internal/model"
)

type CameraRepository struct {
	db *gorm.DB
}

func NewCameraRepository(db *gorm.DB) *CameraRepository {
	return &CameraRepository{db: db}
}

func (r *CameraRepository) ListByPolygon(ctx context.Context, polygonID uuid.UUID) ([]model.Camera, error) {
	var cameras []model.Camera
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			id,
			polygon_id,
			type::text AS type,
			name,
			ST_AsGeoJSON(location) AS location,
			is_active,
			created_at,
			updated_at
		FROM cameras
		WHERE polygon_id = ?
		ORDER BY created_at ASC
	`, polygonID).Scan(&cameras).Error
	if err != nil {
		return nil, err
	}
	return cameras, nil
}

func (r *CameraRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Camera, error) {
	var camera model.Camera
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			id,
			polygon_id,
			type::text AS type,
			name,
			ST_AsGeoJSON(location) AS location,
			is_active,
			created_at,
			updated_at
		FROM cameras
		WHERE id = ?
		LIMIT 1
	`, id).Scan(&camera).Error
	if err != nil {
		return nil, err
	}
	if camera.ID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &camera, nil
}

type CreateCameraParams struct {
	PolygonID uuid.UUID
	Type      model.CameraType
	Name      string
	Location  *string
	IsActive  bool
}

func (r *CameraRepository) Create(ctx context.Context, params CreateCameraParams) (*model.Camera, error) {
	var camera model.Camera
	var location interface{}
	if params.Location != nil {
		location = *params.Location
	}
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO cameras (polygon_id, type, name, location, is_active)
		VALUES (
			?, ?, ?, ST_SetSRID(ST_GeomFromGeoJSON(?), 4326), ?
		)
		RETURNING
			id,
			polygon_id,
			type::text AS type,
			name,
			ST_AsGeoJSON(location) AS location,
			is_active,
			created_at,
			updated_at
	`, params.PolygonID, params.Type, params.Name, location, params.IsActive).Scan(&camera).Error
	if err != nil {
		return nil, err
	}
	return &camera, nil
}

type UpdateCameraParams struct {
	ID       uuid.UUID
	Type     *model.CameraType
	Name     *string
	Location **string
	IsActive *bool
}

func (r *CameraRepository) Update(ctx context.Context, params UpdateCameraParams) (*model.Camera, error) {
	setParts := []string{"updated_at = NOW()"}
	values := make([]interface{}, 0, 5)

	if params.Type != nil {
		setParts = append(setParts, "type = ?")
		values = append(values, *params.Type)
	}
	if params.Name != nil {
		setParts = append(setParts, "name = ?")
		values = append(values, *params.Name)
	}
	if params.Location != nil {
		if *params.Location == nil {
			setParts = append(setParts, "location = NULL")
		} else {
			setParts = append(setParts, "location = ST_SetSRID(ST_GeomFromGeoJSON(?), 4326)")
			values = append(values, **params.Location)
		}
	}
	if params.IsActive != nil {
		setParts = append(setParts, "is_active = ?")
		values = append(values, *params.IsActive)
	}

	if len(setParts) == 1 {
		return r.GetByID(ctx, params.ID)
	}

	values = append(values, params.ID)

	query := fmt.Sprintf(`
		UPDATE cameras
		SET %s
		WHERE id = ?
		RETURNING
			id,
			polygon_id,
			type::text AS type,
			name,
			ST_AsGeoJSON(location) AS location,
			is_active,
			created_at,
			updated_at
	`, strings.Join(setParts, ", "))

	var camera model.Camera
	err := r.db.WithContext(ctx).Raw(query, values...).Scan(&camera).Error
	if err != nil {
		return nil, err
	}
	if camera.ID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &camera, nil
}
