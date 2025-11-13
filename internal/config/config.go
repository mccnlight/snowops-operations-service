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

type Config struct {
	Environment string
	HTTP        HTTPConfig
	DB          DBConfig
	Auth        AuthConfig
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
