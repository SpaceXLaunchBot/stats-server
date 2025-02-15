package main

import (
	"fmt"
	"os"
	"strconv"
)

type config struct {
	pgHost string
	pgPort int
	pgUser string
	pgPass string
	pgDb   string
}

func (c config) ConnectionString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s",
		c.pgUser, c.pgPass,
		c.pgHost, c.pgPort,
		c.pgDb,
	)
}

func (c config) CensoredConnectionString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s",
		c.pgUser, "******",
		c.pgHost, c.pgPort,
		c.pgDb,
	)
}

func envVar[T string | int](envVar string, defaultValue T) T {
	if v, ok := os.LookupEnv(envVar); ok {
		switch any(defaultValue).(type) {
		case int:
			if i, err := strconv.Atoi(v); err == nil {
				return any(i).(T)
			}
		case string:
			return any(v).(T)
		}
	}
	return defaultValue
}

func loadConfig() config {
	return config{
		envVar("SLB_DB_HOST", "localhost"),
		envVar("SLB_DB_PORT", 5432),
		envVar("POSTGRES_USER", "slb"),
		envVar("POSTGRES_PASSWORD", "slb"),
		envVar("POSTGRES_DB", "spacexlaunchbot"),
	}
}
