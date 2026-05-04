package redscli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type configFile struct {
	DefaultProfile string                   `json:"default_profile,omitempty"`
	Profiles       map[string]profileConfig `json:"profiles,omitempty"`
}

type profileConfig struct {
	Token   string `json:"token,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

func defaultConfigPath() string {
	if value := strings.TrimSpace(os.Getenv("REDS_CONFIG")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); value != "" {
		return filepath.Join(value, "redscli", "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".redscli.json"
	}
	return filepath.Join(home, ".config", "redscli", "config.json")
}

func loadConfig(path string) (configFile, error) {
	var cfg configFile
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func saveConfig(path string, cfg configFile) error {
	if cfg.DefaultProfile == "" {
		cfg.DefaultProfile = "default"
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]profileConfig{}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func selectedProfile(cfg configFile, requested string) string {
	if requested != "" {
		return requested
	}
	if env := strings.TrimSpace(os.Getenv("REDS_PROFILE")); env != "" {
		return env
	}
	if cfg.DefaultProfile != "" {
		return cfg.DefaultProfile
	}
	return "default"
}

func effectiveSettings(opts globalOptions) (baseURL, token string, timeout time.Duration, err error) {
	path := opts.configPath
	if path == "" {
		path = defaultConfigPath()
	}
	cfg, err := loadConfig(path)
	if err != nil {
		return "", "", 0, err
	}
	profileName := selectedProfile(cfg, opts.profile)
	if profile, ok := cfg.Profiles[profileName]; ok {
		baseURL = profile.BaseURL
		token = profile.Token
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	timeout = 60 * time.Second

	if env := strings.TrimSpace(os.Getenv("REDS_API_BASE_URL")); env != "" {
		baseURL = env
	} else if env := strings.TrimSpace(os.Getenv("REDS_BASE_URL")); env != "" {
		baseURL = env
	}
	if env := strings.TrimSpace(os.Getenv("REDS_API_TOKEN")); env != "" {
		token = env
	}
	if env := strings.TrimSpace(os.Getenv("REDS_TIMEOUT")); env != "" {
		parsed, parseErr := time.ParseDuration(env)
		if parseErr != nil {
			return "", "", 0, fmt.Errorf("invalid REDS_TIMEOUT: %w", parseErr)
		}
		timeout = parsed
	}

	if opts.baseURLSet {
		baseURL = opts.baseURL
	}
	if opts.tokenSet {
		token = opts.token
	}
	if opts.timeoutSet {
		timeout = opts.timeout
	}
	return baseURL, token, timeout, nil
}

func ensureProfile(cfg *configFile, name string) profileConfig {
	if cfg.DefaultProfile == "" {
		cfg.DefaultProfile = "default"
	}
	if name == "" {
		name = cfg.DefaultProfile
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]profileConfig{}
	}
	return cfg.Profiles[name]
}
