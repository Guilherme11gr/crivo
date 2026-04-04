package domain

import (
	"fmt"
	"strings"
)

// RemediationHint returns an actionable fix suggestion for common issue types.
// Providers call this after creating an Issue to populate the Remediation field.

// --- TypeScript (tsc) ---

// TSRemediation maps common TypeScript error codes to fix hints.
var TSRemediation = map[string]string{
	"TS2322": "Fix the type mismatch: check the expected type and ensure the assigned value matches",
	"TS2307": "Fix the module resolution: install the missing package, check the import path, or add type declarations",
	"TS7006": "Add an explicit type annotation to the parameter, or use `--noImplicitParameters: false`",
	"TS6133": "Remove the unused variable/import, or prefix it with `_` if intentionally unused",
	"TS6196": "Remove the unused type declaration, or use it somewhere in this file",
	"TS2366": "Add a return statement for all code paths, or throw an error for unhandled cases",
	"TS2571": "Check the value being used — it might be `null` or `undefined`. Add a null check or type guard",
	"TS18046": "Add a type guard or null check before accessing this property",
	"TS2694": "The namespace is read-only — use a mutable reference or copy the value",
	"TS2741": "Add the missing property to the object literal to match the expected type",
	"TS2554": "Fix the function call arguments: check the expected parameters and their types",
	"TS1109": "Remove the extra `)` or add the missing opening `(`",
	"TS1005": "Fix the syntax error: check for missing semicolons, brackets, or parentheses",
	"TS2304": "Fix the undefined name: check for typos, missing imports, or missing type declarations",
	"TS2345": "Fix the argument type: the value passed does not match the expected parameter type",
	"TS2532": "Add a null/undefined check before accessing this property (use optional chaining `?.` if appropriate)",
	"TS1488": "The declaration conflicts with an existing declaration — rename it or remove the duplicate",
	"TS2667": "The type predicate does not narrow the type sufficiently — adjust the condition",
	"TS2416": "The property type in the class does not match the interface — align the types",
	"TS2612": "The property will overwrite the parameter property — rename one of them",
}

// TypescriptRemediation returns a fix hint for a TS error code.
func TypescriptRemediation(ruleID string) string {
	// Normalize: uppercase and ensure "TS" prefix
	code := strings.ToUpper(ruleID)
	if !strings.HasPrefix(code, "TS") {
		code = "TS" + code
	}
	if hint, ok := TSRemediation[code]; ok {
		return hint
	}
	// Generic fallback for unknown TS errors
	return fmt.Sprintf("Fix the TypeScript error %s: check types, imports, and syntax at the reported location", ruleID)
}

// --- Secrets (gitleaks) ---

// SecretRemediation returns a fix hint based on the gitleaks rule ID.
func SecretRemediation(ruleID string) string {
	// Extract the secret type from ruleID like "secret/generic-api-key"
	secretType := strings.TrimPrefix(ruleID, "secret/")

	hints := map[string]string{
		"generic-api-key":         "Move this API key to an environment variable and add the file to .gitignore. Rotate the exposed key immediately.",
		"aws-access-token":        "Move AWS credentials to ~/.aws/credentials or use IAM roles. Rotate the exposed access key via AWS Console.",
		"aws-secret-key":          "Move AWS credentials to ~/.aws/credentials or use IAM roles. Rotate the exposed secret key via AWS Console.",
		"github-token":            "Move the GitHub token to GitHub Secrets (Settings > Secrets) or a .env file. Rotate the exposed token.",
		"private-key":             "Move the private key to a secure location outside the repo. Generate a new key pair and rotate immediately.",
		"slack-token":             "Move the Slack token to an environment variable or secret manager. Rotate the exposed token.",
		"stripe-access-token":     "Move the Stripe key to an environment variable. Use Stripe's test mode keys for development. Rotate the exposed key.",
		"slack-webhook":           "Move the webhook URL to an environment variable. Rotate the webhook in Slack settings.",
		"google-api-key":          "Move the Google API key to environment variables. Restrict the key in Google Cloud Console and rotate it.",
		"mailgun-api-key":         "Move the Mailgun API key to an environment variable. Rotate the exposed key.",
		"sendgrid-api-key":        "Move the SendGrid API key to an environment variable. Rotate the exposed key.",
		"twilio-api-key":          "Move the Twilio API key to an environment variable. Rotate the exposed key.",
		"heroku-api-key":          "Move the Heroku API key to an environment variable. Rotate the exposed key.",
		"azure-storage-key":       "Move the Azure storage key to Azure Key Vault or environment variables. Rotate the exposed key.",
		"jwt":                     "Move the JWT secret to an environment variable. Use a strong random secret and rotate it.",
	}

	if hint, ok := hints[secretType]; ok {
		return hint
	}
	// Check partial matches
	for key, hint := range hints {
		if strings.Contains(secretType, key) {
			return hint
		}
	}
	return "Move this secret to an environment variable or secret manager, add the file to .gitignore, and rotate the credential immediately."
}

// --- Dead Code (knip) ---

// DeadcodeRemediation returns a fix hint based on the dead code rule ID.
func DeadcodeRemediation(ruleID string, message string) string {
	switch ruleID {
	case "unused-file":
		return "Delete this file if it's truly unused, or add an import to it if it's needed elsewhere."
	case "unused-export":
		return "Remove the `export` keyword from this symbol, or delete it entirely if unused."
	case "unused-type":
		return "Remove the `export` keyword from this type, or delete the type if unused."
	case "unused-dependency":
		// Extract package name from message like "Dependency is not imported: lodash"
		pkg := strings.TrimPrefix(message, "Dependency is not imported: ")
		pkg = strings.TrimSpace(pkg)
		if pkg != "" && pkg != message {
			return fmt.Sprintf("Remove unused dependency: run `npm uninstall %s`", pkg)
		}
		return "Remove this unused dependency from package.json with `npm uninstall <package>`."
	default:
		return "Remove this dead code or add a reference to it if it's needed."
	}
}

// --- Complexity ---

// ComplexityRemediation returns a fix hint for a complexity violation.
func ComplexityRemediation(message string) string {
	return "Reduce function complexity: extract helper functions, use guard clauses to reduce nesting, simplify conditional logic, and break into smaller focused functions."
}

// --- Duplication ---

// DuplicationRemediation returns a fix hint for a duplication finding.
func DuplicationRemediation(ruleID string) string {
	switch ruleID {
	case "duplication":
		return "Extract the duplicated logic into a shared utility function and call it from both locations."
	case "semantic-duplication":
		return "These functions are structurally similar. Extract common logic into a shared function with parameters for the differences."
	default:
		return "Extract the duplicated code into a shared function or utility module."
	}
}

// --- Coverage ---

// CoverageRemediation returns a fix hint for a low-coverage file.
func CoverageRemediation(message string) string {
	return "Add tests for the uncovered lines. Focus on edge cases, error paths, and branching logic to increase coverage."
}

// --- Semgrep ---

// SemgrepRemediation returns a fix hint based on the CWE tag or rule ID.
func SemgrepRemediation(ruleID string) string {
	// Extract CWE if present, e.g. "javascript.lang.security... [CWE-89]"
	cwe := ""
	if idx := strings.Index(ruleID, "[CWE-"); idx != -1 {
		end := strings.Index(ruleID[idx:], "]")
		if end != -1 {
			cwe = ruleID[idx+1 : idx+end]
		}
	}

	cweHints := map[string]string{
		"CWE-89":  "Use parameterized queries or an ORM to prevent SQL injection. Never concatenate user input into SQL strings.",
		"CWE-79":  "Sanitize user input before rendering in HTML. Use a templating engine that auto-escapes (e.g., React, handlebars).",
		"CWE-22":  "Validate and sanitize file paths. Use allowlists for permitted directories. Never pass user input directly to file system operations.",
		"CWE-78":  "Avoid passing user input to command execution. Use safe APIs instead of shell commands.",
		"CWE-200": "Do not log or expose sensitive data (passwords, tokens, keys). Use redaction in logs.",
		"CWE-327": "Use strong, modern encryption algorithms (AES-256-GCM, ChaCha20). Avoid DES, RC4, MD5, SHA1 for security.",
		"CWE-328": "Use a cryptographically secure random number generator for tokens, passwords, and keys.",
		"CWE-330": "Use `crypto.randomBytes()` (Node) or `secrets` module (Python) instead of `Math.random()` for security-sensitive values.",
		"CWE-502": "Validate and sanitize deserialized data. Avoid deserializing untrusted input. Use safe alternatives like JSON.",
		"CWE-400": "Add rate limiting, input size limits, and timeouts to prevent resource exhaustion attacks.",
		"CWE-798": "Use strong, unique passwords or API keys. Store them in a secret manager, not in source code.",
		"CWE-918": "Validate and allowlist server URLs. Disable redirects for outbound requests to prevent SSRF.",
		"CWE-94":  "Never evaluate user input as code. Use safe parsing instead of eval/Function constructors.",
		"CWE-190": "Validate numeric input bounds. Use safe integer parsing. Handle overflow/underflow conditions.",
		"CWE-20":  "Validate all user input against expected format, length, and character set before processing.",
		"CWE-352": "Add CSRF tokens to state-changing requests. Use SameSite cookie attribute.",
		"CWE-250": "Use proper access controls. Check user permissions before allowing sensitive operations.",
		"CWE-346": "Set appropriate security headers: Content-Security-Policy, X-Content-Type-Options, X-Frame-Options.",
		"CWE-601": "Validate redirect URLs against an allowlist. Use relative paths for internal redirects.",
		"CWE-295": "Validate TLS certificates properly. Do not disable certificate verification in production.",
	}

	if cwe != "" {
		if hint, ok := cweHints[cwe]; ok {
			return hint
		}
		return fmt.Sprintf("Fix the security vulnerability (%s): review the code for unsafe patterns and apply the appropriate security controls.", cwe)
	}

	// Generic fallback
	return "Review the flagged code for security issues. Apply the principle of least privilege and validate all external input."
}

// --- Custom Rules ---

// CustomRuleRemediation returns a fix hint based on the custom rule type.
func CustomRuleRemediation(ruleType string, message string) string {
	switch ruleType {
	case "ban-import":
		return "Remove the banned import and use an approved alternative instead."
	case "ban-pattern":
		return "Replace the matched pattern with an approved approach or remove it."
	case "require-import":
		return "Add the required import statement to this file."
	case "enforce-pattern":
		return "Add the required pattern to this file to comply with the project rule."
	case "max-lines":
		return "Split this file into smaller, focused modules to stay within the line limit."
	case "ban-dependency":
		return "Remove this dependency from package.json (`npm uninstall <package>`) and replace with an approved alternative."
	case "semgrep":
		return "Fix the code to match the semgrep rule requirement. Review the rule definition for details."
	case "advisory":
		return "Review the advisory and apply the recommended changes."
	default:
		return "Review and fix the issue according to the rule description."
	}
}
