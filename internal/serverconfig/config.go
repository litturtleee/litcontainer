package serverconfig

import (
	"gopkg.in/yaml.v3"
	"litcontainer/internal/logger"

	"os"
)

type Config struct {
	Server ServerConfig `yaml:"server"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port string `yaml:"port"`
	Mode string `yaml:"mode"`
}

func Load() *Config {
	var config Config

	err := config.loadFromFile()
	if err == nil {
		return &config
	}

	config.applyEnvOverrides()
	return &config
}

// --- 内部方法 ---

func (c *Config) loadFromFile() error {
	defaultConfigPath := "/etc/litcontainer/serverconfig.yaml"
	_, err := os.Stat(defaultConfigPath)
	if err != nil {
		logger.Error("Default serverconfig file not exists, use env serverconfig")
		return err
	}
	data, err := os.ReadFile(defaultConfigPath)
	if err != nil {
		logger.Error("Read serverconfig file error: %v", err)
		return err
	}

	return yaml.Unmarshal(data, &c)
}

func (c *Config) applyEnvOverrides() {
	c.Server.Host = getEnv("LITCONTAINER_HOST", "0.0.0.0")
	c.Server.Port = getEnv("LITCONTAINER_PORT", "8080")
	c.Server.Mode = getEnv("LITCONTAINER_MODE", "debug")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
