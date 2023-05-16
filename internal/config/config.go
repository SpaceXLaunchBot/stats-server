package config

import (
	"github.com/spf13/viper"
)

// Config contains the application configuration, to be unmarshalled into by Viper.
type Config struct {
	DbHost string `mapstructure:"SLB_DB_HOST"`
	DbPort int    `mapstructure:"SLB_DB_PORT"`
	DbUser string `mapstructure:"POSTGRES_USER"`
	DbPass string `mapstructure:"POSTGRES_PASSWORD"`
	DbName string `mapstructure:"POSTGRES_DB"`
}

// Get looks in ./config.env and environment variables for needed values.
func Get() (Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")

	viper.SetDefault("SLB_DB_HOST", "localhost")
	viper.SetDefault("SLB_DB_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "slb")
	viper.SetDefault("POSTGRES_DB", "spacexlaunchbot")

	_ = viper.BindEnv("SLB_DB_HOST")
	_ = viper.BindEnv("SLB_DB_PORT")
	_ = viper.BindEnv("POSTGRES_USER")
	_ = viper.BindEnv("POSTGRES_PASSWORD")
	_ = viper.BindEnv("POSTGRES_DB")

	// Will error if no config file, but we also load from env vars so no need to panic.
	_ = viper.ReadInConfig()

	var config Config
	err := viper.Unmarshal(&config)
	if err != nil {
		return Config{}, err
	}

	return config, nil
}
