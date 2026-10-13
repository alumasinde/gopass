package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

type Config struct {
	AppName, AppEnv, AppURL, Host, DBDSN, JWTSecret, JWTIssuer, LogLevel string
	Port                                                                 int
	AccessTTL, RefreshTTL                                                time.Duration
	CORS                                                                 []string
}

func Load() (Config, error) {
	_ = godotenv.Load()
	p, _ := strconv.Atoi(env("HTTP_PORT", "8080"))
	a, _ := time.ParseDuration(env("JWT_ACCESS_TTL", "15m"))
	r, _ := time.ParseDuration(env("JWT_REFRESH_TTL", "168h"))
	jwtSecret := env("JWT_SECRET", "")
	corsOrigins := csv(env("CORS_ALLOWED_ORIGINS", ""))

	cfg := Config{
		AppName:    env("APP_NAME", "gopass"),
		AppEnv:     env("APP_ENV", "development"),
		AppURL:     env("APP_URL", "http://localhost:8080"),
		Host:       env("HTTP_HOST", "0.0.0.0"),
		Port:       p,
		DBDSN:      DSN(),
		JWTSecret:  jwtSecret,
		JWTIssuer:  env("JWT_ISSUER", "gopass"),
		AccessTTL:  a,
		RefreshTTL: r,
		CORS:       corsOrigins,
		LogLevel:   env("LOG_LEVEL", "INFO"),
	}

	// Validate configuration
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func Validate(cfg Config) error {
	// Reject missing JWT_SECRET in production
	if cfg.AppEnv == "production" && cfg.JWTSecret == "" {
		return errors.New("JWT_SECRET is required in production")
	}

	// Reject weak JWT_SECRET
	if len(cfg.JWTSecret) < 32 {
		return errors.New("JWT_SECRET must be at least 32 characters")
	}

	// Reject wildcard CORS origins
	for _, origin := range cfg.CORS {
		if origin == "*" || origin == "" {
			return errors.New("wildcard CORS origins are not allowed")
		}
		// Validate origin format
		if !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
			return fmt.Errorf("invalid CORS origin: %s (must start with http:// or https://)", origin)
		}
	}

	// Reject default password in production
	if cfg.AppEnv == "production" {
		dbPassword := os.Getenv("DB_PASSWORD")
		if dbPassword == "ChangeMe12345!" {
			return errors.New("default database password detected, please change DB_PASSWORD")
		}
	}

	return nil
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func csv(s string) []string {
	var o []string
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			o = append(o, v)
		}
	}
	return o
}

func DSN() string {
	if v := strings.TrimSpace(os.Getenv("DB_DSN")); v != "" {
		return v
	}
	name := strings.TrimSpace(os.Getenv("DB_NAME"))
	if name == "" {
		return ""
	}
	c := mysql.NewConfig()
	c.User = env("DB_USER", "root")
	c.Passwd = os.Getenv("DB_PASSWORD")
	c.Net = "tcp"
	c.Addr = net.JoinHostPort(env("DB_HOST", "127.0.0.1"), env("DB_PORT", "3306"))
	c.DBName = name
	c.ParseTime = true
	c.Collation = "utf8mb4_unicode_ci"
	return c.FormatDSN()
}
