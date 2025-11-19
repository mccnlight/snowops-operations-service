package main

import (
	"fmt"
	"os"

	"github.com/nurpe/snowops-operations/internal/auth"
	"github.com/nurpe/snowops-operations/internal/config"
	"github.com/nurpe/snowops-operations/internal/db"
	httphandler "github.com/nurpe/snowops-operations/internal/http"
	"github.com/nurpe/snowops-operations/internal/http/middleware"
	"github.com/nurpe/snowops-operations/internal/logger"
	"github.com/nurpe/snowops-operations/internal/repository"
	"github.com/nurpe/snowops-operations/internal/service"
	"github.com/nurpe/snowops-operations/internal/simulator"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	appLogger := logger.New(cfg.Environment)

	database, err := db.New(cfg, appLogger)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to connect database")
	}

	areaRepo := repository.NewCleaningAreaRepository(database)
	polygonRepo := repository.NewPolygonRepository(database)
	cameraRepo := repository.NewCameraRepository(database)
	areaAccessRepo := repository.NewCleaningAreaAccessRepository(database)
	polygonAccessRepo := repository.NewPolygonAccessRepository(database)
	vehicleRepo := repository.NewVehicleRepository(database)
	gpsRepo := repository.NewGPSPointRepository(database)

	areaService := service.NewAreaService(
		areaRepo,
		areaAccessRepo,
		service.AreaFeatures{
			AllowAkimatWrite:             cfg.Features.AllowAkimatAreaWrite,
			AllowGeometryUpdateWhenInUse: cfg.Features.AllowAreaGeometryUpdateWhenInUse,
		},
	)
	polygonService := service.NewPolygonService(
		polygonRepo,
		cameraRepo,
		polygonAccessRepo,
		service.PolygonFeatures{
			AllowAkimatWrite: cfg.Features.AllowAkimatPolygonWrite,
		},
	)
	monitoringService := service.NewMonitoringService(
		vehicleRepo,
		gpsRepo,
		areaRepo,
		polygonRepo,
		areaAccessRepo,
	)

	tokenParser := auth.NewParser(cfg.Auth.AccessSecret)

	handler := httphandler.NewHandler(areaService, polygonService, monitoringService, appLogger)
	authMiddleware := middleware.Auth(tokenParser)
	router := httphandler.NewRouter(handler, authMiddleware, cfg.Environment)

	// Запускаем GPS-симулятор (если включен)
	if cfg.GPSSimulator.Enabled {
		osmFile := "kz_bbox.pbf"
		simulator := simulator.NewGPSSimulator(
			gpsRepo,
			vehicleRepo,
			areaRepo,
			polygonRepo,
			cameraRepo,
			appLogger,
			osmFile,
			cfg.GPSSimulator.UpdateInterval,
			cfg.GPSSimulator.CleanupDays,
		)
		if err := simulator.Start(); err != nil {
			appLogger.Warn().Err(err).Msg("failed to start GPS simulator")
		} else {
			defer simulator.Stop()
			appLogger.Info().
				Dur("interval", cfg.GPSSimulator.UpdateInterval).
				Int("cleanup_days", cfg.GPSSimulator.CleanupDays).
				Msg("GPS simulator started")
		}
	} else {
		appLogger.Info().Msg("GPS simulator disabled")
	}

	addr := fmt.Sprintf("%s:%d", cfg.HTTP.Host, cfg.HTTP.Port)
	appLogger.Info().Str("addr", addr).Msg("starting operations service")

	if err := router.Run(addr); err != nil {
		appLogger.Error().Err(err).Msg("failed to start server")
		os.Exit(1)
	}
}
