package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
)

type Profile struct {
	CRF    int    `yaml:"crf"`
	Preset string `yaml:"preset"`
}

type Config struct {
	WatchDirs []string `yaml:"watchDirs"`

	DestDir        string             `yaml:"destDir"`
	CRF            int                `yaml:"crf"`
	Preset         string             `yaml:"preset"`
	FPS            int                `yaml:"fps"`
	Mute           bool               `yaml:"mute"`
	Keywords       []string           `yaml:"keywords"`
	IgnoreKeywords []string           `yaml:"ignoreKeywords"`
	NoPad          bool               `yaml:"noPad"`
	StampPerFile   bool               `yaml:"stampPerFile"`
	NoTrash        bool               `yaml:"noTrash"`
	SourcePolicy   string             `yaml:"sourcePolicy"`
	BatchStamp     bool               `yaml:"batchStamp"`
	FFmpegBin      string             `yaml:"ffmpegBin"`
	Concurrent     int                `yaml:"concurrent"`
	Notify         bool               `yaml:"notify"`
	LogFile        string             `yaml:"logFile"`
	DryRun         bool               `yaml:"dryRun"`
	StableTimeout  time.Duration      `yaml:"stableTimeout"`
	StableInterval time.Duration      `yaml:"stableInterval"`
	StableSamples  int                `yaml:"stableSamples"`
	Profiles       map[string]Profile `yaml:"profiles"`
	ParallelSplit  bool               `yaml:"parallelSplit"`
	GPU            bool               `yaml:"gpu"`
}

func NewDefault() *Config {
	cwd, _ := os.Getwd()
	defaultDest := filepath.Join(cwd, "out")
	defaultConcurrent := runtime.NumCPU() - 1
	if defaultConcurrent < 1 {
		defaultConcurrent = 1
	}

	return &Config{
		DestDir:        defaultDest,
		CRF:            22,
		Preset:         "faster",
		FPS:            30,
		BatchStamp:     true,
		Concurrent:     defaultConcurrent,
		Notify:         true,
		SourcePolicy:   "trash",
		StableTimeout:  120 * time.Second,
		StableInterval: time.Second,
		StableSamples:  3,
	}
}

func Load() (*Config, error) {
	cfg := NewDefault()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil // ホームディレクトリが取れなくてもデフォルトで進む
	}

	configPath := filepath.Join(home, ".config", "rec-watch", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return cfg, nil
	}

	f, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer f.Close()

	if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}
	if cfg.NoTrash {
		cfg.SourcePolicy = "keep"
	}
	if err := ValidateSourcePolicy(cfg.SourcePolicy); err != nil {
		return nil, err
	}

	return cfg, nil
}

func ValidateSourcePolicy(policy string) error {
	switch policy {
	case "keep", "trash", "ask":
		return nil
	default:
		return fmt.Errorf("invalid sourcePolicy %q: use keep, trash, or ask", policy)
	}
}
