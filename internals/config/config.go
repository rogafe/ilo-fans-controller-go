package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port             string
	ILOHost          string
	ILOUsername      string
	ILOPassword      string
	MinimumFanSpeed  int
	AllowInsecureTLS bool
	DatabaseURL      string
}

func Load() (Config, error) {
	cfg := Config{
		Port:             getEnv("PORT", "3000"),
		ILOHost:          strings.TrimSpace(os.Getenv("ILO_HOST")),
		ILOUsername:      strings.TrimSpace(os.Getenv("ILO_USERNAME")),
		ILOPassword:      os.Getenv("ILO_PASSWORD"),
		MinimumFanSpeed:  getEnvInt("MINIMUM_FAN_SPEED", 10),
		AllowInsecureTLS: getEnvBool("ILO_INSECURE_TLS", true),
		DatabaseURL:      strings.TrimSpace(os.Getenv("DATABASE_URL")),
	}

	if cfg.DatabaseURL == "" {
		databaseURL, err := buildDatabaseURLFromParts()
		if err != nil {
			return Config{}, err
		}
		cfg.DatabaseURL = databaseURL
	}

	if cfg.MinimumFanSpeed < 0 || cfg.MinimumFanSpeed > 100 {
		return Config{}, fmt.Errorf("MINIMUM_FAN_SPEED must be between 0 and 100")
	}

	return cfg, nil
}

func (c Config) HasILOConfig() bool {
	return c.ILOHost != "" && c.ILOUsername != "" && c.ILOPassword != ""
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func getEnvInt(key string, fallback int) int {
	rawValue := strings.TrimSpace(os.Getenv(key))
	if rawValue == "" {
		return fallback
	}

	value, err := strconv.Atoi(rawValue)
	if err != nil {
		return fallback
	}

	return value
}

func getEnvBool(key string, fallback bool) bool {
	rawValue := strings.TrimSpace(os.Getenv(key))
	if rawValue == "" {
		return fallback
	}

	value, err := strconv.ParseBool(rawValue)
	if err != nil {
		return fallback
	}

	return value
}

func buildDatabaseURLFromParts() (string, error) {
	host := getEnv("DB_HOST", "127.0.0.1")
	port := getEnv("DB_PORT", "5432")
	user := strings.TrimSpace(os.Getenv("DB_USER"))
	password := os.Getenv("DB_PASSWORD")
	databaseName := strings.TrimSpace(os.Getenv("DB_NAME"))
	sslMode := getEnv("DB_SSLMODE", "disable")

	if user == "" || databaseName == "" {
		return "", fmt.Errorf("DATABASE_URL or DB_USER/DB_NAME must be configured")
	}

	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host,
		port,
		user,
		password,
		databaseName,
		sslMode,
	), nil
}
