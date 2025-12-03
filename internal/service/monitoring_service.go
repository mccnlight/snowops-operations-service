package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/nurpe/snowops-operations/internal/model"
	"github.com/nurpe/snowops-operations/internal/repository"
)

type MonitoringService struct {
	vehicleRepo    *repository.VehicleRepository
	gpsRepo        *repository.GPSPointRepository
	areaRepo       *repository.CleaningAreaRepository
	polygonRepo    *repository.PolygonRepository
	areaAccessRepo *repository.CleaningAreaAccessRepository
}

func NewMonitoringService(
	vehicleRepo *repository.VehicleRepository,
	gpsRepo *repository.GPSPointRepository,
	areaRepo *repository.CleaningAreaRepository,
	polygonRepo *repository.PolygonRepository,
	areaAccessRepo *repository.CleaningAreaAccessRepository,
) *MonitoringService {
	return &MonitoringService{
		vehicleRepo:    vehicleRepo,
		gpsRepo:        gpsRepo,
		areaRepo:       areaRepo,
		polygonRepo:    polygonRepo,
		areaAccessRepo: areaAccessRepo,
	}
}

type VehicleLiveData struct {
	VehicleID      uuid.UUID           `json:"vehicle_id"`
	PlateNumber    string              `json:"plate_number"`
	ContractorID   *uuid.UUID          `json:"contractor_id,omitempty"`
	ContractorName *string             `json:"contractor_name,omitempty"`
	LastGPS        *GPSPointData       `json:"last_gps,omitempty"`
	LastTicketID   *uuid.UUID          `json:"last_ticket_id,omitempty"`
	LastAreaID     *uuid.UUID          `json:"last_cleaning_area_id,omitempty"`
	LastPolygonID  *uuid.UUID          `json:"last_polygon_id,omitempty"`
	Status         model.VehicleStatus `json:"status"`
}

type GPSPointData struct {
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	CapturedAt  string  `json:"captured_at"`
	SpeedKmh    float64 `json:"speed_kmh"`
	HeadingDeg  float64 `json:"heading_deg"`
	IsSimulated bool    `json:"is_simulated"`
}

type VehiclesLiveInput struct {
	BBox         *BBox
	ContractorID *uuid.UUID
}

type BBox struct {
	MinLat float64
	MinLon float64
	MaxLat float64
	MaxLon float64
}

func (s *MonitoringService) GetVehiclesLive(ctx context.Context, principal model.Principal, input VehiclesLiveInput) ([]VehicleLiveData, error) {
	// Определяем, какие машины видит пользователь
	var vehicleIDs []uuid.UUID
	var vehicles []model.Vehicle

	if principal.IsAkimat() || principal.IsKgu() {
		// Видят все машины
		var err error
		vehicles, err = s.vehicleRepo.List(ctx, nil, false)
		if err != nil {
			return nil, err
		}
	} else if principal.IsTechnicalOperator() {
		// TOO видит все машины (но не участки)
		var err error
		vehicles, err = s.vehicleRepo.List(ctx, nil, false)
		if err != nil {
			return nil, err
		}
	} else if principal.IsContractor() {
		// Подрядчик видит только свои машины
		var err error
		vehicles, err = s.vehicleRepo.List(ctx, &principal.OrganizationID, false)
		if err != nil {
			return nil, err
		}
	} else if principal.IsDriver() {
		// Водитель видит только машины, связанные с его тикетами
		// Для MVP возвращаем пустой список (в будущем нужно интегрироваться с tickets service)
		vehicles = []model.Vehicle{}
	} else {
		return nil, ErrPermissionDenied
	}

	// Собираем ID машин
	vehicleIDs = make([]uuid.UUID, 0, len(vehicles))
	vehicleMap := make(map[uuid.UUID]model.Vehicle)
	for _, v := range vehicles {
		vehicleIDs = append(vehicleIDs, v.ID)
		vehicleMap[v.ID] = v
	}

	if len(vehicleIDs) == 0 {
		return []VehicleLiveData{}, nil
	}

	// Получаем последние GPS точки (не старше 5 минут)
	maxAge := 5 * time.Minute
	gpsPoints, err := s.gpsRepo.GetLatestForVehicles(ctx, vehicleIDs, maxAge)
	if err != nil {
		return nil, err
	}

	// Формируем ответ
	result := make([]VehicleLiveData, 0, len(vehicles))
	for _, vehicle := range vehicles {
		gpsPoint, hasGPS := gpsPoints[vehicle.ID]

		// Определяем статус
		status := model.VehicleStatusOffline
		if hasGPS {
			age := time.Since(gpsPoint.CapturedAt)
			if age < 2*time.Minute {
				status = model.VehicleStatusInTrip
			} else if age < 5*time.Minute {
				status = model.VehicleStatusIdle
			} else {
				status = model.VehicleStatusOffline
			}
		}

		vehicleData := VehicleLiveData{
			VehicleID:    vehicle.ID,
			PlateNumber:  vehicle.PlateNumber,
			ContractorID: vehicle.ContractorID,
			Status:       status,
		}

		if hasGPS {
			// Проверяем, симулирована ли точка
			isSimulated := false
			if gpsPoint.RawPayload != nil && *gpsPoint.RawPayload != "" {
				var payload map[string]interface{}
				if err := json.Unmarshal([]byte(*gpsPoint.RawPayload), &payload); err == nil {
					if sim, ok := payload["simulated"].(bool); ok && sim {
						isSimulated = true
					}
				}
			}

			vehicleData.LastGPS = &GPSPointData{
				Lat:         gpsPoint.Lat,
				Lon:         gpsPoint.Lon,
				CapturedAt:  gpsPoint.CapturedAt.Format(time.RFC3339),
				SpeedKmh:    gpsPoint.SpeedKmh,
				HeadingDeg:  gpsPoint.HeadingDeg,
				IsSimulated: isSimulated,
			}
		}

		// TODO: Добавить last_ticket_id, last_cleaning_area_id, last_polygon_id
		// через интеграцию с tickets service

		result = append(result, vehicleData)
	}

	return result, nil
}

type TrackPoint struct {
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	CapturedAt string  `json:"captured_at"`
	SpeedKmh   float64 `json:"speed_kmh"`
	HeadingDeg float64 `json:"heading_deg"`
}

type VehicleTrackInput struct {
	From time.Time
	To   time.Time
}

func (s *MonitoringService) GetVehicleTrack(ctx context.Context, principal model.Principal, vehicleID uuid.UUID, input VehicleTrackInput) ([]TrackPoint, error) {
	// Проверяем права доступа
	vehicle, err := s.vehicleRepo.GetByID(ctx, vehicleID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Проверяем, может ли пользователь видеть эту машину
	canView := false
	if principal.IsAkimat() || principal.IsKgu() || principal.IsTechnicalOperator() {
		canView = true
	} else if principal.IsContractor() {
		canView = vehicle.ContractorID != nil && *vehicle.ContractorID == principal.OrganizationID
	} else if principal.IsDriver() {
		// Водитель видит только свои машины (через тикеты)
		// Для MVP возвращаем ошибку
		return nil, ErrPermissionDenied
	}

	if !canView {
		return nil, ErrPermissionDenied
	}

	// Получаем трек
	points, err := s.gpsRepo.GetTrack(ctx, vehicleID, input.From, input.To)
	if err != nil {
		return nil, err
	}

	result := make([]TrackPoint, 0, len(points))
	for _, p := range points {
		result = append(result, TrackPoint{
			Lat:        p.Lat,
			Lon:        p.Lon,
			CapturedAt: p.CapturedAt.Format(time.RFC3339),
			SpeedKmh:   p.SpeedKmh,
			HeadingDeg: p.HeadingDeg,
		})
	}

	return result, nil
}

func (s *MonitoringService) DeleteOldGPSPoints(ctx context.Context, principal model.Principal, olderThan time.Time) (int64, error) {
	// Only KGU and Akimat can delete GPS points
	if !principal.IsKgu() && !principal.IsAkimat() {
		return 0, ErrPermissionDenied
	}

	// Ensure olderThan is in the past
	if olderThan.After(time.Now()) {
		return 0, ErrInvalidInput
	}

	// Delete GPS points older than the specified time
	deleted, err := s.gpsRepo.DeleteOlderThan(ctx, olderThan)
	if err != nil {
		return 0, err
	}

	return deleted, nil
}
