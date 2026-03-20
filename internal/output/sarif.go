package output

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/quality-gate/internal/domain"
)

// SARIF 2.1.0 structures
type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string           `json:"id"`
	ShortDescription sarifMessage     `json:"shortDescription"`
	DefaultConfig    sarifRuleConfig  `json:"defaultConfiguration"`
	Properties       sarifProperties  `json:"properties,omitempty"`
}

type sarifRuleConfig struct {
	Level string `json:"level"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID    string           `json:"ruleId"`
	Level     string           `json:"level"`
	Message   sarifMessage     `json:"message"`
	Locations []sarifLocation  `json:"locations"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn"`
}

type sarifProperties struct {
	Tags []string `json:"tags,omitempty"`
}

// ToSARIF converts analysis results to SARIF 2.1.0 format
func ToSARIF(result *domain.AnalysisResult) ([]byte, error) {
	allIssues := result.AllIssues()

	// Collect unique rules
	ruleMap := map[string]sarifRule{}
	for _, issue := range allIssues {
		if _, exists := ruleMap[issue.RuleID]; exists {
			continue
		}
		ruleMap[issue.RuleID] = sarifRule{
			ID:               issue.RuleID,
			ShortDescription: sarifMessage{Text: issue.RuleID},
			DefaultConfig:    sarifRuleConfig{Level: severityToSARIFLevel(issue.Severity)},
			Properties:       sarifProperties{Tags: issueTypeTags(issue.Type)},
		}
	}

	rules := make([]sarifRule, 0, len(ruleMap))
	for _, r := range ruleMap {
		rules = append(rules, r)
	}

	// Convert issues to SARIF results
	results := make([]sarifResult, 0, len(allIssues))
	for _, issue := range allIssues {
		uri := issue.File
		if !strings.HasPrefix(uri, "file://") {
			uri = "file://" + strings.ReplaceAll(uri, "\\", "/")
		}

		results = append(results, sarifResult{
			RuleID:  issue.RuleID,
			Level:   severityToSARIFLevel(issue.Severity),
			Message: sarifMessage{Text: issue.Message},
			Locations: []sarifLocation{
				{
					PhysicalLocation: sarifPhysicalLocation{
						ArtifactLocation: sarifArtifactLocation{URI: uri},
						Region: sarifRegion{
							StartLine:   issue.Line,
							StartColumn: issue.Column,
						},
					},
				},
			},
		})
	}

	log := sarifLog{
		Schema:  "https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{
			{
				Tool: sarifTool{
					Driver: sarifDriver{
						Name:           "quality-gate",
						Version:        "0.1.0",
						InformationURI: "https://github.com/anthropics/quality-gate",
						Rules:          rules,
					},
				},
				Results: results,
			},
		},
	}

	return json.MarshalIndent(log, "", "  ")
}

// PrintSARIF outputs SARIF to stdout
func PrintSARIF(result *domain.AnalysisResult) error {
	data, err := ToSARIF(result)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func severityToSARIFLevel(s domain.Severity) string {
	switch s {
	case domain.SeverityBlocker, domain.SeverityCritical:
		return "error"
	case domain.SeverityMajor:
		return "warning"
	case domain.SeverityMinor, domain.SeverityInfo:
		return "note"
	default:
		return "note"
	}
}

func issueTypeTags(t domain.IssueType) []string {
	switch t {
	case domain.IssueTypeBug:
		return []string{"reliability"}
	case domain.IssueTypeVulnerability:
		return []string{"security"}
	case domain.IssueTypeSecurityHotspot:
		return []string{"security"}
	case domain.IssueTypeCodeSmell:
		return []string{"maintainability"}
	default:
		return nil
	}
}
