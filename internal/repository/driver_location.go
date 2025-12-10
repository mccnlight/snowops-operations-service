package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/nurpe/snowops-operations/internal/model"
)

type DriverLocationRepository struct {
	db *gorm.DB
}

func NewDriverLocationRepository(db *gorm.DB) *DriverLocationRepository {
	return &DriverLocationRepository{db: db}
}

func (r *DriverLocationRepository) UpsertLocation(ctx context.Context, location *model.DriverLocation) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO driver_locations (driver_id, lat, lon, accuracy, updated_at)
		VALUES (?, ?, ?, ?, NOW())
		ON CONFLICT (driver_id) DO UPDATE
		SET
			lat = EXCLUDED.lat,
			lon = EXCLUDED.lon,
			accuracy = EXCLUDED.accuracy,
			updated_at = NOW()
	`, location.DriverID, location.Lat, location.Lon, location.Accuracy).Error
}

func (r *DriverLocationRepository) GetByDriver(ctx context.Context, driverID uuid.UUID) (*model.DriverLocation, error) {
	var location model.DriverLocation
	err := r.db.WithContext(ctx).
		Table("driver_locations").
		Where("driver_id = ?", driverID).
		First(&location).Error
	if err != nil {
		return nil, err
	}
	return &location, nil
}

func (r *DriverLocationRepository) GetAll(ctx context.Context) ([]model.DriverLocation, error) {
	var locations []model.DriverLocation
	err := r.db.WithContext(ctx).
		Table("driver_locations").
		Find(&locations).Error
	if err != nil {
		return nil, err
	}
	return locations, nil
}

func (r *DriverLocationRepository) GetByContractor(ctx context.Context, contractorID uuid.UUID) ([]model.DriverLocation, error) {
	var locations []model.DriverLocation
	err := r.db.WithContext(ctx).
		Table("driver_locations dl").
		Joins("INNER JOIN drivers d ON d.id = dl.driver_id").
		Where("d.contractor_id = ?", contractorID).
		Select("dl.driver_id, dl.lat, dl.lon, dl.accuracy, dl.updated_at").
		Find(&locations).Error
	if err != nil {
		return nil, err
	}
	return locations, nil
}
