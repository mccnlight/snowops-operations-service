package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CleaningAreaAccessEntry struct {
	ID             uuid.UUID
	CleaningAreaID uuid.UUID
	ContractorID   uuid.UUID
	Source         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	RevokedAt      *time.Time
}

type CleaningAreaAccessRepository struct {
	db *gorm.DB
}

func NewCleaningAreaAccessRepository(db *gorm.DB) *CleaningAreaAccessRepository {
	return &CleaningAreaAccessRepository{db: db}
}

func (r *CleaningAreaAccessRepository) ListByArea(ctx context.Context, areaID uuid.UUID) ([]CleaningAreaAccessEntry, error) {
	var entries []CleaningAreaAccessEntry
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			id,
			cleaning_area_id,
			contractor_id,
			source,
			created_at,
			updated_at,
			revoked_at
		FROM cleaning_area_access
		WHERE cleaning_area_id = ?
		ORDER BY created_at DESC
	`, areaID).Scan(&entries).Error
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *CleaningAreaAccessRepository) Grant(ctx context.Context, areaID, contractorID uuid.UUID, source string) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO cleaning_area_access (cleaning_area_id, contractor_id, source, revoked_at)
		VALUES (?, ?, ?, NULL)
		ON CONFLICT (cleaning_area_id, contractor_id)
		DO UPDATE SET
			source = EXCLUDED.source,
			revoked_at = NULL,
			updated_at = NOW()
	`, areaID, contractorID, source).Error
}

func (r *CleaningAreaAccessRepository) Revoke(ctx context.Context, areaID, contractorID uuid.UUID) error {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE cleaning_area_access
		SET revoked_at = NOW()
		WHERE cleaning_area_id = ? AND contractor_id = ? AND revoked_at IS NULL
	`, areaID, contractorID)
	return result.Error
}

func (r *CleaningAreaAccessRepository) HasActiveEntries(ctx context.Context, areaID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM cleaning_area_access
			WHERE cleaning_area_id = ?
				AND revoked_at IS NULL
		)
	`, areaID).Scan(&exists).Error
	return exists, err
}

func (r *CleaningAreaAccessRepository) HasAccessForContractor(ctx context.Context, areaID, contractorID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM cleaning_area_access
			WHERE cleaning_area_id = ?
				AND contractor_id = ?
				AND revoked_at IS NULL
		)
	`, areaID, contractorID).Scan(&exists).Error
	return exists, err
}
