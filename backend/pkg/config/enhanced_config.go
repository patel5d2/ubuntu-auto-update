package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// Config holds all application configuration
type Config struct {
	Server   ServerConfig   `json:"server"`
	Database DatabaseConfig `json:"database"`
	Redis    RedisConfig    `json:"redis"`
	Auth     AuthConfig     `json:"auth"`
	Security SecurityConfig `json:"security"`
	Logging  LoggingConfig  `json:"logging"`
	Features FeatureConfig  `json:"features"`
	Metrics  MetricsConfig  `json:"metrics"`
}

type ServerConfig struct {
	Port         int           `json:"port"`
	Host         string        `json:"host"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
	Environment  string        `json:"environment"`
}

type DatabaseConfig struct {
	URL             string        `json:"url"`
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time"`
	SSLMode         string        `json:"ssl_mode"`
}

type RedisConfig struct {
	URL         string        `json:"url"`
	Password    string        `json:"password"`
	DB          int           `json:"db"`
	MaxRetries  int           `json:"max_retries"`
	PoolSize    int           `json:"pool_size"`
	DialTimeout time.Duration `json:"dial_timeout"`
}

type AuthConfig struct {
	JWTSecret              string        `json:"jwt_secret"`
	TokenExpiry            time.Duration `json:"token_expiry"`
	RefreshTokenExpiry     time.Duration `json:"refresh_token_expiry"`
	PasswordMinLength      int           `json:"password_min_length"`
	SessionTimeout         time.Duration `json:"session_timeout"`
	MaxLoginAttempts       int           `json:"max_login_attempts"`
	LockoutDuration        time.Duration `json:"lockout_duration"`
	RequireStrongPasswords bool          `json:"require_strong_passwords"`
}

type SecurityConfig struct {
	EnableHTTPS           bool     `json:"enable_https"`
	TLSCertFile           string   `json:"tls_cert_file"`
	TLSKeyFile            string   `json:"tls_key_file"`
	EnableCORS            bool     `json:"enable_cors"`
	CORSAllowedOrigins    []string `json:"cors_allowed_origins"`
	EnableRateLimit       bool     `json:"enable_rate_limit"`
	RateLimitRequests     int      `json:"rate_limit_requests"`
	RateLimitWindow       time.Duration `json:"rate_limit_window"`
	EnableRequestLogging  bool     `json:"enable_request_logging"`
	TrustedProxies        []string `json:"trusted_proxies"`
	EnableCSRF            bool     `json:"enable_csrf"`
}

type LoggingConfig struct {
	Level      string `json:"level"`
	Format     string `json:"format"`
	OutputPath string `json:"output_path"`
	MaxSize    int    `json:"max_size"`
	MaxAge     int    `json:"max_age"`
	MaxBackups int    `json:"max_backups"`
	Compress   bool   `json:"compress"`
}

type FeatureConfig struct {
	EnableMetrics       bool `json:"enable_metrics"`
	EnablePprof         bool `json:"enable_pprof"`
	EnableWebhooks      bool `json:"enable_webhooks"`
	EnableSSHUpdates    bool `json:"enable_ssh_updates"`
	EnableAutoUpdates   bool `json:"enable_auto_updates"`
	EnableHealthChecks  bool `json:"enable_health_checks"`
}

type MetricsConfig struct {
	Enabled       bool   `json:"enabled"`
	Path          string `json:"path"`
	Port          int    `json:"port"`
	EnableRuntime bool   `json:"enable_runtime"`
	EnableCustom  bool   `json:"enable_custom"`
}

// LoadConfig loads configuration from environment variables with defaults
func LoadConfig() (*Config, error) {
	config := &Config{
		Server: ServerConfig{
			Port:         getEnvInt("API_PORT", 8080),
			Host:         getEnvString("API_HOST", "0.0.0.0"),
			ReadTimeout:  getEnvDuration("API_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getEnvDuration("API_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:  getEnvDuration("API_IDLE_TIMEOUT", 120*time.Second),
			Environment:  getEnvString("ENVIRONMENT", "development"),
		},
		Database: DatabaseConfig{
			URL:             getEnvString("DATABASE_URL", ""),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
			ConnMaxIdleTime: getEnvDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
			SSLMode:         getEnvString("DB_SSL_MODE", "disable"),
		},
		Redis: RedisConfig{
			URL:         getEnvString("REDIS_URL", "redis://localhost:6379"),
			Password:    getEnvString("REDIS_PASSWORD", ""),
			DB:          getEnvInt("REDIS_DB", 0),
			MaxRetries:  getEnvInt("REDIS_MAX_RETRIES", 3),
			PoolSize:    getEnvInt("REDIS_POOL_SIZE", 10),
			DialTimeout: getEnvDuration("REDIS_DIAL_TIMEOUT", 5*time.Second),
		},
		Auth: AuthConfig{
			JWTSecret:              getEnvString("JWT_SECRET", ""),
			TokenExpiry:            getEnvDuration("JWT_TOKEN_EXPIRY", 24*time.Hour),
			RefreshTokenExpiry:     getEnvDuration("JWT_REFRESH_TOKEN_EXPIRY", 7*24*time.Hour),
			PasswordMinLength:      getEnvInt("PASSWORD_MIN_LENGTH", 8),
			SessionTimeout:         getEnvDuration("SESSION_TIMEOUT", 30*time.Minute),
			MaxLoginAttempts:       getEnvInt("MAX_LOGIN_ATTEMPTS", 5),
			LockoutDuration:        getEnvDuration("LOCKOUT_DURATION", 15*time.Minute),
			RequireStrongPasswords: getEnvBool("REQUIRE_STRONG_PASSWORDS", true),
		},
		Security: SecurityConfig{
			EnableHTTPS:           getEnvBool("ENABLE_HTTPS", false),
			TLSCertFile:           getEnvString("TLS_CERT_FILE", ""),
			TLSKeyFile:            getEnvString("TLS_KEY_FILE", ""),
			EnableCORS:            getEnvBool("ENABLE_CORS", true),
			CORSAllowedOrigins:    getEnvStringSlice("CORS_ALLOWED_ORIGINS", []string{"*"}),
			EnableRateLimit:       getEnvBool("ENABLE_RATE_LIMIT", true),
			RateLimitRequests:     getEnvInt("RATE_LIMIT_REQUESTS", 100),
			RateLimitWindow:       getEnvDuration("RATE_LIMIT_WINDOW", time.Minute),
			EnableRequestLogging:  getEnvBool("ENABLE_REQUEST_LOGGING", true),
			TrustedProxies:        getEnvStringSlice("TRUSTED_PROXIES", []string{}),
			EnableCSRF:            getEnvBool("ENABLE_CSRF", false),
		},
		Logging: LoggingConfig{
			Level:      getEnvString("LOG_LEVEL", "info"),
			Format:     getEnvString("LOG_FORMAT", "json"),
			OutputPath: getEnvString("LOG_OUTPUT_PATH", ""),
			MaxSize:    getEnvInt("LOG_MAX_SIZE", 100),
			MaxAge:     getEnvInt("LOG_MAX_AGE", 30),
			MaxBackups: getEnvInt("LOG_MAX_BACKUPS", 3),
			Compress:   getEnvBool("LOG_COMPRESS", true),
		},
		Features: FeatureConfig{
			EnableMetrics:       getEnvBool("UAU_FEATURES__ENABLE_METRICS", true),
			EnablePprof:         getEnvBool("UAU_FEATURES__ENABLE_PPROF", false),
			EnableWebhooks:      getEnvBool("UAU_FEATURES__ENABLE_WEBHOOKS", true),
			EnableSSHUpdates:    getEnvBool("UAU_FEATURES__ENABLE_SSH_UPDATES", true),
			EnableAutoUpdates:   getEnvBool("UAU_FEATURES__ENABLE_AUTO_UPDATES", false),
			EnableHealthChecks:  getEnvBool("UAU_FEATURES__ENABLE_HEALTH_CHECKS", true),
		},
		Metrics: MetricsConfig{
			Enabled:       getEnvBool("METRICS_ENABLED", true),
			Path:          getEnvString("METRICS_PATH", "/metrics"),
			Port:          getEnvInt("METRICS_PORT", 9090),
			EnableRuntime: getEnvBool("METRICS_ENABLE_RUNTIME", true),
			EnableCustom:  getEnvBool("METRICS_ENABLE_CUSTOM", true),
		},
	}

	// Validate required configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	log.WithFields(log.Fields{
		"environment": config.Server.Environment,
		"port":        config.Server.Port,
		"log_level":   config.Logging.Level,
		"features":    config.Features,
	}).Info("Configuration loaded")

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Database.URL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	if c.Auth.JWTSecret == "" && c.Server.Environment == "production" {
		return fmt.Errorf("JWT_SECRET is required in production")
	}

	if c.Security.EnableHTTPS {
		if c.Security.TLSCertFile == "" || c.Security.TLSKeyFile == "" {
			return fmt.Errorf("TLS cert and key files are required when HTTPS is enabled")
		}
	}

	return nil
}

// Helper functions for environment variable parsing
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
		log.WithFields(log.Fields{
			"key":   key,
			"value": value,
		}).Warn("Invalid integer environment variable, using default")
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
		log.WithFields(log.Fields{
			"key":   key,
			"value": value,
		}).Warn("Invalid boolean environment variable, using default")
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
		log.WithFields(log.Fields{
			"key":   key,
			"value": value,
		}).Warn("Invalid duration environment variable, using default")
	}
	return defaultValue
}

func getEnvStringSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}