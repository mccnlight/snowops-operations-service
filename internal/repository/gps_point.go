package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/nurpe/snowops-operations/internal/model"
)

type GPSPointRepository struct {
	db *gorm.DB
}

func NewGPSPointRepository(db *gorm.DB) *GPSPointRepository {
	return &GPSPointRepository{db: db}
}

func (r *GPSPointRepository) Create(ctx context.Context, point *model.GPSPoint) error {
	return r.db.WithContext(ctx).Table("gps_points").Create(point).Error
}

func (r *GPSPointRepository) GetLatestByVehicle(ctx context.Context, vehicleID uuid.UUID) (*model.GPSPoint, error) {
	var point model.GPSPoint
	err := r.db.WithContext(ctx).
		Table("gps_points").
		Where("vehicle_id = ?", vehicleID).
		Order("captured_at DESC").
		First(&point).Error
	if err != nil {
		return nil, err
	}
	return &point, nil
}

func (r *GPSPointRepository) GetTrack(ctx context.Context, vehicleID uuid.UUID, from, to time.Time) ([]model.GPSPoint, error) {
	var points []model.GPSPoint
	err := r.db.WithContext(ctx).
		Table("gps_points").
		Where("vehicle_id = ? AND captured_at >= ? AND captured_at <= ?", vehicleID, from, to).
		Order("captured_at ASC").
		Find(&points).Error
	return points, err
}

func (r *GPSPointRepository) GetLatestForVehicles(ctx context.Context, vehicleIDs []uuid.UUID, maxAge time.Duration) (map[uuid.UUID]*model.GPSPoint, error) {
	if len(vehicleIDs) == 0 {
		return make(map[uuid.UUID]*model.GPSPoint), nil
	}

	cutoff := time.Now().Add(-maxAge)

	var points []model.GPSPoint
	err := r.db.WithContext(ctx).
		Table("gps_points").
		Where("vehicle_id IN ? AND captured_at >= ?", vehicleIDs, cutoff).
		Order("vehicle_id, captured_at DESC").
		Find(&points).Error

	if err != nil {
		return nil, err
	}

	result := make(map[uuid.UUID]*model.GPSPoint)
	seen := make(map[uuid.UUID]bool)

	for i := range points {
		if !seen[points[i].VehicleID] {
			result[points[i].VehicleID] = &points[i]
			seen[points[i].VehicleID] = true
		}
	}

	return result, nil
}

func (r *GPSPointRepository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Table("gps_points").
		Where("captured_at < ?", cutoff).
		Delete(&model.GPSPoint{})
	return result.RowsAffected, result.Error
}

