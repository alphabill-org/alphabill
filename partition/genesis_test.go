package partition

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/btcsuite/btcd/btcutil/base58"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

var zeroHash = make([]byte, 32)
var systemIdentifier = types.SystemID([]byte{0, 0, 0, 1})
var nodeID peer.ID = "test"

func TestNewGenesisPartitionNode_NotOk(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	type args struct {
		state *state.State
		opts  []GenesisOption
	}

	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name:    "state is nil",
			args:    args{state: nil},
			wantErr: ErrStateIsNil,
		},
		{
			name: "client signer is nil",
			args: args{
				state: state.NewEmptyState(),
				opts: []GenesisOption{WithSystemIdentifier(systemIdentifier), WithPeerID("1")},
			},
			wantErr: ErrSignerIsNil,
		},
		{
			name: "encryption public key is nil",
			args: args{
				state: state.NewEmptyState(),
				opts: []GenesisOption{
					WithSystemIdentifier(systemIdentifier),
					WithSigningKey(signer),
					WithEncryptionPubKey(nil),
					WithPeerID("1")},
			},
			wantErr: ErrEncryptionPubKeyIsNil,
		},
		{
			name: "invalid system identifier",
			args: args{
				state: state.NewEmptyState(),
				opts: []GenesisOption{
					WithSystemIdentifier(nil),
					WithPeerID("1"),
					WithSigningKey(signer),
					WithEncryptionPubKey(pubKeyBytes),
					WithHashAlgorithm(gocrypto.SHA256),
				},
			},
			wantErr: errInvalidSystemIdentifier,
		},
		{
			name: "peer ID is empty",
			args: args{
				state: state.NewEmptyState(),
				opts: []GenesisOption{
					WithSystemIdentifier(systemIdentifier),
					WithSigningKey(signer),
					WithPeerID(""),
				},
			},
			wantErr: genesis.ErrNodeIdentifierIsEmpty,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewNodeGenesis(tt.args.state, tt.args.opts...)
			require.Nil(t, got)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestNewGenesisPartitionNode_Ok(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	pn := createPartitionNode(t, signer, verifier, systemIdentifier, nodeID)
	require.NotNil(t, pn)
	require.Equal(t, base58.Encode([]byte(nodeID)), pn.NodeIdentifier)
	require.Equal(t, pubKey, pn.SigningPublicKey)
	blockCertificationRequestRequest := pn.BlockCertificationRequest
	require.Equal(t, systemIdentifier, blockCertificationRequestRequest.SystemIdentifier)
	require.NoError(t, blockCertificationRequestRequest.IsValid(verifier))

	ir := blockCertificationRequestRequest.InputRecord
	expectedHash := make([]byte, 32)
	require.Equal(t, expectedHash, ir.Hash)
	require.Equal(t, calculateBlockHash(systemIdentifier, nil, true), ir.BlockHash)
	require.Equal(t, zeroHash, ir.PreviousHash)
}

func createPartitionNode(t *testing.T, nodeSigningKey crypto.Signer, nodeEncryptionPublicKey crypto.Verifier, systemIdentifier []byte, nodeIdentifier peer.ID) *genesis.PartitionNode {
	t.Helper()
	encPubKeyBytes, err := nodeEncryptionPublicKey.MarshalPublicKey()
	require.NoError(t, err)
	pn, err := NewNodeGenesis(
		state.NewEmptyState(),
		WithPeerID(nodeIdentifier),
		WithSystemIdentifier(systemIdentifier),
		WithSigningKey(nodeSigningKey),
		WithEncryptionPubKey(encPubKeyBytes),
		WithT2Timeout(2500),
	)
	require.NoError(t, err)
	return pn
}

func calculateBlockHash(systemIdentifier, previousHash []byte, isEmpty bool) []byte {
	// blockhash = hash(header_hash, raw_txs_hash, mt_root_hash)
	hasher := gocrypto.SHA256.New()
	if isEmpty {
		return zeroHash
	}
	hasher.Write(systemIdentifier)
	hasher.Write(previousHash)
	headerHash := hasher.Sum(nil)
	hasher.Reset()

	txsHash := hasher.Sum(nil)
	hasher.Reset()

	treeHash := make([]byte, gocrypto.SHA256.Size())

	return hash.Sum(gocrypto.SHA256, headerHash, txsHash, treeHash)
}
