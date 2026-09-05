package lint

// Command selects the configured contracts to lint.
type Command struct {
	ManifestPath string
	Family       string
}

// Result reports findings collected before success or the first execution error.
// Findings are normal results: Valid is false when Issues is non-empty.
type Result struct {
	Valid     bool
	Contracts []string
	Issues    []Issue
}

// Issue is one stable, presentation-neutral devctl lint finding.
type Issue struct {
	Code   string
	Target string
	Path   string
	// Line and Column are one-based; zero means the location is unavailable.
	Line       int
	Column     int
	Field      string
	Parameters *Parameters
}

// Parameters carries code-specific facts used by delivery renderers.
type Parameters struct {
	OperationID string
	Location    string
	Type        string
	Subtype     string
	SpecPath    string
}
