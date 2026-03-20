package config

// Profiles define preset configurations
var Profiles = map[string]func(cfg *Config){
	"strict":   applyStrict,
	"balanced": applyBalanced,
	"lenient":  applyLenient,
}

// ApplyProfile modifies config based on the selected profile
func ApplyProfile(cfg *Config) {
	if fn, ok := Profiles[cfg.Profile]; ok {
		fn(cfg)
	}
}

func applyStrict(cfg *Config) {
	cfg.QualityGate.NewCode.Coverage = 90
	cfg.QualityGate.NewCode.Bugs = 0
	cfg.QualityGate.NewCode.Vulnerabilities = 0
	cfg.QualityGate.NewCode.Duplications = 1

	cfg.QualityGate.Overall.Coverage = 80
	cfg.QualityGate.Overall.Bugs = 0
	cfg.QualityGate.Overall.Vulnerabilities = 0
	cfg.QualityGate.Overall.Duplications = 3

	cfg.Coverage.Lines = 80
	cfg.Coverage.Branches = 70
	cfg.Coverage.Functions = 80
	cfg.Coverage.Statements = 80

	cfg.Duplication.Threshold = 3
	cfg.Duplication.MinLines = 3
	cfg.Duplication.MinTokens = 30

	// Enable all checks in strict mode
	cfg.Checks.Typescript = true
	cfg.Checks.ESLint = true
	cfg.Checks.Coverage = true
	cfg.Checks.Duplication = true
	cfg.Checks.Semgrep = true
	cfg.Checks.Secrets = true
	cfg.Checks.DeadCode = true
}

func applyBalanced(cfg *Config) {
	// balanced is the default — already set in DefaultConfig()
}

func applyLenient(cfg *Config) {
	cfg.QualityGate.NewCode.Coverage = 50
	cfg.QualityGate.NewCode.Bugs = 0
	cfg.QualityGate.NewCode.Vulnerabilities = 0
	cfg.QualityGate.NewCode.Duplications = 10

	cfg.QualityGate.Overall.Coverage = 30
	cfg.QualityGate.Overall.Bugs = 5
	cfg.QualityGate.Overall.Vulnerabilities = 0
	cfg.QualityGate.Overall.Duplications = 10

	cfg.Coverage.Lines = 30
	cfg.Coverage.Branches = 20
	cfg.Coverage.Functions = 30
	cfg.Coverage.Statements = 30

	cfg.Duplication.Threshold = 10
	cfg.Duplication.MinLines = 10
	cfg.Duplication.MinTokens = 70
}
