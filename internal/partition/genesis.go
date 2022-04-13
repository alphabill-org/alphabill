package partition

import (
	gocrypto "crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"
	"github.com/libp2p/go-libp2p-core/peer"
)

var ErrSignerIsNil = errors.New("signer is nil")
var ErrInvalidSystemIdentifier = errors.New("system identifier is invalid")

type (
	genesisConf struct {
		peerID           peer.ID
		systemIdentifier []byte
		hashAlgorithm    gocrypto.Hash
		signer           crypto.Signer
	}

	Option func(c *genesisConf)
)

func (c genesisConf) isValid() error {
	if c.peerID == "" {
		return genesis.ErrNodeIdentifierIsEmpty
	}
	if c.signer == nil {
		return ErrSignerIsNil
	}
	if len(c.systemIdentifier) == 0 {
		return ErrInvalidSystemIdentifier
	}
	return nil
}

func WithPeerID(peerID peer.ID) Option {
	return func(c *genesisConf) {
		c.peerID = peerID
	}
}

func WithSystemIdentifier(systemIdentifier []byte) Option {
	return func(c *genesisConf) {
		c.systemIdentifier = systemIdentifier
	}
}

func WithHashAlgorithm(hashAlgorithm gocrypto.Hash) Option {
	return func(c *genesisConf) {
		c.hashAlgorithm = hashAlgorithm
	}
}

func WithSigner(signer crypto.Signer) Option {
	return func(c *genesisConf) {
		c.signer = signer
	}
}

// NewNodeGenesis creates a new genesis.PartitionNode from the given inputs. This function creates the first
// p1.P1Request by calling the TransactionSystem.RCompl function. Must contain PeerID, signer, and system identifier
// options:
//
//    pn, err := NewNodeGenesis(
//					txSystem,
//					WithPeerID(myPeerID),
//					WithSigner(signer),
//					WithSystemIdentifier(sysID),
//				)
//
// This function must be called by all partition nodes in the network.
func NewNodeGenesis(txSystem TransactionSystem, opts ...Option) (*genesis.PartitionNode, error) {
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
	hash, summaryValue := txSystem.Init()
	zeroHash := make([]byte, c.hashAlgorithm.Size())

	// first block
	b := &block.Block{
		SystemIdentifier:    c.systemIdentifier,
		TxSystemBlockNumber: 1,
		PreviousBlockHash:   zeroHash,
		Transactions:        nil,
	}
	blockHash := b.Hash(c.hashAlgorithm)

	// P1 request
	id := c.peerID.String()
	p1Request := &p1.P1Request{
		SystemIdentifier: c.systemIdentifier,
		NodeIdentifier:   id,
		RootRoundNumber:  1,
		InputRecord: &certificates.InputRecord{
			PreviousHash: zeroHash, // extend zero hash
			Hash:         hash,
			BlockHash:    blockHash,
			SummaryValue: summaryValue.Bytes(),
		},
	}
	err := p1Request.Sign(c.signer)
	if err != nil {
		return nil, err
	}

	verifier, err := c.signer.Verifier()
	if err != nil {
		return nil, err
	}

	if err := p1Request.IsValid(verifier); err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	pubKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, err
	}

	// partition node
	node := &genesis.PartitionNode{
		NodeIdentifier: id,
		PublicKey:      pubKey,
		P1Request:      p1Request,
	}
	return node, nil
}

// NewPartitionGenesis validates the given genesis.PartitionNode values, creates a genesis.PartitionRecord, and
// verifies the genesis.PartitionRecord.
//
// This function must be called by ONE partition node in the network. The result of this method must be sent to the
// root chain.
func NewPartitionGenesis(nodes []*genesis.PartitionNode, t2Timeout uint32) (*genesis.PartitionRecord, error) {
	if len(nodes) == 0 {
		return nil, genesis.ErrValidatorsMissing
	}
	// validate nodes
	for _, n := range nodes {
		if err := n.IsValid(); err != nil {
			return nil, err
		}
	}

	// create partition record
	pr := &genesis.PartitionRecord{
		SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
			SystemIdentifier: nodes[0].P1Request.SystemIdentifier,
			T2Timeout:        t2Timeout,
		},
		Validators: nodes,
	}

	// validate partition record
	if err := pr.IsValid(); err != nil {
		return nil, err
	}
	return pr, nil
}
