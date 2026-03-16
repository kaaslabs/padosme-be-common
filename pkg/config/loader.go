package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadEnvFile reads a .env file and sets each KEY=VALUE pair as an environment
// variable (using os.Setenv). Existing env vars are NOT overwritten (dotenv
// semantics: process environment takes precedence).
//
// Lines starting with '#' and empty lines are ignored.
// Values may optionally be quoted with single or double quotes.
func LoadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		// Treat missing .env as non-fatal — callers can decide.
		return fmt.Errorf("config: open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := parseLine(line)
		if !ok {
			return fmt.Errorf("config: %s line %d: malformed KEY=VALUE pair: %q", path, lineNum, line)
		}

		// Do not overwrite existing env vars.
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("config: setenv %s: %w", key, err)
			}
		}
	}

	return scanner.Err()
}

// GetEnv returns the value of the named environment variable, or defaultValue
// if the variable is not set or is empty.
func GetEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// MustGetEnv returns the value of the named environment variable. It panics if
// the variable is not set. Use this for required configuration.
func MustGetEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("config: required environment variable %q is not set", key))
	}
	return v
}

// parseLine parses a single KEY=VALUE line. Returns false if the line does not
// contain an '=' character.
func parseLine(line string) (key, value string, ok bool) {
	idx := strings.IndexByte(line, '=')
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])

	// Strip optional surrounding quotes.
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}

	return key, value, key != ""
}
