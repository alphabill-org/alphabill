package partition

import (
	"bytes"
	gocrypto "crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/blockproposal"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
)

var (
	ErrSystemIdentifierIsNil = errors.New("System identifier is nil")
	ErrBlockStoreIsNil       = errors.New("block store is nil")
	ErrTransactionExpired    = errors.New("transaction timeout must be greater than current block height")
)

type (

	// TxValidator is used to validate generic transactions (e.g. timeouts, system identifiers, etc.). This validator
	// should not contain transaction system specific validation logic.
	TxValidator interface {
		Validate(tx txsystem.GenericTransaction) error
	}

	// UnicityCertificateValidator is used to validate certificates.UnicityCertificate.
	UnicityCertificateValidator interface {

		// Validate validates the given certificates.UnicityCertificate. Returns an error if given unicity certificate
		// is not valid.
		Validate(uc *certificates.UnicityCertificate) error
	}

	// BlockProposalValidator is used to validate block proposals.
	BlockProposalValidator interface {

		// Validate validates the given blockproposal.BlockProposal. Returns an error if given block proposal
		// is not valid.
		Validate(bp *blockproposal.BlockProposal, nodeSignatureVerifier crypto.Verifier) error
	}

	// DefaultUnicityCertificateValidator is a default implementation of UnicityCertificateValidator.
	DefaultUnicityCertificateValidator struct {
		systemIdentifier      []byte
		systemDescriptionHash []byte
		trustBase             crypto.Verifier
		algorithm             gocrypto.Hash
	}

	// DefaultBlockProposalValidator is a default implementation of UnicityCertificateValidator.
	DefaultBlockProposalValidator struct {
		systemIdentifier      []byte
		systemDescriptionHash []byte
		trustBase             crypto.Verifier
		algorithm             gocrypto.Hash
	}

	DefaultTxValidator struct {
		systemIdentifier []byte
		blockStore       store.BlockStore
	}
)

// NewDefaultTxValidator creates a new instance of default	TxValidator.
func NewDefaultTxValidator(systemIdentifier []byte, blockStore store.BlockStore) (TxValidator, error) {
	if systemIdentifier == nil {
		return nil, ErrSystemIdentifierIsNil
	}
	if blockStore == nil {
		return nil, ErrBlockStoreIsNil
	}
	return &DefaultTxValidator{
		systemIdentifier: systemIdentifier,
		blockStore:       blockStore,
	}, nil
}

func (dtv *DefaultTxValidator) Validate(tx txsystem.GenericTransaction) error {
	if !bytes.Equal(dtv.systemIdentifier, tx.SystemID()) {
		//  transaction was not sent to correct transaction system
		return errors.Wrapf(ErrInvalidSystemIdentifier, "expected %X, got %X", dtv.systemIdentifier, tx.SystemID())
	}

	block := dtv.blockStore.LatestBlock()
	if tx.Timeout() <= block.BlockNumber {
		// transaction is expired
		return errors.Wrapf(ErrTransactionExpired, "timeout %v; blockNumber: %v", tx.Timeout(), block.BlockNumber)
	}
	return nil
}

// NewDefaultUnicityCertificateValidator creates a new instance of default UnicityCertificateValidator.
func NewDefaultUnicityCertificateValidator(
	systemDescription *genesis.SystemDescriptionRecord,
	trustBase crypto.Verifier,
	algorithm gocrypto.Hash,
) (UnicityCertificateValidator, error) {
	if err := systemDescription.IsValid(); err != nil {
		return nil, err
	}
	if trustBase == nil {
		return nil, certificates.ErrVerifierIsNil
	}
	h := systemDescription.Hash(algorithm)
	return &DefaultUnicityCertificateValidator{
		systemIdentifier:      systemDescription.SystemIdentifier,
		trustBase:             trustBase,
		systemDescriptionHash: h,
		algorithm:             algorithm,
	}, nil
}

func (ucv *DefaultUnicityCertificateValidator) Validate(uc *certificates.UnicityCertificate) error {
	return uc.IsValid(ucv.trustBase, ucv.algorithm, ucv.systemIdentifier, ucv.systemDescriptionHash)
}

// NewDefaultBlockProposalValidator creates a new instance of default BlockProposalValidator.
func NewDefaultBlockProposalValidator(
	systemDescription *genesis.SystemDescriptionRecord,
	trustBase crypto.Verifier,
	algorithm gocrypto.Hash,
) (BlockProposalValidator, error) {
	if err := systemDescription.IsValid(); err != nil {
		return nil, err
	}
	if trustBase == nil {
		return nil, certificates.ErrVerifierIsNil
	}
	h := systemDescription.Hash(algorithm)
	return &DefaultBlockProposalValidator{
		systemIdentifier:      systemDescription.SystemIdentifier,
		trustBase:             trustBase,
		systemDescriptionHash: h,
		algorithm:             algorithm,
	}, nil
}

func (bpv *DefaultBlockProposalValidator) Validate(bp *blockproposal.BlockProposal, nodeSignatureVerifier crypto.Verifier) error {
	return bp.IsValid(
		nodeSignatureVerifier,
		bpv.trustBase,
		bpv.algorithm,
		bpv.systemIdentifier,
		bpv.systemDescriptionHash,
	)
}
