package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                      string
	ILOHost                   string
	ILOUsername               string
	ILOPassword               string
	ILOSSHKexAlgos            []string
	ILOSSHHostKeyAlgos        []string
	ILOSSHPubkeyAcceptedAlgos []string
	ILOSSHCiphers             []string
	ILOSSHMACs                []string
	ILOSNMPHost               string
	ILOSNMPPort               uint16
	ILOSNMPCommunity          string
	ILOSNMPVersion            string
	ILOSNMPTimeoutSeconds     int
	ILOSNMPRetries            int
	MinimumFanSpeed           int
	FanApplyTimeoutSeconds    int
	FanApplyTolerance         int
	AllowInsecureTLS          bool
	DatabaseURL               string
}

func Load() (Config, error) {
	cfg := Config{
		Port:                      getEnv("PORT", "3000"),
		ILOHost:                   strings.TrimSpace(os.Getenv("ILO_HOST")),
		ILOUsername:               strings.TrimSpace(os.Getenv("ILO_USERNAME")),
		ILOPassword:               os.Getenv("ILO_PASSWORD"),
		ILOSSHKexAlgos:            getEnvList("ILO_SSH_KEX_ALGORITHMS"),
		ILOSSHHostKeyAlgos:        getEnvList("ILO_SSH_HOST_KEY_ALGORITHMS"),
		ILOSSHPubkeyAcceptedAlgos: getEnvList("ILO_SSH_PUBKEY_ACCEPTED_ALGORITHMS"),
		ILOSSHCiphers:             getEnvList("ILO_SSH_CIPHERS"),
		ILOSSHMACs:                getEnvList("ILO_SSH_MACS"),
		ILOSNMPHost:               getEnv("ILO_SNMP_HOST", strings.TrimSpace(os.Getenv("ILO_HOST"))),
		ILOSNMPPort:               uint16(getEnvInt("ILO_SNMP_PORT", 161)),
		ILOSNMPCommunity:          getEnv("ILO_SNMP_COMMUNITY", "public"),
		ILOSNMPVersion:            getEnv("ILO_SNMP_VERSION", "2c"),
		ILOSNMPTimeoutSeconds:     getEnvInt("ILO_SNMP_TIMEOUT_SECONDS", 5),
		ILOSNMPRetries:            getEnvInt("ILO_SNMP_RETRIES", 1),
		MinimumFanSpeed:           getEnvInt("MINIMUM_FAN_SPEED", 10),
		FanApplyTimeoutSeconds:    getEnvInt("FAN_APPLY_TIMEOUT_SECONDS", 30),
		FanApplyTolerance:         getEnvInt("FAN_APPLY_TOLERANCE", 2),
		AllowInsecureTLS:          getEnvBool("ILO_INSECURE_TLS", true),
		DatabaseURL:               strings.TrimSpace(os.Getenv("DATABASE_URL")),
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

	if cfg.FanApplyTimeoutSeconds < 1 || cfg.FanApplyTimeoutSeconds > 600 {
		return Config{}, fmt.Errorf("FAN_APPLY_TIMEOUT_SECONDS must be between 1 and 600")
	}

	if cfg.FanApplyTolerance < 0 || cfg.FanApplyTolerance > 20 {
		return Config{}, fmt.Errorf("FAN_APPLY_TOLERANCE must be between 0 and 20")
	}

	return cfg, nil
}

func (c Config) HasILOConfig() bool {
	return c.ILOHost != "" && c.ILOUsername != "" && c.ILOPassword != ""
}

func (c Config) HasILOSNMPConfig() bool {
	return c.ILOSNMPHost != "" && c.ILOSNMPCommunity != ""
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

func getEnvList(key string) []string {
	rawValue := strings.TrimSpace(os.Getenv(key))
	if rawValue == "" {
		return nil
	}

	parts := strings.Split(rawValue, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}

	if len(values) == 0 {
		return nil
	}

	return values
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
