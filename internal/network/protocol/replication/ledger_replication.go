package replication

import (
	"errors"
	"fmt"
)

var (
	ErrLedgerReplicationRespIsNil = errors.New("ledger replication response is nil")
	ErrLedgerResponseBlocksIsNil  = errors.New("ledger response blocks is nil")
	ErrLedgerReplicationReqIsNil  = errors.New("ledger replication requests is nil")
	ErrInvalidSystemIdentifier    = errors.New("invalid system identifier")
	ErrNodeIdentifierIsMissing    = errors.New("node identifier is missing")
)

func (r *LedgerReplicationResponse) Pretty() string {
	count := len(r.Blocks)
	// error message or no blocks
	if r.Message != "" {
		return fmt.Sprintf("status: %s, message: %s, %v blocks", r.Status.String(), r.Message, count)
	}
	return fmt.Sprintf("status: %s, %v blocks", r.Status.String(), count)
}

func (r *LedgerReplicationResponse) IsValid() error {
	if r == nil {
		return ErrLedgerReplicationRespIsNil
	}
	if r.Blocks == nil {
		return ErrLedgerResponseBlocksIsNil
	}
	return nil
}

func (r *LedgerReplicationRequest) IsValid() error {
	if r == nil {
		return ErrLedgerReplicationReqIsNil
	}
	if len(r.SystemIdentifier) != 4 {
		return ErrInvalidSystemIdentifier
	}
	if r.NodeIdentifier == "" {
		return ErrNodeIdentifierIsMissing
	}
	if r.EndBlockNumber != 0 && r.EndBlockNumber < r.BeginBlockNumber {
		return fmt.Errorf("invalid block request range from %v to %v", r.BeginBlockNumber, r.EndBlockNumber)
	}
	return nil
}
