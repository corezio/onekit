package onek

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type TargetConfig struct {
	Out string `toml:"out"`
}

type OpenAPITargetConfig struct {
	Out         string `toml:"out"`
	Title       string `toml:"title"`
	Version     string `toml:"version"`
	Description string `toml:"description"`
}

type GenerateConfig struct {
	GoServer     *TargetConfig        `toml:"go-server"`
	GoClient     *TargetConfig        `toml:"go-client"`
	TSClient     *TargetConfig        `toml:"ts-client"`
	TSServer     *TargetConfig        `toml:"ts-server"`
	PythonClient *TargetConfig        `toml:"python-client"`
	OpenAPI      *OpenAPITargetConfig `toml:"openapi"`
}

type Config struct {
	Module   string         `toml:"module"`
	Generate GenerateConfig `toml:"generate"`

	dir string
}

const configFileName = "onekit.toml"

func LoadConfig(dir string) (*Config, error) {
	path := filepath.Join(dir, configFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	err = toml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.dir = dir
	return &cfg, nil
}

func (c *Config) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(c.dir, path)
}
