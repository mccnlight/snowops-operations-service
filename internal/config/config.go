package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type HTTPConfig struct {
	Host string
	Port int
}

type DBConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type AuthConfig struct {
	AccessSecret string
}

type FeatureFlags struct {
	AllowAkimatAreaWrite             bool
	AllowAkimatPolygonWrite          bool
	AllowAreaGeometryUpdateWhenInUse bool
}

type GPSSimulatorConfig struct {
	Enabled      bool
	UpdateInterval time.Duration
	CleanupDays    int // Автоматическая очистка точек старше N дней (0 = отключено)
}

type Config struct {
	Environment string
	HTTP        HTTPConfig
	DB          DBConfig
	Auth        AuthConfig
	Features    FeatureFlags
	GPSSimulator GPSSimulatorConfig
}

func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName("app")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("./deploy")
	v.AddConfigPath("./internal/config")

	v.AutomaticEnv()

	_ = v.ReadInConfig()

	cfg := &Config{
		Environment: v.GetString("APP_ENV"),
		HTTP: HTTPConfig{
			Host: v.GetString("HTTP_HOST"),
			Port: v.GetInt("HTTP_PORT"),
		},
		DB: DBConfig{
			DSN:             v.GetString("DB_DSN"),
			MaxOpenConns:    v.GetInt("DB_MAX_OPEN_CONNS"),
			MaxIdleConns:    v.GetInt("DB_MAX_IDLE_CONNS"),
			ConnMaxLifetime: v.GetDuration("DB_CONN_MAX_LIFETIME"),
		},
		Auth: AuthConfig{
			AccessSecret: v.GetString("JWT_ACCESS_SECRET"),
		},
		Features: FeatureFlags{
			AllowAkimatAreaWrite:             v.GetBool("FEATURE_ALLOW_AKIMAT_AREA_WRITE"),
			AllowAkimatPolygonWrite:          v.GetBool("FEATURE_ALLOW_AKIMAT_POLYGON_WRITE"),
			AllowAreaGeometryUpdateWhenInUse: v.GetBool("FEATURE_ALLOW_AREA_GEOMETRY_UPDATE_WHEN_IN_USE"),
		},
		GPSSimulator: GPSSimulatorConfig{
			Enabled:       getBoolWithDefault(v, "GPS_SIMULATOR_ENABLED", v.GetString("APP_ENV") == "development"),
			UpdateInterval: getDurationWithDefault(v, "GPS_SIMULATOR_INTERVAL", 5*time.Second),
			CleanupDays:    getIntWithDefault(v, "GPS_SIMULATOR_CLEANUP_DAYS", 7),
		},
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validate(cfg *Config) error {
	if cfg.DB.DSN == "" {
		return fmt.Errorf("DB_DSN is required")
	}
	if cfg.Auth.AccessSecret == "" {
		return fmt.Errorf("JWT_ACCESS_SECRET is required")
	}
	if cfg.HTTP.Host == "" {
		return fmt.Errorf("HTTP_HOST is required")
	}
	if cfg.HTTP.Port == 0 {
		return fmt.Errorf("HTTP_PORT is required")
	}
	return nil
}

func getDurationWithDefault(v *viper.Viper, key string, defaultValue time.Duration) time.Duration {
	if v.IsSet(key) {
		return v.GetDuration(key)
	}
	return defaultValue
}

func getBoolWithDefault(v *viper.Viper, key string, defaultValue bool) bool {
	if v.IsSet(key) {
		return v.GetBool(key)
	}
	return defaultValue
}

func getIntWithDefault(v *viper.Viper, key string, defaultValue int) int {
	if v.IsSet(key) {
		return v.GetInt(key)
	}
	return defaultValue
}
