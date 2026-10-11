package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port                string
	EmailPattern        string
	EmailDomain         string
	SigninIntervalDays  int
	MailgunAPIKey       string
	MailgunSigningKey   string
	MegatoolsPath       string
	DataDir             string
	CsvPath             string
	JsonPath            string
	DefaultPassword     string
	DefaultConcurrency int
}

func Load() *Config {
	port := getEnv("PORT", "3000")
	pattern := getEnv("EMAIL_PATTERN", "mega%d")
	domain := getEnv("EMAIL_DOMAIN", "example.com")
	intervalDays := getEnvInt("SIGNIN_INTERVAL_DAYS", 30)
	mgKey := getEnv("MAILGUN_API_KEY", "")
	mgSignKey := getEnv("MAILGUN_WEBHOOK_SIGNING_KEY", mgKey)
	megatools := getEnv("MEGATOOLS_PATH", "megatools")
	dataDir := getEnv("DATA_DIR", ".")
	csvPath := getEnv("CSV_PATH", dataDir+"/accounts.csv")
	jsonPath := getEnv("JSON_PATH", dataDir+"/accounts.json")
	defaultPassword := getEnv("MEGA_DEFAULT_PASSWORD", "")
	concurrency := getEnvInt("CONCURRENCY", 4)

	return &Config{
		Port:                port,
		EmailPattern:        pattern,
		EmailDomain:         domain,
		SigninIntervalDays:  intervalDays,
		MailgunAPIKey:       mgKey,
		MailgunSigningKey:   mgSignKey,
		MegatoolsPath:       megatools,
		DataDir:             dataDir,
		CsvPath:             csvPath,
		JsonPath:            jsonPath,
		DefaultPassword:     defaultPassword,
		DefaultConcurrency: concurrency,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultVal
}
