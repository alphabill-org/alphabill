package templates

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/predicates"
	"github.com/stretchr/testify/require"
)

func TestTemplateRunner(t *testing.T) {
	t.Parallel()

	t.Run("nil predicate", func(t *testing.T) {
		err := runner.Execute(nil, nil, nil)
		require.ErrorContains(t, err, "predicate is nil")
	})

	t.Run("invalid predicate tag", func(t *testing.T) {
		err := runner.Execute(&predicates.Predicate{Tag: 0x01}, nil, nil)
		require.ErrorContains(t, err, "invalid predicate tag")
	})

	t.Run("unknown predicate template", func(t *testing.T) {
		err := runner.Execute(&predicates.Predicate{Tag: TemplateStartByte, ID: 0xAF}, nil, nil)
		require.ErrorContains(t, err, "unknown predicate template")
	})

	t.Run("template execution error", func(t *testing.T) {
		err := runner.Execute(&predicates.Predicate{Tag: TemplateStartByte, ID: AlwaysFalseID}, nil, nil)
		require.ErrorContains(t, err, "always false")
	})

	t.Run("success", func(t *testing.T) {
		err := runner.Execute(&predicates.Predicate{Tag: TemplateStartByte, ID: AlwaysTrueID}, nil, nil)
		require.NoError(t, err)
	})
}