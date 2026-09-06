package project

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidationResultIsValid(t *testing.T) {
	t.Parallel()

	require.True(t, ValidationResult{}.IsValid())
	require.False(t, ValidationResult{Issues: []Issue{{}}}.IsValid())
}
