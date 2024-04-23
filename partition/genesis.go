package partition

import (
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	pg "github.com/alphabill-org/alphabill/partition/genesis"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

var ErrStateIsNil = errors.New("state is nil")
var ErrSignerIsNil = errors.New("signer is nil")
var ErrEncryptionPubKeyIsNil = errors.New("encryption public key is nil")
var errInvalidSystemIdentifier = errors.New("invalid transaction system identifier")

type (
	genesisConf struct {
		peerID                peer.ID
		systemIdentifier      types.SystemID
		hashAlgorithm         gocrypto.Hash
		signer                crypto.Signer
		encryptionPubKeyBytes []byte
		t2Timeout             uint32
		params                []byte
	}

	GenesisOption func(c *genesisConf)
)

func (c *genesisConf) isValid() error {
	if c.peerID == "" {
		return genesis.ErrNodeIdentifierIsEmpty
	}
	if c.signer == nil {
		return ErrSignerIsNil
	}
	if len(c.encryptionPubKeyBytes) == 0 {
		return ErrEncryptionPubKeyIsNil
	}
	if c.systemIdentifier == 0 {
		return errInvalidSystemIdentifier
	}
	return nil
}

func WithPeerID(peerID peer.ID) GenesisOption {
	return func(c *genesisConf) {
		c.peerID = peerID
	}
}

func WithSystemIdentifier(systemIdentifier types.SystemID) GenesisOption {
	return func(c *genesisConf) {
		c.systemIdentifier = systemIdentifier
	}
}

func WithHashAlgorithm(hashAlgorithm gocrypto.Hash) GenesisOption {
	return func(c *genesisConf) {
		c.hashAlgorithm = hashAlgorithm
	}
}

func WithSigningKey(signer crypto.Signer) GenesisOption {
	return func(c *genesisConf) {
		c.signer = signer
	}
}

func WithEncryptionPubKey(encryptionPubKey []byte) GenesisOption {
	return func(c *genesisConf) {
		c.encryptionPubKeyBytes = encryptionPubKey
	}
}

func WithT2Timeout(t2Timeout uint32) GenesisOption {
	return func(c *genesisConf) {
		c.t2Timeout = t2Timeout
	}
}

func WithParams(params []byte) GenesisOption {
	return func(c *genesisConf) {
		c.params = params
	}
}

// NewNodeGenesis creates a new genesis.PartitionNode from the given inputs. This function creates the first
// block certification request by calling the TransactionSystem.EndBlock function. Must contain PeerID, signer, and
// system identifier and public encryption key configuration:
//
//	   pn, err := NewNodeGenesis(
//						txSystem,
//						WithPeerID(myPeerID),
//						WithSigningKey(signer),
//						WithSystemIdentifier(sysID),
//						WithEncryptionPubKey(encPubKey),
//					)
//
// This function must be called by all partition nodes in the network.
func NewNodeGenesis(state *state.State, opts ...GenesisOption) (*genesis.PartitionNode, error) {
	if state == nil {
		return nil, ErrStateIsNil
	}
	c := &genesisConf{
		hashAlgorithm: gocrypto.SHA256,
	}

	for _, option := range opts {
		option(c)
	}

	if err := c.isValid(); err != nil {
		return nil, err
	}

	zeroHash := make([]byte, c.hashAlgorithm.Size())
	summaryValue, hash, err := state.CalculateRoot()
	if err != nil {
		return nil, err
	}
	if hash == nil {
		hash = zeroHash
	}
	// calculate block hash
	gIR := &types.InputRecord{
		PreviousHash: zeroHash, // extend zero hash
		Hash:         hash,
		RoundNumber:  pg.PartitionRoundNumber,
		SummaryValue: util.Uint64ToBytes(summaryValue),
	}
	// create genesis block
	gBlock := &types.Block{
		Header: &types.Header{
			SystemID:          c.systemIdentifier,
			ProposerID:        "genesis",
			PreviousBlockHash: zeroHash,
		},
		Transactions: make([]*types.TransactionRecord, 0),
		UnicityCertificate: &types.UnicityCertificate{
			InputRecord: gIR,
		},
	}
	// calculate first block hash
	blockHash, err := gBlock.Hash(c.hashAlgorithm)
	if err != nil {
		return nil, fmt.Errorf("geneis block hash calculation failed")
	}
	gIR.BlockHash = blockHash
	id := c.peerID.String()
	// Protocol request
	blockCertificationRequest := &certification.BlockCertificationRequest{
		SystemIdentifier: c.systemIdentifier,
		NodeIdentifier:   id,
		InputRecord:      gIR,
		RootRoundNumber:  pg.RootRoundNumber,
	}
	if err := blockCertificationRequest.Sign(c.signer); err != nil {
		return nil, err
	}

	verifier, err := c.signer.Verifier()
	if err != nil {
		return nil, err
	}

	if err := blockCertificationRequest.IsValid(verifier); err != nil {
		return nil, err
	}

	signingPubKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, err
	}

	// partition node
	node := &genesis.PartitionNode{
		NodeIdentifier:            id,
		SigningPublicKey:          signingPubKey,
		EncryptionPublicKey:       c.encryptionPubKeyBytes,
		BlockCertificationRequest: blockCertificationRequest,
		T2Timeout:                 c.t2Timeout,
		Params:                    c.params,
	}
	if err := node.IsValid(); err != nil {
		return nil, err
	}
	return node, nil
}
