package config

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func Load() error {
	v := viper.New()
	v.SetConfigFile("./config.conf")
	v.SetConfigType("properties")

	if err := v.ReadInConfig(); err != nil {
		// If config file is not found, just log a warning and continue.
		// Configuration can come from environment variables instead.
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Warn("Config file not found, using environment variables only")
			return nil
		}
		if os.IsNotExist(err) {
			log.Warn("Config file ./config.conf not found, using environment variables only")
			return nil
		}
		return err
	}

	for _, key := range v.AllKeys() {
		os.Setenv(key, v.GetString(key))
	}

	log.Info("Configuration loaded from ./config.conf")
	return nil
}
