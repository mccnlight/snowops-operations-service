package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PolygonAccessEntry struct {
	ID           uuid.UUID
	PolygonID    uuid.UUID
	ContractorID uuid.UUID
	Source       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	RevokedAt    *time.Time
}

type PolygonAccessRepository struct {
	db *gorm.DB
}

func NewPolygonAccessRepository(db *gorm.DB) *PolygonAccessRepository {
	return &PolygonAccessRepository{db: db}
}

func (r *PolygonAccessRepository) ListByPolygon(ctx context.Context, polygonID uuid.UUID) ([]PolygonAccessEntry, error) {
	var entries []PolygonAccessEntry
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			id,
			polygon_id,
			contractor_id,
			source,
			created_at,
			updated_at,
			revoked_at
		FROM polygon_access
		WHERE polygon_id = ?
		ORDER BY created_at DESC
	`, polygonID).Scan(&entries).Error
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *PolygonAccessRepository) Grant(ctx context.Context, polygonID, contractorID uuid.UUID, source string) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO polygon_access (polygon_id, contractor_id, source, revoked_at)
		VALUES (?, ?, ?, NULL)
		ON CONFLICT (polygon_id, contractor_id)
		DO UPDATE SET
			source = EXCLUDED.source,
			revoked_at = NULL,
			updated_at = NOW()
	`, polygonID, contractorID, source).Error
}

func (r *PolygonAccessRepository) Revoke(ctx context.Context, polygonID, contractorID uuid.UUID) error {
	return r.db.WithContext(ctx).Exec(`
		UPDATE polygon_access
		SET revoked_at = NOW()
		WHERE polygon_id = ? AND contractor_id = ? AND revoked_at IS NULL
	`, polygonID, contractorID).Error
}

func (r *PolygonAccessRepository) HasAccessForContractor(ctx context.Context, polygonID, contractorID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1 FROM polygon_access
			WHERE polygon_id = ?
				AND contractor_id = ?
				AND revoked_at IS NULL
		)
	`, polygonID, contractorID).Scan(&exists).Error
	return exists, err
}
