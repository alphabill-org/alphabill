//go:build manual

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
	"github.com/stretchr/testify/require"
)

const partitionID1 types.SystemID = 1

var genesisInputRecord = &types.InputRecord{
	PreviousHash: make([]byte, 32),
	Hash:         []byte{1, 1, 1, 1},
	BlockHash:    []byte{0, 0, 1, 2},
	SummaryValue: []byte{0, 0, 1, 3},
	RoundNumber:  1,
}

func Test(t *testing.T) {
	ss := []string{"A", "B", "C"}
	for _, s := range ss {
		gen := generateGenesis(t)
		file := filepath.Join(".", fmt.Sprintf("root-genesis-%s.json", s))
		err := os.WriteFile(file, []byte(gen), 0666)
		require.NoError(t, err)
	}
}

func generateGenesis(t *testing.T) string {
	_, partitionRecord := testutils.CreatePartitionNodesAndPartitionRecord(t, genesisInputRecord, partitionID1, 1)
	rootNode := testutils.NewTestNode(t)
	verifier := rootNode.Verifier
	rootPubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	id := rootNode.PeerConf.ID
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), rootNode.Signer, rootPubKeyBytes, []*genesis.PartitionRecord{partitionRecord})
	require.NoError(t, err)

	bs, err := json.MarshalIndent(rootGenesis, "", "  ")
	require.NoError(t, err)
	return string(bs)
}
