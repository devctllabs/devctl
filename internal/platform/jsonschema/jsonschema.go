package jsonschema

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RootTitle returns the non-empty root title that owns generated top-level type naming.
func RootTitle(data []byte) (string, error) {
	var document struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return "", fmt.Errorf("json.Unmarshal: %w", err)
	}
	title := strings.TrimSpace(document.Title)
	if title == "" {
		return "", fmt.Errorf("root title is required")
	}
	return title, nil
}
