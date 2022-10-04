package partition

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/btcsuite/btcd/btcutil/base58"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

var zeroHash = make([]byte, 32)
var systemIdentifier = []byte{0, 0, 0, 1}
var nodeID peer.ID = "test"

func TestNewGenesisPartitionNode_NotOk(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	type args struct {
		txSystem txsystem.TransactionSystem
		opts     []GenesisOption
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
			name: "client signer is nil",
			args: args{
				txSystem: &testtxsystem.CounterTxSystem{},
				opts:     []GenesisOption{WithSystemIdentifier(systemIdentifier), WithPeerID("1")},
			},
			wantErr: ErrSignerIsNil,
		},
		{
			name: "encryption public key is nil",
			args: args{
				txSystem: &testtxsystem.CounterTxSystem{},
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
				txSystem: &testtxsystem.CounterTxSystem{},
				opts: []GenesisOption{
					WithSystemIdentifier(nil),
					WithPeerID("1"),
					WithSigningKey(signer),
					WithEncryptionPubKey(pubKeyBytes),
					WithHashAlgorithm(gocrypto.SHA256),
				},
			},
			wantErr: ErrInvalidSystemIdentifier,
		},
		{
			name: "peer ID is empty",
			args: args{
				txSystem: &testtxsystem.CounterTxSystem{},
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
			got, err := NewNodeGenesis(tt.args.txSystem, tt.args.opts...)
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
	require.Equal(t, uint64(1), blockCertificationRequestRequest.RootRoundNumber)
	require.NoError(t, blockCertificationRequestRequest.IsValid(verifier))

	ir := blockCertificationRequestRequest.InputRecord
	expectedHash := make([]byte, 32)
	require.Equal(t, expectedHash, ir.Hash)
	require.Equal(t, calculateBlockHash(1, systemIdentifier, nil), ir.BlockHash)
	require.Equal(t, zeroHash, ir.PreviousHash)
}

func createPartitionNode(t *testing.T, nodeSigningKey crypto.Signer, nodeEncryptionPublicKey crypto.Verifier, systemIdentifier []byte, nodeIdentifier peer.ID) *genesis.PartitionNode {
	t.Helper()
	txSystem := &testtxsystem.CounterTxSystem{}
	encPubKeyBytes, err := nodeEncryptionPublicKey.MarshalPublicKey()
	require.NoError(t, err)
	pn, err := NewNodeGenesis(
		txSystem,
		WithPeerID(nodeIdentifier),
		WithSystemIdentifier(systemIdentifier),
		WithSigningKey(nodeSigningKey),
		WithEncryptionPubKey(encPubKeyBytes),
		WithT2Timeout(2500),
	)
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
