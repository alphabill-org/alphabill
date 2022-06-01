package partition

import (
	gocrypto "crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"
	"github.com/libp2p/go-libp2p-core/peer"
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
		initialBillValue      uint64
		dcMoneySupplyValue    uint64
	}

	GenesisOption func(c *genesisConf)
)

func (c genesisConf) isValid() error {
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

func WithInitialBillValue(initialBillValue uint64) GenesisOption {
	return func(c *genesisConf) {
		c.initialBillValue = initialBillValue
	}
}

func WithDCMoneySupplyValue(dcMoneySupplyValue uint64) GenesisOption {
	return func(c *genesisConf) {
		c.dcMoneySupplyValue = dcMoneySupplyValue
	}
}

// NewNodeGenesis creates a new genesis.PartitionNode from the given inputs. This function creates the first
// p1.P1Request by calling the TransactionSystem.EndBlock function. Must contain PeerID, signer, and system identifier
// and public encryption key configuration:
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
			SummaryValue: summaryValue,
		},
	}
	err = p1Request.Sign(c.signer)
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

	signingPubKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, err
	}

	// partition node
	node := &genesis.PartitionNode{
		NodeIdentifier:      id,
		SigningPublicKey:    signingPubKey,
		EncryptionPublicKey: c.encryptionPubKeyBytes,
		P1Request:           p1Request,
		T2Timeout:           c.t2Timeout,
		InitialBillValue:    c.initialBillValue,
		DcMoneySupplyValue:  c.dcMoneySupplyValue,
	}
	if err := node.IsValid(); err != nil {
		return nil, err
	}
	return node, nil
}
