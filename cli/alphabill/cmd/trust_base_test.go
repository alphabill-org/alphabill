package cmd

import (
	// "context"
	// "fmt"
	// "path/filepath"
	// "strconv"
	// "strings"
	// "testing"

	// "github.com/alphabill-org/alphabill-go-base/types"
	// testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	// "github.com/stretchr/testify/require"
)

// TODO: re-enable
// func TestTrustBaseGenerateAndSign(t *testing.T) {
// 	homeDir := t.TempDir()
// 	logF := testobserve.NewFactory(t)

// 	// create root genesis files
// 	consensus := consensusParams{totalNodes: 4}
// 	genesisFiles := createRootGenesisFiles(t, homeDir, consensus)
// 	genesisArg := fmt.Sprintf("%s,%s,%s,%s", genesisFiles[0], genesisFiles[1], genesisFiles[2], genesisFiles[3])

// 	// merge to distributed root genesis
// 	outputDir := filepath.Join(homeDir, "result")
// 	cmd := New(logF)
// 	args := "root-genesis combine --home " + homeDir +
// 		" -o " + outputDir +
// 		" --root-genesis=" + genesisArg
// 	cmd.baseCmd.SetArgs(strings.Split(args, " "))
// 	require.NoError(t, cmd.Execute(context.Background()))

// 	// create trust base file
// 	mergedRootGenesisFile := filepath.Join(outputDir, rootGenesisFileName)
// 	cmd = New(logF)
// 	args = "root-genesis gen-trust-base --home " + homeDir +
// 		" --root-genesis=" + mergedRootGenesisFile
// 	cmd.baseCmd.SetArgs(strings.Split(args, " "))
// 	require.NoError(t, cmd.Execute(context.Background()))

// 	// verify trust base file
// 	trustBaseFile := filepath.Join(homeDir, rootTrustBaseFileName)
// 	tb, err := types.NewTrustBaseFromFile(trustBaseFile)
// 	require.NoError(t, err)
// 	require.Len(t, tb.RootNodes, 4)

// 	for i := uint8(8); i < consensus.totalNodes; i++ {
// 		cmd = New(logF)
// 		rootNodeDir := filepath.Join(homeDir, defaultRootChainDir+strconv.Itoa(int(i)))
// 		rootNodeKeyFile := filepath.Join(rootNodeDir, defaultKeysFileName)
// 		args = "root-genesis sign-trust-base --home " + homeDir + " -k " + rootNodeKeyFile
// 		cmd.baseCmd.SetArgs(strings.Split(args, " "))
// 		require.NoError(t, cmd.Execute(context.Background()))
// 	}

// 	// verify trust base file contains correct signatures
// 	tb, err = types.NewTrustBaseFromFile(trustBaseFile)
// 	require.NoError(t, err)
// 	require.Len(t, tb.GetRootNodes(), int(consensus.totalNodes))
// }
