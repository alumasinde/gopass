package config

import (
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

func Load() Config {
	_ = godotenv.Load()
	p, _ := strconv.Atoi(env("HTTP_PORT", "8080"))
	a, _ := time.ParseDuration(env("JWT_ACCESS_TTL", "15m"))
	r, _ := time.ParseDuration(env("JWT_REFRESH_TTL", "168h"))
	return Config{AppName: env("APP_NAME", "gopass"), AppEnv: env("APP_ENV", "development"), AppURL: env("APP_URL", "http://localhost:8080"), Host: env("HTTP_HOST", "0.0.0.0"), Port: p, DBDSN: DSN(), JWTSecret: env("JWT_SECRET", ""), JWTIssuer: env("JWT_ISSUER", "gopass"), AccessTTL: a, RefreshTTL: r, CORS: csv(env("CORS_ALLOWED_ORIGINS", "")), LogLevel: env("LOG_LEVEL", "INFO")}
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
