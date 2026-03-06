package config

import (
	"fmt"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// application configuration
type Config struct {
	Log       LogConfig       `mapstructure:"log"`
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Reference ReferenceConfig `mapstructure:"reference"`
	Discovery DiscoveryConfig `mapstructure:"discovery"`
	Checker   CheckerConfig   `mapstructure:"checker"`
	Scoring   ScoringConfig   `mapstructure:"scoring"`
	Epoch     EpochConfig     `mapstructure:"epoch"`
	Flagging  FlaggingConfig  `mapstructure:"flagging"`
	Cache     CacheConfig     `mapstructure:"cache"`
}

// LogConfig
type LogConfig struct {
	Level string `mapstructure:"level"` // debug, info, warn, error
}

// HTTP server configuration
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// PostgreSQL connection configuration
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
	MaxConns int    `mapstructure:"maxConns"`
}

func (d *DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode,
	)
}

// APIEndpoint holds configuration for a single API endpoint with toggle
type APIEndpoint struct {
	URL     string `mapstructure:"url"`
	Enabled bool   `mapstructure:"enabled"`
}

// ReferenceConfig holds reference tick service configuration
type ReferenceConfig struct {
	PollInterval int           `mapstructure:"pollInterval"` // secs
	Timeout      int           `mapstructure:"timeout"`      // secs
	APIs         []APIEndpoint `mapstructure:"apis"`
}

// GetEnabledAPIs returns only the enabled API URLs
func (r *ReferenceConfig) GetEnabledAPIs() []string {
	var enabled []string
	for _, api := range r.APIs {
		if api.Enabled {
			enabled = append(enabled, api.URL)
		}
	}
	return enabled
}

// DiscoveryConfig holds node discovery configuration
type DiscoveryConfig struct {
	PollInterval int    `mapstructure:"pollInterval"` // seconds
	Endpoint     string `mapstructure:"endpoint"`
	Timeout      int    `mapstructure:"timeout"` // seconds
}

// CheckerConfig holds check cycle configuration
type CheckerConfig struct {
	WorkerCount  int `mapstructure:"workerCount"`
	BaseInterval int `mapstructure:"baseInterval"` // secs
	JitterMax    int `mapstructure:"jitterMax"`    // secs
	CheckTimeout int `mapstructure:"checkTimeout"` // secs
	LitePort     int `mapstructure:"litePort"`
	BobPort      int `mapstructure:"bobPort"`

	CheckerID  string `mapstructure:"checkerID"`  // Unique ID for this checker instance
	Region     string `mapstructure:"region"`     // Geographic region
	ClaimBatch int    `mapstructure:"claimBatch"` // Number of nodes to claim per cycle
	ClaimTTL   int    `mapstructure:"claimTTL"`   // Claim expiration in seconds
}

// MetricConfig holds configuration for a scoring metric with toggle
type MetricConfig struct {
	Enabled bool    `mapstructure:"enabled"`
	Weight  float64 `mapstructure:"weight"`
}

// ThresholdConfig holds configuration for an eligibility threshold with toggle
type ThresholdConfig struct {
	Enabled bool    `mapstructure:"enabled"`
	Value   float64 `mapstructure:"value"`
}

// ScoringConfig holds scoring calculation configuration
type ScoringConfig struct {
	Uptime          MetricConfig `mapstructure:"uptime"`
	Sync            MetricConfig `mapstructure:"sync"`
	TickBuffer      int          `mapstructure:"tickBuffer"`
	DecayRate       float64      `mapstructure:"decayRate"`
	TimestampMaxAge int          `mapstructure:"timestampMaxAge"` // secs
}

// GetNormalizedWeights returns weights normalized to sum to 1.0
func (s *ScoringConfig) GetNormalizedWeights() (uptimeWeight, syncWeight float64) {
	var totalWeight float64
	if s.Uptime.Enabled {
		totalWeight += s.Uptime.Weight
	}
	if s.Sync.Enabled {
		totalWeight += s.Sync.Weight
	}

	if totalWeight == 0 {
		return 0.5, 0.5
	}

	if s.Uptime.Enabled {
		uptimeWeight = s.Uptime.Weight / totalWeight
	}
	if s.Sync.Enabled {
		syncWeight = s.Sync.Weight / totalWeight
	}

	return uptimeWeight, syncWeight
}

// EpochConfig holds epoch transition configuration
type EpochConfig struct {
	TotalPoolAmount    int64           `mapstructure:"totalPoolAmount"`
	LitePoolPercent    float64         `mapstructure:"litePoolPercent"`
	BobPoolPercent     float64         `mapstructure:"bobPoolPercent"`
	UptimeThreshold    ThresholdConfig `mapstructure:"uptimeThreshold"`
	SyncThreshold      ThresholdConfig `mapstructure:"syncThreshold"`
	MinChecksThreshold ThresholdConfig `mapstructure:"minChecksThreshold"`
	GracePeriodMinutes int             `mapstructure:"gracePeriodMinutes"`
}

// FlaggingConfig holds auto-flagging configuration
type FlaggingConfig struct {
	Enabled      bool `mapstructure:"enabled"`
	PollInterval int  `mapstructure:"pollInterval"` // seconds
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	TTL int `mapstructure:"ttl"` // secs
}

// Load loads configuration from a JSON file
func Load(configPath string) (*Config, error) {
	_ = godotenv.Load()

	v := viper.New()

	setDefaults(v)

	v.SetConfigFile(configPath)
	v.SetConfigType("json")

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Environment variable overrides
	v.SetEnvPrefix("GUARDIANS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Log defaults
	v.SetDefault("log.level", "info")

	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)

	// Database defaults
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5555)
	v.SetDefault("database.user", "default")
	v.SetDefault("database.password", "")
	v.SetDefault("database.dbname", "default")
	v.SetDefault("database.sslmode", "enabled")
	v.SetDefault("database.maxConns", 25)

	// Reference defaults
	v.SetDefault("reference.pollInterval", 1)
	v.SetDefault("reference.timeout", 5)
	v.SetDefault("reference.apis", []map[string]interface{}{
		{"url": "default", "enabled": true},
	})

	// Discovery
	v.SetDefault("discovery.pollInterval", 1)
	v.SetDefault("discovery.endpoint", "default")
	v.SetDefault("discovery.timeout", 1)

	// Checker
	v.SetDefault("checker.workerCount", 1)
	v.SetDefault("checker.baseInterval", 1)
	v.SetDefault("checker.jitterMax", 1)
	v.SetDefault("checker.checkTimeout", 1)
	v.SetDefault("checker.litePort", 41841)
	v.SetDefault("checker.bobPort", 40420)
	v.SetDefault("checker.checkerID", "default")
	v.SetDefault("checker.region", "default")
	v.SetDefault("checker.claimBatch", 1)
	v.SetDefault("checker.claimTTL", 1)

	// Scoring
	v.SetDefault("scoring.uptime.enabled", true)
	v.SetDefault("scoring.uptime.weight", 1)
	v.SetDefault("scoring.sync.enabled", true)
	v.SetDefault("scoring.sync.weight", 1)
	v.SetDefault("scoring.tickBuffer", 1)
	v.SetDefault("scoring.decayRate", 1)
	v.SetDefault("scoring.timestampMaxAge", 1)

	// Epoch config
	v.SetDefault("epoch.totalPoolAmount", 1)
	v.SetDefault("epoch.litePoolPercent", 80.0)
	v.SetDefault("epoch.bobPoolPercent", 20.0)
	v.SetDefault("epoch.uptimeThreshold.enabled", true)
	v.SetDefault("epoch.uptimeThreshold.value", 70.0)
	v.SetDefault("epoch.syncThreshold.enabled", true)
	v.SetDefault("epoch.syncThreshold.value", 40.0)
	v.SetDefault("epoch.minChecksThreshold.enabled", true)
	v.SetDefault("epoch.minChecksThreshold.value", 1)
	v.SetDefault("epoch.gracePeriodMinutes", 1)

	// Flagging
	v.SetDefault("flagging.enabled", true)
	v.SetDefault("flagging.pollInterval", 1)

	// Cache
	v.SetDefault("cache.ttl", 300) // 5 minutes
}
