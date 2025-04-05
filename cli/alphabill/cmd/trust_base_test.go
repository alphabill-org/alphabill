package cmd

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/stretchr/testify/require"
)

func TestTrustBaseGenerateAndSign(t *testing.T) {
	logF := testobserve.NewFactory(t)

	// root node 1
	homeDir1 := t.TempDir()
	cmd := New(logF)
	cmd.baseCmd.SetArgs([]string{
		"root-node", "init", "--home", homeDir1, "--generate",
	})
	require.NoError(t, cmd.Execute(context.Background()))
	nodeInfoFile1 := filepath.Join(homeDir1, nodeInfoFileName)

	// root node 2
	homeDir2 := t.TempDir()
	cmd = New(logF)
	cmd.baseCmd.SetArgs([]string{
		"root-node", "init", "--home", homeDir2, "--generate",
	})
	require.NoError(t, cmd.Execute(context.Background()))
	nodeInfoFile2 := filepath.Join(homeDir2, nodeInfoFileName)

	cmd = New(logF)
	cmd.baseCmd.SetArgs([]string{
		"trust-base", "generate",
		"--home", homeDir1,
		"--node-info", nodeInfoFile1,
		"--node-info", nodeInfoFile2,
		"--network-id", "5",
		"--quorum-threshold", "2",
	})
	require.NoError(t, cmd.Execute(context.Background()))

	// verify the resulting file
	trustBasePath := filepath.Join(homeDir1, "trust-base.json")
	trustBase, err := util.ReadJsonFile(trustBasePath, &types.RootTrustBaseV1{})
	require.NoError(t, err)
	require.Equal(t, types.NetworkID(5), trustBase.NetworkID)
	require.Equal(t, uint64(1), trustBase.Epoch)
	require.Len(t, trustBase.RootNodes, 2)

	// root node 1 signs the trust base in its home dir
	cmd = New(logF)
	cmd.baseCmd.SetArgs([]string{"trust-base", "sign", "--home", homeDir1})
	require.NoError(t, cmd.Execute(context.Background()))

	// root node 2 signs the trust base at custom location
	cmd = New(logF)
	cmd.baseCmd.SetArgs([]string{"trust-base", "sign", "--home", homeDir2, "--trust-base", trustBasePath})
	require.NoError(t, cmd.Execute(context.Background()))

	// verify trust base has 2 signatures
	trustBase, err = util.ReadJsonFile(filepath.Join(homeDir1, "trust-base.json"), &types.RootTrustBaseV1{})
	require.NoError(t, err)
	require.Len(t, trustBase.GetRootNodes(), 2)
	require.Len(t, trustBase.Signatures, 2)
}
