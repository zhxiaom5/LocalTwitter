package server

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const DatabaseFileName = "localtwitter.db"

type AppConfig struct {
	DatabaseDir string `json:"database_dir"`
}

type DatabaseConfig struct {
	Directory string `json:"directory"`
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	Writable  bool   `json:"writable"`
}

func LoadConfig(path string, defaultDatabaseDir string) (AppConfig, error) {
	cfg := AppConfig{DatabaseDir: filepath.Clean(defaultDatabaseDir)}
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return AppConfig{}, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return AppConfig{}, err
	}
	if cfg.DatabaseDir == "" {
		cfg.DatabaseDir = defaultDatabaseDir
	}
	cfg.DatabaseDir = filepath.Clean(cfg.DatabaseDir)
	return cfg, nil
}

func SaveConfig(path string, cfg AppConfig) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func DatabasePath(directory string) string {
	return filepath.Join(filepath.Clean(directory), DatabaseFileName)
}

func InspectDatabaseDirectory(directory string) (DatabaseConfig, error) {
	clean := filepath.Clean(directory)
	if err := ValidateDirectory(clean); err != nil {
		return DatabaseConfig{}, err
	}
	dbPath := DatabasePath(clean)
	_, statErr := os.Stat(dbPath)
	exists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return DatabaseConfig{}, statErr
	}
	writable := directoryWritable(clean)
	return DatabaseConfig{Directory: clean, Path: dbPath, Exists: exists, Writable: writable}, nil
}

func directoryWritable(directory string) bool {
	file, err := os.CreateTemp(directory, ".localtwitter-write-test-*")
	if err != nil {
		return false
	}
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
	return true
}
