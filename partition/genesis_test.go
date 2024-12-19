package partition

import (
	gocrypto "crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/btcsuite/btcd/btcutil/base58"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/state"
)

var zeroHash hex.Bytes = make([]byte, 32)
var nodeID peer.ID = "test"

func TestNewGenesisPartitionNode_NotOk(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	validPDR := types.PartitionDescriptionRecord{
		Version:             1,
		NetworkID:   5,
		PartitionID: 1,
		TypeIDLen:           8,
		UnitIDLen:           128,
		T2Timeout:           5 * time.Second,
	}

	type args struct {
		state *state.State
		pdr   types.PartitionDescriptionRecord
		opts  []GenesisOption
	}

	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name:    "state is nil",
			args:    args{state: nil, pdr: validPDR},
			wantErr: ErrStateIsNil,
		},
		{
			name: "client signer is nil",
			args: args{
				state: state.NewEmptyState(),
				pdr:   validPDR,
				opts:  []GenesisOption{WithPeerID("1"), WithAuthPubKey(pubKeyBytes)},
			},
			wantErr: ErrSignerIsNil,
		},
		{
			name: "authentication public key is nil",
			args: args{
				state: state.NewEmptyState(),
				pdr:   validPDR,
				opts: []GenesisOption{
					WithSignPrivKey(signer),
					WithAuthPubKey(nil),
					WithPeerID("1")},
			},
			wantErr: ErrAuthPubKeyIsNil,
		},
		{
			name: "peer ID is empty",
			args: args{
				state: state.NewEmptyState(),
				pdr:   validPDR,
				opts: []GenesisOption{
					WithSignPrivKey(signer),
					WithPeerID(""),
				},
			},
			wantErr: genesis.ErrNodeIDIsEmpty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewNodeGenesis(tt.args.state, tt.args.pdr, tt.args.opts...)
			require.Nil(t, got)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}

	// invalid partition identifier
	got, err := NewNodeGenesis(
		state.NewEmptyState(),
		types.PartitionDescriptionRecord{Version: 1, NetworkID: 5, PartitionID: 0},
		WithPeerID("1"),
		WithSignPrivKey(signer),
		WithAuthPubKey(pubKeyBytes),
		WithHashAlgorithm(gocrypto.SHA256),
	)
	require.Nil(t, got)
	require.EqualError(t, err, `calculating genesis block hash: block hash calculation failed: invalid block: partition identifier is unassigned`)
}

func TestNewGenesisPartitionNode_Ok(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	pdr := types.PartitionDescriptionRecord{Version: 1, NetworkID: 5, PartitionID: 1, T2Timeout: 2500 * time.Millisecond}
	authKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	pn := createPartitionNode(t, signer, authKey, pdr, nodeID)
	require.NotNil(t, pn)
	require.Equal(t, base58.Encode([]byte(nodeID)), pn.NodeID)
	require.Equal(t, hex.Bytes(pubKey), pn.SignKey)
	blockCertificationRequestRequest := pn.BlockCertificationRequest
	require.Equal(t, pdr.PartitionID, blockCertificationRequestRequest.PartitionID)
	require.NoError(t, blockCertificationRequestRequest.IsValid(verifier))

	ir := blockCertificationRequestRequest.InputRecord
	expectedHash := hex.Bytes(make([]byte, 32))
	require.Equal(t, expectedHash, ir.Hash)
	require.Equal(t, zeroHash, ir.BlockHash)
	require.Equal(t, zeroHash, ir.PreviousHash)
}

func createPartitionNode(t *testing.T, nodeSigningKey crypto.Signer, authKey []byte, pdr types.PartitionDescriptionRecord, nodeID peer.ID) *genesis.PartitionNode {
	t.Helper()
	pn, err := NewNodeGenesis(
		state.NewEmptyState(),
		pdr,
		WithPeerID(nodeID),
		WithSignPrivKey(nodeSigningKey),
		WithAuthPubKey(authKey),
	)
	require.NoError(t, err)
	return pn
}
