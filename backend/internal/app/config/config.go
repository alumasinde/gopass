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

	port, err := strconv.Atoi(env("HTTP_PORT", "8080"))
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("HTTP_PORT must be a number between 1 and 65535")
	}
	access, err := time.ParseDuration(env("JWT_ACCESS_TTL", "15m"))
	if err != nil || access <= 0 {
		return Config{}, fmt.Errorf("JWT_ACCESS_TTL must be a positive duration such as 15m")
	}
	refresh, err := time.ParseDuration(env("JWT_REFRESH_TTL", "168h"))
	if err != nil || refresh <= 0 {
		return Config{}, fmt.Errorf("JWT_REFRESH_TTL must be a positive duration such as 168h")
	}

	cfg := Config{
		AppName:    env("APP_NAME", "gopass"),
		AppEnv:     strings.ToLower(env("APP_ENV", "development")),
		AppURL:     env("APP_URL", "http://localhost:8080"),
		Host:       env("HTTP_HOST", "0.0.0.0"),
		Port:       port,
		DBDSN:      DSN(),
		JWTSecret:  env("JWT_SECRET", ""),
		JWTIssuer:  env("JWT_ISSUER", "gopass"),
		AccessTTL:  access,
		RefreshTTL: refresh,
		CORS:       csv(env("CORS_ALLOWED_ORIGINS", "")),
		LogLevel:   env("LOG_LEVEL", "INFO"),
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Validate(cfg Config) error {
	if cfg.DBDSN == "" {
		return errors.New("DB_DSN (or DB_NAME and the DB_* parts) is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return errors.New("JWT_SECRET must be at least 32 characters")
	}
	if cfg.AccessTTL >= cfg.RefreshTTL {
		return errors.New("JWT_ACCESS_TTL must be shorter than JWT_REFRESH_TTL")
	}
	for _, origin := range cfg.CORS {
		if origin == "*" {
			return errors.New("wildcard CORS origin is not allowed (credentials are enabled)")
		}
		if !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
			return fmt.Errorf("invalid CORS origin %q (must start with http:// or https://)", origin)
		}
	}
	if cfg.AppEnv == "production" {
		if len(cfg.CORS) == 0 {
			return errors.New("CORS_ALLOWED_ORIGINS must be set in production")
		}
		for _, origin := range cfg.CORS {
			if strings.HasPrefix(origin, "http://") {
				return fmt.Errorf("insecure CORS origin %q is not allowed in production", origin)
			}
		}
		if strings.Contains(strings.ToLower(cfg.JWTSecret), "replace-with") {
			return errors.New("JWT_SECRET still has the example placeholder value")
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
	var c *mysql.Config
	if v := strings.TrimSpace(os.Getenv("DB_DSN")); v != "" {
		parsed, err := mysql.ParseDSN(v)
		if err != nil {
			// Preserve the original value so the normal database startup path
			// can report the DSN parsing error with its usual context.
			return v
		}
		c = parsed
	} else {
		name := strings.TrimSpace(os.Getenv("DB_NAME"))
		if name == "" {
			return ""
		}
		c = mysql.NewConfig()
		c.User = env("DB_USER", "root")
		c.Passwd = os.Getenv("DB_PASSWORD")
		c.Net = "tcp"
		c.Addr = net.JoinHostPort(env("DB_HOST", "127.0.0.1"), env("DB_PORT", "3306"))
		c.DBName = name
		c.Collation = "utf8mb4_unicode_ci"
	}
	// GoPass stores DATETIME values as UTC wall-clock values. MySQL DATETIME
	// has no timezone metadata, so force the driver to interpret parsed values
	// as UTC instead of the machine's local timezone (e.g. Africa/Nairobi).
	c.ParseTime = true
	c.Loc = time.UTC
	if c.Params == nil {
		c.Params = map[string]string{}
	}
	// `loc` controls Go-side parsing; `time_zone` controls MySQL's session.
	// Both must be UTC so TIMESTAMP defaults and parsed DATETIME values agree.
	c.Params["time_zone"] = "'+00:00'"
	return c.FormatDSN()
}
