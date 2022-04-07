package partition

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"
	"github.com/libp2p/go-libp2p-core/peer"
)

// TODO AB-111
type Genesis struct {
	InputRecord              *certificates.InputRecord
	UnicityCertificateRecord *certificates.UnicityCertificate
}

var ErrPeerIDIsEmpty = errors.New("peer ID is empty")

// NewGenesisPartitionNode creates a new genesis.PartitionNode from the given inputs.
func NewGenesisPartitionNode(txSystem TransactionSystem, peerID peer.ID, conf *Configuration) (*genesis.PartitionNode, error) {
	if txSystem == nil {
		return nil, ErrTxSystemIsNil
	}
	if peerID == "" {
		return nil, ErrPeerIDIsEmpty
	}
	if conf == nil {
		return nil, ErrPartitionConfigurationIsNil
	}

	// create the first round of the tx system
	hash, summaryValue := txSystem.RCompl()
	zeroHash := make([]byte, conf.HashAlgorithm.Size())

	// first block
	b := &Block{
		systemIdentifier:    conf.SystemIdentifier,
		txSystemBlockNumber: 1,
		previousBlockHash:   zeroHash,
		transactions:        nil,
	}
	blockHash := b.Hash(conf.HashAlgorithm)

	// P1 request
	p1Request := &p1.P1Request{
		SystemIdentifier: conf.SystemIdentifier,
		NodeIdentifier:   peerID.String(),
		RootRoundNumber:  1,
		InputRecord: &certificates.InputRecord{
			PreviousHash: zeroHash, // extend zero hash
			Hash:         hash,
			BlockHash:    blockHash,
			SummaryValue: summaryValue.Bytes(),
		},
	}
	err := p1Request.Sign(conf.Signer)
	if err != nil {
		return nil, err
	}

	verifier, err := conf.Signer.Verifier()

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
		NodeIdentifier: peerID.String(),
		PublicKey:      pubKey,
		P1Request:      p1Request,
	}
	return node, nil
}

// NewGenesisPartitionRecord validates the given genesis.PartitionNode values, creates a genesis.PartitionRecord
// containing the nodes and given T2 timeout, and verifies the genesis.PartitionRecord.
func NewGenesisPartitionRecord(nodes []*genesis.PartitionNode, t2Timeout uint32) (*genesis.PartitionRecord, error) {
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
