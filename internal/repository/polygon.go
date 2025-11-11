package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/nurpe/snowops-operations/internal/model"
)

type PolygonFilter struct {
	OnlyActive bool
}

type PolygonRepository struct {
	db *gorm.DB
}

func NewPolygonRepository(db *gorm.DB) *PolygonRepository {
	return &PolygonRepository{db: db}
}

func (r *PolygonRepository) List(ctx context.Context, filter PolygonFilter) ([]model.Polygon, error) {
	query := r.db.WithContext(ctx).Table("polygons p").
		Select(`
			p.id,
			p.name,
			p.address,
			ST_AsGeoJSON(p.geometry) AS geometry,
			p.is_active,
			p.created_at,
			p.updated_at,
			COALESCE(c.cnt, 0) AS camera_count
		`).
		Joins(`
			LEFT JOIN (
				SELECT polygon_id, COUNT(*) AS cnt
				FROM cameras
				WHERE is_active = TRUE
				GROUP BY polygon_id
			) c ON c.polygon_id = p.id
		`).
		Order("p.name ASC")

	if filter.OnlyActive {
		query = query.Where("p.is_active = TRUE")
	}

	var polygons []model.Polygon
	if err := query.Scan(&polygons).Error; err != nil {
		return nil, err
	}
	return polygons, nil
}

func (r *PolygonRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Polygon, error) {
	var polygon model.Polygon
	err := r.db.WithContext(ctx).
		Raw(`
			SELECT
				p.id,
				p.name,
				p.address,
				ST_AsGeoJSON(p.geometry) AS geometry,
				p.is_active,
				p.created_at,
				p.updated_at,
				COALESCE(c.cnt, 0) AS camera_count
			FROM polygons p
			LEFT JOIN (
				SELECT polygon_id, COUNT(*) AS cnt
				FROM cameras
				WHERE is_active = TRUE
				GROUP BY polygon_id
			) c ON c.polygon_id = p.id
			WHERE p.id = ?
			LIMIT 1
		`, id).Scan(&polygon).Error
	if err != nil {
		return nil, err
	}
	if polygon.ID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &polygon, nil
}

type CreatePolygonParams struct {
	Name     string
	Address  *string
	Geometry string
	IsActive bool
}

func (r *PolygonRepository) Create(ctx context.Context, params CreatePolygonParams) (*model.Polygon, error) {
	var polygon model.Polygon
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO polygons (name, address, geometry, is_active)
		VALUES (?, ?, ST_SetSRID(ST_GeomFromGeoJSON(?), 4326), ?)
		RETURNING
			id,
			name,
			address,
			ST_AsGeoJSON(geometry) AS geometry,
			is_active,
			created_at,
			updated_at
	`, params.Name, params.Address, params.Geometry, params.IsActive).Scan(&polygon).Error
	if err != nil {
		return nil, err
	}
	return &polygon, nil
}

type UpdatePolygonParams struct {
	ID       uuid.UUID
	Name     *string
	Address  **string
	IsActive *bool
}

func (r *PolygonRepository) UpdateMetadata(ctx context.Context, params UpdatePolygonParams) (*model.Polygon, error) {
	setClauses := []string{"updated_at = NOW()"}
	values := make([]interface{}, 0, 4)

	if params.Name != nil {
		setClauses = append(setClauses, "name = ?")
		values = append(values, *params.Name)
	}
	if params.Address != nil {
		if *params.Address == nil {
			setClauses = append(setClauses, "address = NULL")
		} else {
			setClauses = append(setClauses, "address = ?")
			values = append(values, **params.Address)
		}
	}
	if params.IsActive != nil {
		setClauses = append(setClauses, "is_active = ?")
		values = append(values, *params.IsActive)
	}

	values = append(values, params.ID)

	query := fmt.Sprintf(`
		UPDATE polygons
		SET %s
		WHERE id = ?
		RETURNING
			id,
			name,
			address,
			ST_AsGeoJSON(geometry) AS geometry,
			is_active,
			created_at,
			updated_at
	`, strings.Join(setClauses, ", "))

	var polygon model.Polygon
	err := r.db.WithContext(ctx).Raw(query, values...).Scan(&polygon).Error
	if err != nil {
		return nil, err
	}
	if polygon.ID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &polygon, nil
}

func (r *PolygonRepository) UpdateGeometry(ctx context.Context, id uuid.UUID, geoJSON string) (*model.Polygon, error) {
	var polygon model.Polygon
	err := r.db.WithContext(ctx).Raw(`
		UPDATE polygons
		SET
			geometry = ST_SetSRID(ST_GeomFromGeoJSON(?), 4326),
			updated_at = NOW()
		WHERE id = ?
		RETURNING
			id,
			name,
			address,
			ST_AsGeoJSON(geometry) AS geometry,
			is_active,
			created_at,
			updated_at
	`, geoJSON, id).Scan(&polygon).Error
	if err != nil {
		return nil, err
	}
	if polygon.ID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &polygon, nil
}
