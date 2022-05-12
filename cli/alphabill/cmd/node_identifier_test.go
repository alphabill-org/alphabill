package cmd

import (
	"context"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIdentifier_KeysNotFound(t *testing.T) {
	dir := setupTestHomeDir(t, "identifier")
	file := path.Join(dir, keysFile)
	cmd := New()
	args := "identifier -k" + file
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("failed to load keys %s", file))
}

func TestIdentifier_Ok(t *testing.T) {
	dir := setupTestHomeDir(t, "identifier")
	file := path.Join(dir, keysFile)

	_, err := LoadKeys(file, true)
	require.NoError(t, err)
	cmd := New()
	args := "identifier -k" + file
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)
}
