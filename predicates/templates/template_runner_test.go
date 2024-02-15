package templates

import (
	"testing"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/stretchr/testify/require"
)

func TestTemplateRunner(t *testing.T) {
	t.Parallel()

	t.Run("nil predicate", func(t *testing.T) {
		err := runner.Execute(nil, nil)
		require.ErrorContains(t, err, "predicate is nil")
	})

	t.Run("invalid predicate tag", func(t *testing.T) {
		err := runner.Execute(&predicates.Predicate{Tag: 0x01}, nil)
		require.ErrorContains(t, err, "invalid predicate tag")
	})

	t.Run("predicate code length not 1", func(t *testing.T) {
		err := runner.Execute(&predicates.Predicate{Tag: TemplateStartByte, Code: []byte{0xAF, 0xAF}}, nil)
		require.ErrorContains(t, err, "expected predicate code length to be 1, got: 2")
	})

	t.Run("unknown predicate template", func(t *testing.T) {
		err := runner.Execute(&predicates.Predicate{Tag: TemplateStartByte, Code: []byte{0xAF}}, nil)
		require.ErrorContains(t, err, "unknown predicate template")
	})

	t.Run("template execution error", func(t *testing.T) {
		err := runner.Execute(&predicates.Predicate{Tag: TemplateStartByte, Code: []byte{AlwaysFalseID}}, nil)
		require.ErrorContains(t, err, "always false")
	})

	t.Run("success", func(t *testing.T) {
		err := runner.Execute(&predicates.Predicate{Tag: TemplateStartByte, Code: []byte{AlwaysTrueID}}, nil)
		require.NoError(t, err)
	})
}
