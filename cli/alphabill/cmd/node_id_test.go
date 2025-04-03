package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/stretchr/testify/require"
)

func TestNodeID_KeyConfNotFound(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, keyConfFileName)
	cmd := New(testobserve.NewFactory(t))
	cmd.baseCmd.SetArgs([]string{
		"node-id", "--key-conf", file,
	})
	err := cmd.Execute(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("failed to load %q", file))
}

func TestNodeID_Ok(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, keyConfFileName)

	flags := &keyConfFlags{}
	flags.KeyConfFile = file

	// Generate keyConf to file
	_, err := flags.loadKeyConf(&baseFlags{}, true)
	require.NoError(t, err)

	cmd := New(testobserve.NewFactory(t))
	cmd.baseCmd.SetArgs([]string{
		"node-id", "--key-conf", file,
	})
	err = cmd.Execute(context.Background())
	require.NoError(t, err)
}
