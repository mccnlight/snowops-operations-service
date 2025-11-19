package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/nurpe/snowops-operations/internal/model"
	"github.com/nurpe/snowops-operations/internal/repository"
)

const (
	SpeedKmh = 20.0
	SpeedMs  = SpeedKmh / 3.6 // ~5.55 м/с
)

var (
	DistancePerTick float64
)

type LatLon struct {
	Lat float64
	Lon float64
}

type GPSSimulator struct {
	db               *repository.GPSPointRepository
	vehicleRepo      *repository.VehicleRepository
	areaRepo         *repository.CleaningAreaRepository
	polygonRepo      *repository.PolygonRepository
	cameraRepo       *repository.CameraRepository
	log              zerolog.Logger
	osmFile          string
	updateInterval   time.Duration
	cleanupDays      int
	roads            []Road
	currentRoad      *Road
	currentIndex     int
	currentPos       float64 // позиция на текущем сегменте (0.0 - 1.0)
	vehicleID        uuid.UUID
	wasInPolygon     bool       // флаг для отслеживания входа в полигон
	currentPolygonID *uuid.UUID // ID текущего полигона, если внутри
	ctx              context.Context
	cancel           context.CancelFunc
}

type Road struct {
	Nodes []LatLon
	Name  string
}

func NewGPSSimulator(
	db *repository.GPSPointRepository,
	vehicleRepo *repository.VehicleRepository,
	areaRepo *repository.CleaningAreaRepository,
	polygonRepo *repository.PolygonRepository,
	cameraRepo *repository.CameraRepository,
	log zerolog.Logger,
	osmFile string,
	updateInterval time.Duration,
	cleanupDays int,
) *GPSSimulator {
	ctx, cancel := context.WithCancel(context.Background())

	// Вычисляем расстояние за тик
	DistancePerTick = SpeedMs * updateInterval.Seconds()

	return &GPSSimulator{
		db:             db,
		vehicleRepo:    vehicleRepo,
		areaRepo:       areaRepo,
		polygonRepo:    polygonRepo,
		cameraRepo:     cameraRepo,
		log:            log,
		osmFile:        osmFile,
		updateInterval: updateInterval,
		cleanupDays:    cleanupDays,
		wasInPolygon:   false,
		ctx:            ctx,
		cancel:         cancel,
	}
}

func (s *GPSSimulator) Start() error {
	// Получаем или создаём тестовую машину
	vehicle, err := s.vehicleRepo.GetOrCreateTestVehicle(s.ctx)
	if err != nil {
		return fmt.Errorf("failed to get test vehicle: %w", err)
	}
	s.vehicleID = vehicle.ID

	// Загружаем захардкоженный маршрут
	s.loadHardcodedRoute()

	if len(s.roads) == 0 {
		return fmt.Errorf("no roads found")
	}

	// Валидация начальной точки - проверяем, что она находится в участке уборки
	startPoint := s.roads[0].Nodes[0]
	area, err := s.areaRepo.FindAreaContainingPoint(s.ctx, startPoint.Lat, startPoint.Lon)
	if err != nil {
		s.log.Warn().
			Float64("lat", startPoint.Lat).
			Float64("lon", startPoint.Lon).
			Err(err).
			Msg("start point is not in any cleaning area, continuing anyway")
	} else {
		s.log.Info().
			Str("area_id", area.ID.String()).
			Str("area_name", area.Name).
			Float64("lat", startPoint.Lat).
			Float64("lon", startPoint.Lon).
			Msg("start point validated - inside cleaning area")
	}

	// Выбираем первую дорогу (захардкоженный маршрут)
	s.selectRandomRoad()

	// Запускаем симуляцию
	go s.run()

	s.log.Info().
		Str("vehicle_id", s.vehicleID.String()).
		Int("roads_count", len(s.roads)).
		Msg("GPS simulator started")

	return nil
}

func (s *GPSSimulator) Stop() {
	s.cancel()
	s.log.Info().Msg("GPS simulator stopped")
}

func (s *GPSSimulator) loadHardcodedRoute() {
	// Захардкоженный маршрут для симуляции
	// Начальная точка: 54.842920/69.207121
	// Маршрут: список промежуточных точек
	// Конечная точка: 54.841848/69.264708
	s.roads = []Road{
		{
			Name: "Hardcoded Simulation Route",
			Nodes: []LatLon{
				// Начальная точка
				{Lat: 54.842920, Lon: 69.207121},
				// Промежуточные точки маршрута
				{Lat: 54.843342, Lon: 69.209881},
				{Lat: 54.843009, Lon: 69.213915},
				{Lat: 54.842807, Lon: 69.216831},
				{Lat: 54.842608, Lon: 69.219229},
				{Lat: 54.842308, Lon: 69.220744},
				{Lat: 54.841766, Lon: 69.222453},
				{Lat: 54.841165, Lon: 69.223885},
				{Lat: 54.840893, Lon: 69.224845},
				{Lat: 54.840708, Lon: 69.225808},
				{Lat: 54.840587, Lon: 69.227077},
				{Lat: 54.840617, Lon: 69.228198},
				{Lat: 54.840793, Lon: 69.229598},
				{Lat: 54.841392, Lon: 69.231320},
				{Lat: 54.84817, Lon: 69.24148},
				{Lat: 54.846661, Lon: 69.262061},
				{Lat: 54.846427, Lon: 69.261519},
				{Lat: 54.841569, Lon: 69.265569},
				// Конечная точка
				{Lat: 54.841848, Lon: 69.264708},
			},
		},
	}
}

func (s *GPSSimulator) selectRandomRoad() {
	if len(s.roads) == 0 {
		return
	}
	// Выбираем первую дорогу (можно сделать случайный выбор)
	s.currentRoad = &s.roads[0]
	s.currentIndex = 0
	s.currentPos = 0.0
}

func (s *GPSSimulator) run() {
	ticker := time.NewTicker(s.updateInterval)
	defer ticker.Stop()

	// Запускаем очистку старых данных, если настроено
	if s.cleanupDays > 0 {
		go s.cleanupOldPoints()
	}

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.updatePosition(); err != nil {
				s.log.Error().Err(err).Msg("failed to update GPS position")
			}
		}
	}
}

func (s *GPSSimulator) updatePosition() error {
	if s.currentRoad == nil || len(s.currentRoad.Nodes) < 2 {
		return fmt.Errorf("invalid road")
	}

	// Вычисляем следующую позицию
	segment := s.getCurrentSegment()
	if segment == nil {
		// Переходим на следующий сегмент
		s.currentIndex++
		if s.currentIndex >= len(s.currentRoad.Nodes)-1 {
			// Достигли конца дороги, выбираем новую
			s.selectRandomRoad()
			segment = s.getCurrentSegment()
		} else {
			segment = s.getCurrentSegment()
		}
		if segment == nil {
			return fmt.Errorf("no valid segment")
		}
	}

	// Вычисляем расстояние до следующей точки
	segmentLength := s.distance(segment.From, segment.To)
	distanceToMove := DistancePerTick

	// Если до конца сегмента осталось меньше, чем нужно пройти, переходим на следующий
	if (1.0-s.currentPos)*segmentLength < distanceToMove {
		s.currentIndex++
		s.currentPos = 0.0
		if s.currentIndex >= len(s.currentRoad.Nodes)-1 {
			// Конец дороги, выбираем новую
			s.selectRandomRoad()
			segment = s.getCurrentSegment()
			if segment == nil {
				return fmt.Errorf("no valid segment")
			}
		} else {
			segment = s.getCurrentSegment()
		}
	}

	// Вычисляем новую позицию
	progress := distanceToMove / segmentLength
	if progress > 1.0 {
		progress = 1.0
	}
	s.currentPos += progress

	// Интерполируем координаты
	lat := segment.From.Lat + (segment.To.Lat-segment.From.Lat)*s.currentPos
	lon := segment.From.Lon + (segment.To.Lon-segment.From.Lon)*s.currentPos

	// Вычисляем направление (heading)
	heading := s.calculateHeading(segment.From, segment.To)

	// Проверяем вход в полигон
	var lprEvent map[string]interface{}
	inPolygon := false
	var currentPolygonID *uuid.UUID

	// Получаем все активные полигоны
	polygons, err := s.polygonRepo.List(s.ctx, repository.PolygonFilter{OnlyActive: true})
	if err == nil {
		// Проверяем каждый полигон
		for _, polygon := range polygons {
			contains, err := s.polygonRepo.ContainsPoint(s.ctx, polygon.ID, lat, lon)
			if err == nil && contains {
				inPolygon = true
				currentPolygonID = &polygon.ID

				// Если только что вошли в полигон (были снаружи, теперь внутри)
				if !s.wasInPolygon {
					s.log.Info().
						Str("polygon_id", polygon.ID.String()).
						Str("polygon_name", polygon.Name).
						Float64("lat", lat).
						Float64("lon", lon).
						Msg("vehicle entered polygon - generating LPR event")

					// Ищем LPR камеру в полигоне
					var cameraID *uuid.UUID
					cameras, err := s.cameraRepo.ListByPolygon(s.ctx, polygon.ID)
					if err == nil {
						for _, camera := range cameras {
							if camera.IsActive && camera.Type == model.CameraTypeLPR {
								cameraID = &camera.ID
								break
							}
						}
					}

					// Формируем LPR событие
					lprEvent = map[string]interface{}{
						"polygon_id":   polygon.ID.String(),
						"polygon_name": polygon.Name,
						"event_type":   "ENTRY",
						"timestamp":    time.Now().Format(time.RFC3339),
					}
					if cameraID != nil {
						lprEvent["camera_id"] = cameraID.String()
					}
				}
				break
			}
		}
	}

	// Обновляем флаг
	s.wasInPolygon = inPolygon
	s.currentPolygonID = currentPolygonID

	// Создаём GPS точку
	point := &model.GPSPoint{
		ID:         uuid.New(),
		VehicleID:  s.vehicleID,
		CapturedAt: time.Now(),
		Lat:        lat,
		Lon:        lon,
		SpeedKmh:   SpeedKmh,
		HeadingDeg: heading,
	}

	// Добавляем метаданные о симуляции
	payload := map[string]interface{}{
		"simulated": true,
		"source":    "osm-simulator",
	}

	// Добавляем LPR событие, если оно есть
	if lprEvent != nil {
		payload["lpr_event"] = lprEvent
	}

	payloadJSON, _ := json.Marshal(payload)
	payloadStr := string(payloadJSON)
	point.RawPayload = &payloadStr

	// Сохраняем в БД
	if err := s.db.Create(s.ctx, point); err != nil {
		return fmt.Errorf("failed to save GPS point: %w", err)
	}

	return nil
}

type Segment struct {
	From LatLon
	To   LatLon
}

func (s *GPSSimulator) getCurrentSegment() *Segment {
	if s.currentRoad == nil || s.currentIndex >= len(s.currentRoad.Nodes)-1 {
		return nil
	}
	return &Segment{
		From: s.currentRoad.Nodes[s.currentIndex],
		To:   s.currentRoad.Nodes[s.currentIndex+1],
	}
}

func (s *GPSSimulator) distance(from, to LatLon) float64 {
	// Haversine формула для вычисления расстояния между двумя точками
	const earthRadius = 6371000 // метры

	lat1 := from.Lat * math.Pi / 180
	lat2 := to.Lat * math.Pi / 180
	deltaLat := (to.Lat - from.Lat) * math.Pi / 180
	deltaLon := (to.Lon - from.Lon) * math.Pi / 180

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

func (s *GPSSimulator) calculateHeading(from, to LatLon) float64 {
	lat1 := from.Lat * math.Pi / 180
	lat2 := to.Lat * math.Pi / 180
	deltaLon := (to.Lon - from.Lon) * math.Pi / 180

	y := math.Sin(deltaLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(deltaLon)

	heading := math.Atan2(y, x) * 180 / math.Pi
	heading = math.Mod(heading+360, 360) // Нормализуем в диапазон 0-360

	return heading
}

func (s *GPSSimulator) cleanupOldPoints() {
	if s.cleanupDays <= 0 {
		return
	}

	ticker := time.NewTicker(1 * time.Hour) // Проверяем каждый час
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().AddDate(0, 0, -s.cleanupDays)
			deleted, err := s.db.DeleteOlderThan(s.ctx, cutoff)
			if err != nil {
				s.log.Error().Err(err).Msg("failed to cleanup old GPS points")
			} else if deleted > 0 {
				s.log.Info().
					Int64("deleted", deleted).
					Time("cutoff", cutoff).
					Msg("cleaned up old GPS points")
			}
		}
	}
}
