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
	OnlyActive     bool
	ContractorID   *uuid.UUID
	OrganizationID *uuid.UUID // Для фильтрации по LANDFILL организации
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
			p.organization_id,
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

	if filter.OrganizationID != nil {
		query = query.Where("p.organization_id = ?", *filter.OrganizationID)
	}

	if filter.ContractorID != nil {
		query = query.Where(`
			EXISTS (
				SELECT 1
				FROM polygon_access pa
				WHERE pa.polygon_id = p.id
					AND pa.contractor_id = ?
					AND pa.revoked_at IS NULL
			)
		`, *filter.ContractorID)
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
				p.organization_id,
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
	Name           string
	Address        *string
	Geometry       string
	OrganizationID *uuid.UUID // Для LANDFILL организаций
	IsActive       bool
}

func (r *PolygonRepository) Create(ctx context.Context, params CreatePolygonParams) (*model.Polygon, error) {
	var polygon model.Polygon
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO polygons (name, address, geometry, organization_id, is_active)
		VALUES (?, ?, ST_SetSRID(ST_GeomFromGeoJSON(?), 4326), ?, ?)
		RETURNING
			id,
			name,
			address,
			ST_AsGeoJSON(geometry) AS geometry,
			organization_id,
			is_active,
			created_at,
			updated_at
	`, params.Name, params.Address, params.Geometry, params.OrganizationID, params.IsActive).Scan(&polygon).Error
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
			organization_id,
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
			organization_id,
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

func (r *PolygonRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Table("polygons").
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

func (r *PolygonRepository) HasRelatedTrips(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("trips").
		Where("polygon_id = ?", id).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PolygonRepository) ContainsPoint(ctx context.Context, polygonID uuid.UUID, lat, lng float64) (bool, error) {
	var contains bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT ST_Contains(
			(SELECT geometry FROM polygons WHERE id = ?),
			ST_SetSRID(ST_MakePoint(?, ?), 4326)
		)
	`, polygonID, lng, lat).Scan(&contains).Error
	if err != nil {
		return false, err
	}
	return contains, nil
}

// GetContractorIDForDriver returns the contractor_id for a given driver_id
func (r *PolygonRepository) GetContractorIDForDriver(ctx context.Context, driverID uuid.UUID) (*uuid.UUID, error) {
	var result struct {
		ContractorID *uuid.UUID `gorm:"column:contractor_id"`
	}

	err := r.db.WithContext(ctx).
		Raw(`SELECT contractor_id FROM drivers WHERE id = ?`, driverID).
		Scan(&result).Error
	if err != nil {
		return nil, err
	}

	return result.ContractorID, nil
}
