package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotEnv loads OFFERPILOT_CONFIG_PATH when configured, otherwise the first
// .env file found in the working directory or one of its parents. Non-empty
// process environment variables always win.
func LoadDotEnv() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("OFFERPILOT_CONFIG_PATH")); configured != "" {
		path, err := filepath.Abs(configured)
		if err != nil {
			return "", fmt.Errorf("config: resolve %s: %w", configured, err)
		}
		if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
			return "", nil
		} else if statErr != nil {
			return "", fmt.Errorf("config: stat %s: %w", path, statErr)
		}
		if err := loadEnvFile(path); err != nil {
			return "", err
		}
		return path, nil
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("config: get working directory: %w", err)
	}

	for directory := workingDirectory; ; directory = filepath.Dir(directory) {
		path := filepath.Join(directory, ".env")
		if _, statErr := os.Stat(path); statErr == nil {
			if err := loadEnvFile(path); err != nil {
				return "", err
			}
			return path, nil
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("config: stat %s: %w", path, statErr)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", nil
		}
	}
}

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("config: open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		key, value, ok := parseEnvLine(scanner.Text())
		if !ok {
			continue
		}
		if existing, exists := os.LookupEnv(key); exists && strings.TrimSpace(existing) != "" {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("config: set %s: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("config: read %s: %w", path, err)
	}
	return nil
}

func parseEnvLine(raw string) (string, string, bool) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	separator := strings.IndexByte(line, '=')
	if separator <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:separator])
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	value := strings.TrimSpace(line[separator+1:])
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return key, value, true
}
