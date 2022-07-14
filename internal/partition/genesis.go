package partition

import (
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/libp2p/go-libp2p-core/peer"
	"google.golang.org/protobuf/types/known/anypb"
)

var ErrSignerIsNil = errors.New("signer is nil")
var ErrEncryptionPubKeyIsNil = errors.New("encryption public key is nil")
var ErrInvalidSystemIdentifier = errors.New("system identifier is invalid")

type (
	genesisConf struct {
		peerID                peer.ID
		systemIdentifier      []byte
		hashAlgorithm         gocrypto.Hash
		signer                crypto.Signer
		encryptionPubKeyBytes []byte
		t2Timeout             uint32
		params                *anypb.Any
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
	if len(c.systemIdentifier) == 0 {
		return ErrInvalidSystemIdentifier
	}
	return nil
}

func WithPeerID(peerID peer.ID) GenesisOption {
	return func(c *genesisConf) {
		c.peerID = peerID
	}
}

func WithSystemIdentifier(systemIdentifier []byte) GenesisOption {
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

func WithParams(params *anypb.Any) GenesisOption {
	return func(c *genesisConf) {
		c.params = params
	}
}

// NewNodeGenesis creates a new genesis.PartitionNode from the given inputs. This function creates the first
// block certification request by calling the TransactionSystem.EndBlock function. Must contain PeerID, signer, and
// system identifier and public encryption key configuration:
//
//    pn, err := NewNodeGenesis(
//					txSystem,
//					WithPeerID(myPeerID),
//					WithSigningKey(signer),
//					WithSystemIdentifier(sysID),
// 					WithEncryptionPubKey(encPubKey),
//				)
//
// This function must be called by all partition nodes in the network.
func NewNodeGenesis(txSystem txsystem.TransactionSystem, opts ...GenesisOption) (*genesis.PartitionNode, error) {
	if txSystem == nil {
		return nil, ErrTxSystemIsNil
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

	// create the first round of the tx system
	state, err := txSystem.State()
	if err != nil {
		return nil, err
	}
	hash := state.Root()
	summaryValue := state.Summary()

	zeroHash := make([]byte, c.hashAlgorithm.Size())

	// first block
	b := &block.Block{
		SystemIdentifier:  c.systemIdentifier,
		BlockNumber:       1,
		PreviousBlockHash: zeroHash,
		Transactions:      nil,
	}
	blockHash := b.Hash(c.hashAlgorithm)

	// Protocol request
	id := c.peerID.String()
	blockCertificationRequest := &certification.BlockCertificationRequest{
		SystemIdentifier: c.systemIdentifier,
		NodeIdentifier:   id,
		RootRoundNumber:  1,
		InputRecord: &certificates.InputRecord{
			PreviousHash: zeroHash, // extend zero hash
			Hash:         hash,
			BlockHash:    blockHash,
			SummaryValue: summaryValue,
		},
	}
	err = blockCertificationRequest.Sign(c.signer)
	if err != nil {
		return nil, err
	}

	verifier, err := c.signer.Verifier()
	if err != nil {
		return nil, err
	}

	if err := blockCertificationRequest.IsValid(verifier); err != nil {
		return nil, err
	}
	if err != nil {
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
