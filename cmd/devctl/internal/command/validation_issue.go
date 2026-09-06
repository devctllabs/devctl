package command

import projectdomain "github.com/devctllabs/devctl/internal/domain/project"

// ValidationIssueDTO is the safe CLI representation of one project validation issue.
type ValidationIssueDTO struct {
	// Code identifies the stable validation rule that failed.
	Code string `json:"code"`
	// Path identifies the affected project file when available.
	Path string `json:"path,omitempty"`
	// Line and Column are one-based source coordinates; zero means unavailable.
	Line   int `json:"line,omitempty"`
	Column int `json:"column,omitempty"`
	// Field identifies the affected manifest field when available.
	Field string `json:"field,omitempty"`
	// Parameters contains rule-specific, presentation-safe facts.
	Parameters *ValidationParametersDTO `json:"parameters,omitempty"`
}

// ValidationParametersDTO carries issue-specific facts safe for CLI output.
type ValidationParametersDTO struct {
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Value    string `json:"value,omitempty"`
}

// ValidationIssueDTOs maps presentation-neutral issues to their shared CLI form.
func ValidationIssueDTOs(values []projectdomain.Issue) []ValidationIssueDTO {
	issues := make([]ValidationIssueDTO, 0, len(values))
	for _, issue := range values {
		var parameters *ValidationParametersDTO
		if issue.Parameters != nil {
			parameters = &ValidationParametersDTO{
				Expected: issue.Parameters.Expected,
				Actual:   issue.Parameters.Actual,
				Value:    issue.Parameters.Value,
			}
		}
		issues = append(issues, ValidationIssueDTO{
			Code: string(issue.Code), Path: issue.Path, Line: issue.Line, Column: issue.Column,
			Field: issue.Field, Parameters: parameters,
		})
	}
	return issues
}
