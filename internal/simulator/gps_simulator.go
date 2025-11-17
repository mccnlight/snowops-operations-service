package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
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
	db            *repository.GPSPointRepository
	vehicleRepo   *repository.VehicleRepository
	log           zerolog.Logger
	osmFile       string
	updateInterval time.Duration
	cleanupDays    int
	roads         []Road
	currentRoad   *Road
	currentIndex  int
	currentPos    float64 // позиция на текущем сегменте (0.0 - 1.0)
	vehicleID     uuid.UUID
	ctx           context.Context
	cancel        context.CancelFunc
}

type Road struct {
	Nodes []LatLon
	Name  string
}

func NewGPSSimulator(
	db *repository.GPSPointRepository,
	vehicleRepo *repository.VehicleRepository,
	log zerolog.Logger,
	osmFile string,
	updateInterval time.Duration,
	cleanupDays int,
) *GPSSimulator {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Вычисляем расстояние за тик
	DistancePerTick = SpeedMs * updateInterval.Seconds()
	
	return &GPSSimulator{
		db:            db,
		vehicleRepo:   vehicleRepo,
		log:           log,
		osmFile:       osmFile,
		updateInterval: updateInterval,
		cleanupDays:   cleanupDays,
		ctx:           ctx,
		cancel:        cancel,
	}
}

func (s *GPSSimulator) Start() error {
	// Получаем или создаём тестовую машину
	vehicle, err := s.vehicleRepo.GetOrCreateTestVehicle(s.ctx)
	if err != nil {
		return fmt.Errorf("failed to get test vehicle: %w", err)
	}
	s.vehicleID = vehicle.ID

	// Загружаем дороги из OSM
	if err := s.loadRoads(); err != nil {
		s.log.Warn().Err(err).Msg("failed to load roads from OSM, using fallback route")
		// Используем простой маршрут по умолчанию (Петропавловск)
		s.roads = []Road{
			{
				Nodes: []LatLon{
					{Lat: 54.80, Lon: 69.00},
					{Lat: 54.85, Lon: 69.10},
					{Lat: 54.90, Lon: 69.20},
					{Lat: 54.95, Lon: 69.30},
				},
				Name: "Fallback Route",
			},
		}
	}

	if len(s.roads) == 0 {
		return fmt.Errorf("no roads found")
	}

	// Выбираем случайную дорогу
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

func (s *GPSSimulator) loadRoads() error {
	// Простой парсер OSM PBF - для MVP используем упрощённый подход
	// В реальности нужно использовать библиотеку типа github.com/qedus/osm
	// Но для демо создадим простой парсер, который ищет primary highways

	file, err := os.Open(s.osmFile)
	if err != nil {
		return fmt.Errorf("failed to open OSM file: %w", err)
	}
	defer file.Close()

	// Для MVP используем упрощённый подход - создаём несколько тестовых дорог
	// В продакшене нужно использовать полноценный парсер OSM PBF
	// Здесь создаём маршруты в районе Петропавловска
	s.roads = []Road{
		{
			Name: "Primary Highway 1",
			Nodes: []LatLon{
				{Lat: 54.8700, Lon: 69.1400},
				{Lat: 54.8720, Lon: 69.1450},
				{Lat: 54.8740, Lon: 69.1500},
				{Lat: 54.8760, Lon: 69.1550},
				{Lat: 54.8780, Lon: 69.1600},
				{Lat: 54.8800, Lon: 69.1650},
			},
		},
		{
			Name: "Primary Highway 2",
			Nodes: []LatLon{
				{Lat: 54.8600, Lon: 69.1300},
				{Lat: 54.8650, Lon: 69.1350},
				{Lat: 54.8700, Lon: 69.1400},
				{Lat: 54.8750, Lon: 69.1450},
				{Lat: 54.8800, Lon: 69.1500},
			},
		},
		{
			Name: "Primary Highway 3",
			Nodes: []LatLon{
				{Lat: 54.8500, Lon: 69.1200},
				{Lat: 54.8550, Lon: 69.1250},
				{Lat: 54.8600, Lon: 69.1300},
				{Lat: 54.8650, Lon: 69.1350},
				{Lat: 54.8700, Lon: 69.1400},
			},
		},
	}

	// TODO: В будущем здесь должен быть полноценный парсер OSM PBF
	// который будет искать ways с highway=primary и извлекать их nodes

	return nil
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

