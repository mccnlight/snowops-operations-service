package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/nurpe/snowops-operations/internal/model"
)

type VehicleRepository struct {
	db *gorm.DB
}

func NewVehicleRepository(db *gorm.DB) *VehicleRepository {
	return &VehicleRepository{db: db}
}

func (r *VehicleRepository) Create(ctx context.Context, vehicle *model.Vehicle) error {
	return r.db.WithContext(ctx).Table("vehicles").Create(vehicle).Error
}

func (r *VehicleRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Vehicle, error) {
	var vehicle model.Vehicle
	err := r.db.WithContext(ctx).
		Table("vehicles").
		Where("id = ?", id).
		First(&vehicle).Error
	if err != nil {
		return nil, err
	}
	return &vehicle, nil
}

func (r *VehicleRepository) List(ctx context.Context, contractorID *uuid.UUID, onlyActive bool) ([]model.Vehicle, error) {
	query := r.db.WithContext(ctx).Table("vehicles")

	if contractorID != nil {
		query = query.Where("contractor_id = ?", *contractorID)
	}

	if onlyActive {
		query = query.Where("is_active = TRUE")
	}

	var vehicles []model.Vehicle
	err := query.Find(&vehicles).Error
	return vehicles, err
}

func (r *VehicleRepository) GetOrCreateTestVehicle(ctx context.Context) (*model.Vehicle, error) {
	// Ищем тестовую машину
	var vehicle model.Vehicle
	err := r.db.WithContext(ctx).
		Table("vehicles").
		Where("plate_number = ?", "TEST-001").
		First(&vehicle).Error

	if err == nil {
		return &vehicle, nil
	}

	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// Создаём тестовую машину
	vehicle = model.Vehicle{
		ID:          uuid.New(),
		PlateNumber: "TEST-001",
		IsActive:    true,
	}
	if err := r.Create(ctx, &vehicle); err != nil {
		return nil, err
	}

	return &vehicle, nil
}

