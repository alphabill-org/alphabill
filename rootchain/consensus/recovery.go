package consensus

import (
	"fmt"
	"sync"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

// how long to wait before repeating status request
const statusReqShelfLife = 800 * time.Millisecond

type recoveryState struct {
	toRound    uint64
	triggerMsg any
	sent       time.Time // when the status request was created/sent
	m          sync.Mutex
}

func (rs *recoveryState) InRecovery() bool {
	rs.m.Lock()
	defer rs.m.Unlock()
	return rs.triggerMsg != nil
}

func (rs *recoveryState) ToRound() uint64 {
	rs.m.Lock()
	defer rs.m.Unlock()
	return rs.toRound
}

func (rs *recoveryState) Clear() any {
	rs.m.Lock()
	defer rs.m.Unlock()

	msg := rs.triggerMsg
	rs.triggerMsg = nil
	return msg
}

func (rs *recoveryState) Set(trigger any) (map[string]hex.Bytes, error) {
	toRound, signatures, err := msgToRecoveryInfo(trigger)
	if err != nil {
		return nil, fmt.Errorf("failed to extract recovery info: %w", err)
	}
	rs.m.Lock()
	defer rs.m.Unlock()

	if rs.triggerMsg != nil && rs.toRound >= toRound && time.Since(rs.sent) < statusReqShelfLife {
		return nil, fmt.Errorf("already in recovery to round %d, ignoring request to recover to round %d", rs.toRound, toRound)
	}

	rs.triggerMsg = trigger
	rs.toRound = toRound
	rs.sent = time.Now()
	return signatures, nil
}

func msgToRecoveryInfo(msg any) (toRound uint64, signatures map[string]hex.Bytes, _ error) {
	switch mt := msg.(type) {
	case *abdrc.ProposalMsg:
		toRound = mt.Block.Qc.GetRound()
		signatures = mt.Block.Qc.Signatures
	case *abdrc.VoteMsg:
		toRound = mt.HighQc.GetRound()
		signatures = mt.HighQc.Signatures
	case *abdrc.TimeoutMsg:
		toRound = mt.Timeout.HighQc.VoteInfo.RoundNumber
		signatures = mt.Timeout.HighQc.Signatures
	case *drctypes.QuorumCert:
		toRound = mt.GetParentRound()
		signatures = mt.Signatures
	default:
		return 0, nil, fmt.Errorf("unknown message type, cannot be used for recovery: %T", mt)
	}

	return toRound, signatures, nil
}

func (rs *recoveryState) String() string {
	rs.m.Lock()
	defer rs.m.Unlock()
	if rs.triggerMsg == nil {
		return "<nil>"
	}
	return fmt.Sprintf("toRound: %d trigger %T @ %s", rs.toRound, rs.triggerMsg, rs.sent)
}
