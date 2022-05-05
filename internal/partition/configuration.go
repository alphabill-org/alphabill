package partition

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
)

const DefaultT1Timeout = 750 * time.Millisecond

var (
	ErrEventBusIsNil       = errors.New("event bus is nil")
	ErrTxSystemIsNil       = errors.New("transaction system is nil")
	ErrPeerIsNil           = errors.New("peer is nil")
	ErrGenesisIsNil        = errors.New("genesis is nil")
	ErrInvalidRootHash     = errors.New("tx system root hash does not equal to genesis file hash")
	ErrInvalidSummaryValue = errors.New("tx system summary value does not equal to genesis file summary value")
)

type (
	configuration struct {
		context                     context.Context
		txValidator                 TxValidator
		unicityCertificateValidator UnicityCertificateValidator
		blockProposalValidator      BlockProposalValidator
		leaderSelector              LeaderSelector
		blockStore                  store.BlockStore
		t1Timeout                   time.Duration // T1 timeout of the node. Time to wait before node creates a new block proposal.
		hashAlgorithm               gocrypto.Hash // make hash algorithm configurable in the future. currently it is using SHA-256.
		peer                        *network.Peer
		signer                      crypto.Signer
		eventbus                    *eventbus.EventBus
		genesis                     *genesis.PartitionGenesis
		trustBase                   crypto.Verifier
	}

	NodeOption func(c *configuration)
)

func WithContext(context context.Context) NodeOption {
	return func(c *configuration) {
		c.context = context
	}
}

func WithEventBus(eb *eventbus.EventBus) NodeOption {
	return func(c *configuration) {
		c.eventbus = eb
	}
}

func WithTxValidator(txValidator TxValidator) NodeOption {
	return func(c *configuration) {
		c.txValidator = txValidator
	}
}

func WithUnicityCertificateValidator(unicityCertificateValidator UnicityCertificateValidator) NodeOption {
	return func(c *configuration) {
		c.unicityCertificateValidator = unicityCertificateValidator
	}
}

func WithBlockProposalValidator(blockProposalValidator BlockProposalValidator) NodeOption {
	return func(c *configuration) {
		c.blockProposalValidator = blockProposalValidator
	}
}

func WithLeaderSelector(leaderSelector LeaderSelector) NodeOption {
	return func(c *configuration) {
		c.leaderSelector = leaderSelector
	}
}

func WithBlockStore(blockStore store.BlockStore) NodeOption {
	return func(c *configuration) {
		c.blockStore = blockStore
	}
}

func WithT1Timeout(t1Timeout time.Duration) NodeOption {
	return func(c *configuration) {
		c.t1Timeout = t1Timeout
	}
}

func loadAndValidateConfiguration(peer *network.Peer, signer crypto.Signer, genesis *genesis.PartitionGenesis, txs txsystem.TransactionSystem, nodeOptions ...NodeOption) (*configuration, error) {
	if peer == nil {
		return nil, ErrPeerIsNil
	}
	if signer == nil {
		return nil, ErrSignerIsNil
	}
	if genesis == nil {
		return nil, ErrGenesisIsNil
	}
	if txs == nil {
		return nil, ErrTxSystemIsNil
	}
	c := &configuration{
		peer:          peer,
		signer:        signer,
		genesis:       genesis,
		hashAlgorithm: gocrypto.SHA256,
	}
	for _, option := range nodeOptions {
		option(c)
	}
	err := c.initMissingDefaults(peer)
	if err != nil {
		return nil, err
	}
	if err := c.isGenesisValid(txs); err != nil {
		return nil, err
	}

	return c, err
}

// initMissingDefaults loads missing default configuration.
func (c *configuration) initMissingDefaults(peer *network.Peer) error {
	if c.context == nil {
		c.context = context.Background()
	}
	if c.t1Timeout == 0 {
		c.t1Timeout = DefaultT1Timeout
	}
	if c.eventbus == nil {
		c.eventbus = eventbus.New()
	}
	if c.blockStore == nil {
		c.blockStore = store.NewInMemoryBlockStore()
	}
	var err error
	if c.leaderSelector == nil {
		c.leaderSelector, err = NewDefaultLeaderSelector(peer, c.eventbus)
		if err != nil {
			return err
		}
	}
	trustBaseBytes := c.genesis.TrustBase
	c.trustBase, err = crypto.NewVerifierSecp256k1(trustBaseBytes)
	if err != nil {
		return err
	}
	if c.blockProposalValidator == nil {
		c.blockProposalValidator, err = NewDefaultBlockProposalValidator(c.genesis.SystemDescriptionRecord, c.trustBase, c.hashAlgorithm)
		if err != nil {
			return err
		}
	}
	if c.unicityCertificateValidator == nil {
		c.unicityCertificateValidator, err = NewDefaultUnicityCertificateValidator(c.genesis.SystemDescriptionRecord, c.trustBase, c.hashAlgorithm)
		if err != nil {
			return err
		}
	}
	if c.txValidator == nil {
		c.txValidator, err = NewDefaultTxValidator(c.GetSystemIdentifier(), c.blockStore)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *configuration) genesisBlock() *block.Block {
	return &block.Block{
		SystemIdentifier:   c.genesis.SystemDescriptionRecord.SystemIdentifier,
		BlockNumber:        1,
		Transactions:       []*transaction.Transaction{},
		UnicityCertificate: c.genesis.GetCertificate(),
	}
}

func (c *configuration) isGenesisValid(txs txsystem.TransactionSystem) error {
	if err := c.genesis.IsValid(c.trustBase, c.hashAlgorithm); err != nil {
		logger.Warning("Invalid partition genesis file: %v", err)
		return errors.Wrap(err, "invalid root partition genesis file")
	}
	state := txs.State()
	txGenesisRoot := state.Root()
	txSummaryValue := state.Summary()
	genesisCertificate := c.genesis.Certificate
	genesisInputRecord := genesisCertificate.InputRecord
	if !bytes.Equal(genesisInputRecord.Hash, txGenesisRoot) {
		logger.Warning("tx system root hash does not equal to genesis file hash. "+
			"genesis hash: %X, txSystem hash: %X", genesisInputRecord.Hash, txGenesisRoot)
		return ErrInvalidRootHash
	}

	if !bytes.Equal(genesisInputRecord.SummaryValue, txSummaryValue) {
		logger.Warning("tx system summary value does not equal to genesis file summary value. "+
			"Genesis SummaryValue: %X, TxSystem SummaryValue: %X", genesisInputRecord.SummaryValue, txSummaryValue)
		return ErrInvalidSummaryValue
	}
	return nil
}

func (c *configuration) GetSystemIdentifier() []byte {
	return c.genesis.SystemDescriptionRecord.SystemIdentifier
}

func (c *configuration) GetPublicKey(nodeIdentifier string) (crypto.Verifier, error) {
	keys := c.genesis.Keys
	for _, key := range keys {
		if key.NodeIdentifier == nodeIdentifier {
			return crypto.NewVerifierSecp256k1(key.PublicKey)
		}
	}
	return nil, errors.Errorf("public key with node id %v not found", nodeIdentifier)
}
