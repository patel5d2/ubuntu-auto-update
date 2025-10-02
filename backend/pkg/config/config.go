package config

import (
	"os"

	"github.com/spf13/viper"
)

func Load() error {
	v := viper.New()
	v.SetConfigFile("./config.conf")
	v.SetConfigType("properties")

	if err := v.ReadInConfig(); err != nil {
		return err
	}

	for _, key := range v.AllKeys() {
		os.Setenv(key, v.GetString(key))
	}

	return nil
}