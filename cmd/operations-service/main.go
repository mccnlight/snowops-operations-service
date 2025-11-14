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

	tokenParser := auth.NewParser(cfg.Auth.AccessSecret)

	handler := httphandler.NewHandler(areaService, polygonService, appLogger)
	authMiddleware := middleware.Auth(tokenParser)
	router := httphandler.NewRouter(handler, authMiddleware, cfg.Environment)

	addr := fmt.Sprintf("%s:%d", cfg.HTTP.Host, cfg.HTTP.Port)
	appLogger.Info().Str("addr", addr).Msg("starting operations service")

	if err := router.Run(addr); err != nil {
		appLogger.Error().Err(err).Msg("failed to start server")
		os.Exit(1)
	}
}
