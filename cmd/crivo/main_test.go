package main

import "testing"

func TestParseArgs_DisableChecksSupportsRepeatAndCommaList(t *testing.T) {
	opts := parseArgs([]string{
		"run",
		"--disable", "complexity,coverage",
		"--disable", "semgrep",
	})

	expected := []string{"complexity", "coverage", "semgrep"}
	for _, checkID := range expected {
		if !opts.disabledChecks[checkID] {
			t.Fatalf("expected disabledChecks[%q] to be true", checkID)
		}
	}
}

func TestParseCheckList_NormalizesValues(t *testing.T) {
	values := parseCheckList(" Complexity, coverage , ,SEMGRP ")

	if len(values) != 3 {
		t.Fatalf("expected 3 values, got %d", len(values))
	}

	if values[0] != "complexity" || values[1] != "coverage" || values[2] != "semgrp" {
		t.Fatalf("unexpected normalized values: %#v", values)
	}
}
