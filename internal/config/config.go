package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// GatePolicy controls which checks can block the gate.
//   - "release": blocks on prod type errors, lint errors, secrets only
//   - "strict": blocks on all check failures
//   - "informational": never blocks, only reports
type GatePolicy string

const (
	GatePolicyRelease       GatePolicy = "release"
	GatePolicyStrict        GatePolicy = "strict"
	GatePolicyInformational GatePolicy = "informational"
)

// Config represents the .qualitygate.yaml configuration
type Config struct {
	Profile     string            `yaml:"profile" json:"profile"`
	Policy      GatePolicy        `yaml:"gate-policy" json:"gatePolicy"`
	Languages   []string          `yaml:"languages" json:"languages"`
	Src         []string          `yaml:"src" json:"src"`
	Exclude     []string          `yaml:"exclude" json:"exclude"`
	Checks      ChecksConfig      `yaml:"checks" json:"checks"`
	QualityGate QualityGateConfig `yaml:"quality-gate" json:"qualityGate"`
	Coverage    CoverageConfig    `yaml:"coverage" json:"coverage"`
	Duplication DuplicationConfig `yaml:"duplication" json:"duplication"`
	Complexity  ComplexityConfig  `yaml:"complexity" json:"complexity"`
}

type ChecksConfig struct {
	Typescript bool `yaml:"typescript" json:"typescript"`
	ESLint     bool `yaml:"eslint" json:"eslint"`
	Semgrep    bool `yaml:"semgrep" json:"semgrep"`
	Coverage   bool `yaml:"coverage" json:"coverage"`
	Duplication bool `yaml:"duplication" json:"duplication"`
	Secrets    bool `yaml:"secrets" json:"secrets"`
	DeadCode   bool `yaml:"dead-code" json:"deadCode"`
}

type QualityGateConfig struct {
	NewCode ThresholdConfig `yaml:"new-code" json:"newCode"`
	Overall ThresholdConfig `yaml:"overall" json:"overall"`
}

type ThresholdConfig struct {
	Coverage        float64 `yaml:"coverage" json:"coverage"`
	Bugs            int     `yaml:"bugs" json:"bugs"`
	Vulnerabilities int     `yaml:"vulnerabilities" json:"vulnerabilities"`
	Duplications    float64 `yaml:"duplications" json:"duplications"`
	CodeSmells      int     `yaml:"code-smells" json:"codeSmells"`
}

type CoverageConfig struct {
	Lines      float64 `yaml:"lines" json:"lines"`
	Branches   float64 `yaml:"branches" json:"branches"`
	Functions  float64 `yaml:"functions" json:"functions"`
	Statements float64 `yaml:"statements" json:"statements"`
}

type DuplicationConfig struct {
	Threshold float64 `yaml:"threshold" json:"threshold"`
	MinLines  int     `yaml:"min-lines" json:"minLines"`
	MinTokens int     `yaml:"min-tokens" json:"minTokens"`
}

type ComplexityConfig struct {
	Threshold int `yaml:"threshold" json:"threshold"` // max cognitive complexity per function (default: 15)
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Profile:   "balanced",
		Policy:    GatePolicyRelease,
		Languages: []string{"typescript"},
		Src:       []string{"src/"},
		Exclude: []string{
			"node_modules/",
			"dist/",
			".next/",
			"coverage/",
			"*.min.js",
		},
		Checks: ChecksConfig{
			Typescript:  true,
			ESLint:      true,
			Semgrep:     false,
			Coverage:    true,
			Duplication: true,
			Secrets:     false,
			DeadCode:    false,
		},
		QualityGate: QualityGateConfig{
			NewCode: ThresholdConfig{
				Coverage:        80,
				Bugs:            0,
				Vulnerabilities: 0,
				Duplications:    3,
			},
			Overall: ThresholdConfig{
				Coverage:        60,
				Bugs:            0,
				Vulnerabilities: 0,
				Duplications:    5,
			},
		},
		Coverage: CoverageConfig{
			Lines:      60,
			Branches:   50,
			Functions:  60,
			Statements: 60,
		},
		Duplication: DuplicationConfig{
			Threshold: 5,
			MinLines:  5,
			MinTokens: 50,
		},
		Complexity: ComplexityConfig{
			Threshold: 15,
		},
	}
}

// Load reads config from .qualitygate.yaml in the project dir, merging with defaults
func Load(projectDir string) (*Config, string) {
	cfg := DefaultConfig()

	candidates := []string{
		".qualitygate.yaml",
		".qualitygate.yml",
		".qualitygate.json",
	}

	for _, name := range candidates {
		configPath := filepath.Join(projectDir, name)
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}

		// First pass: read just the profile
		var profileOnly struct {
			Profile string `yaml:"profile"`
		}
		_ = yaml.Unmarshal(data, &profileOnly)

		// Apply profile defaults first
		if profileOnly.Profile != "" {
			cfg.Profile = profileOnly.Profile
			ApplyProfile(cfg)
		}

		// Then overlay user's explicit values on top
		if err := yaml.Unmarshal(data, cfg); err != nil {
			continue
		}

		return cfg, configPath
	}

	return cfg, "defaults"
}

// GenerateDefault returns the default YAML config as bytes
func GenerateDefault() ([]byte, error) {
	return yaml.Marshal(DefaultConfig())
}
