package partition

import (
	gocrypto "crypto"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/partition"

	"github.com/btcsuite/btcutil/base58"

	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"

	"github.com/stretchr/testify/require"

	"github.com/libp2p/go-libp2p-core/peer"
)

var zeroHash = make([]byte, 32)
var systemIdentifier = []byte{0, 0, 0, 1}
var nodeID peer.ID = "test"
var nodeID2 peer.ID = "test2"

func TestNewGenesisPartitionNode_InvalidInputs(t *testing.T) {
	type args struct {
		txSystem TransactionSystem
		peerID   peer.ID
		conf     *Configuration
	}

	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name:    "tx system is nil",
			args:    args{txSystem: nil},
			wantErr: ErrTxSystemIsNil,
		},
		{
			name: "peer ID is empty",
			args: args{
				txSystem: &partition.MockTxSystem{},
				peerID:   "",
				conf:     &Configuration{SystemIdentifier: systemIdentifier},
			},
			wantErr: ErrPeerIDIsEmpty,
		},
		{
			name: "configuration is nil",
			args: args{
				txSystem: &partition.MockTxSystem{},
				peerID:   nodeID,
				conf:     nil,
			},
			wantErr: ErrPartitionConfigurationIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewGenesisPartitionNode(tt.args.txSystem, tt.args.peerID, tt.args.conf)
			require.Nil(t, got)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestNewGenesisPartitionNode_Ok(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	pn := createPartitionNode(t, signer, systemIdentifier, nodeID)
	require.NotNil(t, pn)
	require.Equal(t, base58.Encode([]byte(nodeID)), pn.NodeIdentifier)
	require.Equal(t, pubKey, pn.PublicKey)
	p1Request := pn.P1Request
	require.Equal(t, systemIdentifier, p1Request.SystemIdentifier)
	require.Equal(t, uint64(1), p1Request.RootRoundNumber)
	require.NoError(t, p1Request.IsValid(verifier))

	ir := p1Request.InputRecord
	expectedHash := make([]byte, 32)
	expectedHash[0] = 1
	require.Equal(t, expectedHash, ir.Hash)
	require.Equal(t, calculateBlockHash(1, systemIdentifier, zeroHash), ir.BlockHash)
	require.Equal(t, zeroHash, ir.PreviousHash)
}

func TestNewGenesisPartitionRecord_InvalidInputs(t *testing.T) {
	signer, _ := testsig.CreateSignerAndVerifier(t)
	type args struct {
		nodes     []*genesis.PartitionNode
		t2Timeout uint32
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "validators missing",
			args: args{
				nodes: nil,
			},
			wantErr: genesis.ErrValidatorsMissing,
		},
		{
			name: "invalid partition node",
			args: args{
				nodes: []*genesis.PartitionNode{
					{
						NodeIdentifier: "",
					},
				},
				t2Timeout: 10,
			},
			wantErr: genesis.ErrNodeIdentifierIsEmpty,
		},
		{
			name: "invalid timeout",
			args: args{
				nodes: []*genesis.PartitionNode{
					createPartitionNode(t, signer, systemIdentifier, nodeID),
				},
				t2Timeout: 0,
			},
			wantErr: genesis.ErrT2TimeoutIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewGenesisPartitionRecord(tt.args.nodes, tt.args.t2Timeout)
			require.Nil(t, got)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestNewGenesisPartitionRecord_Ok(t *testing.T) {
	var timeout uint32 = 10
	signer1, _ := testsig.CreateSignerAndVerifier(t)
	signer2, _ := testsig.CreateSignerAndVerifier(t)
	pn1 := createPartitionNode(t, signer1, systemIdentifier, nodeID)
	pn2 := createPartitionNode(t, signer2, systemIdentifier, nodeID2)
	nodes := []*genesis.PartitionNode{pn1, pn2}
	pr, err := NewGenesisPartitionRecord(nodes, timeout)
	require.NoError(t, err)
	require.NotNil(t, pr)
	require.Equal(t, timeout, pr.SystemDescriptionRecord.T2Timeout)
	require.Equal(t, systemIdentifier, pr.SystemDescriptionRecord.SystemIdentifier)
	require.Equal(t, nodes, pr.Validators)
}

func createPartitionNode(t *testing.T, signer crypto.Signer, systemIdentifier []byte, nodeIdentifier peer.ID) *genesis.PartitionNode {
	t.Helper()
	txSystem := &partition.MockTxSystem{}
	conf := &Configuration{
		SystemIdentifier: systemIdentifier,
		HashAlgorithm:    gocrypto.SHA256,
		Signer:           signer,
	}
	pn, err := NewGenesisPartitionNode(txSystem, nodeIdentifier, conf)
	require.NoError(t, err)
	return pn
}

func calculateBlockHash(blockNr uint64, systemIdentifier, previousHash []byte) []byte {
	hasher := gocrypto.SHA256.New()
	hasher.Write(systemIdentifier)
	hasher.Write(util.Uint64ToBytes(blockNr))
	hasher.Write(previousHash)
	return hasher.Sum(nil)
}
