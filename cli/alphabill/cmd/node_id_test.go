package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/stretchr/testify/require"
)

func TestIdentifier_KeysNotFound(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, keyConfFileName)
	cmd := New(testobserve.NewFactory(t))
	args := "identifier -k" + file
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.Execute(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("failed to load keys %s", file))
}

func TestIdentifier_Ok(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, keyConfFileName)

	_, err := LoadKeys(file, true, false)
	require.NoError(t, err)
	cmd := New(testobserve.NewFactory(t))
	args := "identifier -k" + file
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)
}
