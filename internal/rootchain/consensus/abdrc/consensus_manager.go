package abdrc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/leader"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/storage"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/exp/slices"
)

type (
	recoveryInfo struct {
		toRound    uint64
		triggerMsg any
	}

	RootNet interface {
		Send(msg network.OutputMessage, receivers []peer.ID) error
		Broadcast(msg network.OutputMessage) error
		ReceivedChannel() <-chan network.ReceivedMessage
	}

	// Leader provides interface to different leader selection algorithms
	Leader interface {
		// GetLeaderForRound returns valid leader (node id) for round/view number
		GetLeaderForRound(round uint64) peer.ID
		// currentRound - what PaceMaker considers to be the current round at the time QC is processed.
		Update(qc *abtypes.QuorumCert, currentRound uint64) error
	}

	ConsensusManager struct {
		certReqCh      chan consensus.IRChangeRequest
		certResultCh   chan *types.UnicityCertificate
		params         *consensus.Parameters
		id             peer.ID
		localTimeout   *time.Ticker
		net            RootNet
		pacemaker      *Pacemaker
		leaderSelector Leader
		trustBase      *RootTrustBase
		irReqBuffer    *IrReqBuffer
		safety         *SafetyModule
		blockStore     *storage.BlockStore
		partitions     partitions.PartitionConfiguration
		irReqVerifier  *IRChangeReqVerifier
		t2Timeouts     *PartitionTimeoutGenerator
		waitPropose    bool
		voteBuffer     []*abdrc.VoteMsg
		recovery       *recoveryInfo
	}
)

// NewDistributedAbConsensusManager creates new "Atomic Broadcast" protocol based distributed consensus manager
func NewDistributedAbConsensusManager(nodeID peer.ID, rg *genesis.RootGenesis,
	partitionStore partitions.PartitionConfiguration, net RootNet, signer crypto.Signer, opts ...consensus.Option) (*ConsensusManager, error) {
	// Sanity checks
	if rg == nil {
		return nil, errors.New("cannot start distributed consensus, genesis root record is nil")
	}
	if net == nil {
		return nil, errors.New("network is nil")
	}
	// load options
	optional := consensus.LoadConf(opts)
	// load consensus configuration from genesis
	cParams := consensus.NewConsensusParams(rg.Root)
	// init storage
	bStore, err := storage.NewBlockStore(cParams.HashAlgorithm, rg.Partitions, optional.Storage)
	if err != nil {
		return nil, fmt.Errorf("consensus block storage init failed: %w", err)
	}
	reqVerifier, err := NewIRChangeReqVerifier(cParams, partitionStore, bStore)
	if err != nil {
		return nil, fmt.Errorf("block verifier construct error: %w", err)
	}
	t2TimeoutGen, err := NewLucBasedT2TimeoutGenerator(cParams, partitionStore, bStore)
	if err != nil {
		return nil, fmt.Errorf("failed to create T2 timeout generator: %w", err)
	}
	hqcRound := bStore.GetHighQc().VoteInfo.RoundNumber
	safetyModule, err := NewSafetyModule(nodeID.String(), signer, optional.Storage)
	if err != nil {
		return nil, err
	}

	leader, err := leaderSelector(rg, bStore.Block)
	if err != nil {
		return nil, fmt.Errorf("failed to create consensus leader selector: %w", err)
	}

	tb, err := NewRootTrustBaseFromGenesis(rg.Root)
	if err != nil {
		return nil, fmt.Errorf("consensus root trust base init failed, %w", err)
	}
	consensusManager := &ConsensusManager{
		certReqCh:      make(chan consensus.IRChangeRequest),
		certResultCh:   make(chan *types.UnicityCertificate),
		params:         cParams,
		id:             nodeID,
		net:            net,
		pacemaker:      NewPacemaker(hqcRound, cParams.LocalTimeout, cParams.BlockRate),
		leaderSelector: leader,
		trustBase:      tb,
		irReqBuffer:    NewIrReqBuffer(),
		safety:         safetyModule,
		blockStore:     bStore,
		partitions:     partitionStore,
		irReqVerifier:  reqVerifier,
		t2Timeouts:     t2TimeoutGen,
		waitPropose:    false,
	}
	return consensusManager, nil
}

func leaderSelector(rg *genesis.RootGenesis, blockLoader leader.BlockLoader) (ls Leader, err error) {
	// NB! both leader selector algorithms make the assumption that the rootNodes slice is
	// sorted and it's content doesn't change!
	rootNodes, err := rg.NodeIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get root node IDs: %w", err)
	}
	slices.Sort(rootNodes)

	switch len(rootNodes) {
	case 0:
		return nil, errors.New("number of peers must be greater than zero")
	case 1:
		// really should have "constant leader" algorithm but this shouldn't happen IRL.
		// we need this case as some tests create only one root node. Could use reputation
		// based selection with excludeSize=0 but round-robin is more efficient...
		return leader.NewRoundRobin(rootNodes, 1)
	default:
		// we're limited to window size and exclude size 1 as our block loader (block store) doesn't
		// keep history, ie we can't load blocks older than previous block.
		return leader.NewReputationBased(rootNodes, 1, 1, blockLoader)
	}
}

func (x *ConsensusManager) RequestCertification() chan<- consensus.IRChangeRequest {
	return x.certReqCh
}

func (x *ConsensusManager) CertificationResult() <-chan *types.UnicityCertificate {
	return x.certResultCh
}

func (x *ConsensusManager) GetLatestUnicityCertificate(id types.SystemID) (*types.UnicityCertificate, error) {
	ucs := x.blockStore.GetCertificates()
	luc, f := ucs[p.SystemIdentifier(id)]
	if !f {
		return nil, fmt.Errorf("no certificate found for system id %X", id)
	}
	return luc, nil
}

func (x *ConsensusManager) Run(ctx context.Context) error {
	// Start timers and network processing
	x.localTimeout = time.NewTicker(x.params.LocalTimeout)
	// Am I the leader?
	currentRound := x.pacemaker.GetCurrentRound()
	if x.leaderSelector.GetLeaderForRound(currentRound) == x.id {
		logger.Info("%v round %v root node started as leader, waiting to propose", x.id.ShortString(), currentRound)
		// on start wait a bit before making a proposal
		x.waitPropose = true
		go func() {
			select {
			case <-time.After(x.params.BlockRate):
				x.processNewRoundEvent()
			case <-ctx.Done():
			}
		}()
	} else {
		logger.Info("%v round %v root node started, waiting for proposal from leader %v",
			x.id.ShortString(), currentRound, x.leaderSelector.GetLeaderForRound(currentRound).String())
	}
	return x.loop(ctx)
}

func (x *ConsensusManager) loop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			logger.Info("%v exiting distributed consensus manager main loop", x.id.ShortString())
			return ctx.Err()
		case msg, ok := <-x.net.ReceivedChannel():
			if !ok {
				logger.Warning("%v root network received channel closed, exiting distributed consensus main loop",
					x.id.ShortString())
				return fmt.Errorf("exit drc consensus, network channel closed")
			}
			if msg.Message == nil {
				logger.Warning("%v root network received message is nil", x.id.ShortString())
				continue
			}
			switch mt := msg.Message.(type) {
			case *abdrc.IRChangeReqMsg:
				util.WriteTraceJsonLog(logger, fmt.Sprintf("IR Change Request from %v", msg.From), mt)
				x.onIRChange(mt)
			case *abdrc.ProposalMsg:
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Proposal from %v", msg.From), mt)
				x.onProposalMsg(ctx, mt)
			case *abdrc.VoteMsg:
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Vote from %v", msg.From), mt)
				x.onVoteMsg(ctx, mt)
			case *abdrc.TimeoutMsg:
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Timeout vote from %v", msg.From), mt)
				x.onTimeoutMsg(mt)
			case *abdrc.GetStateMsg:
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Recovery state request from %v", msg.From), mt)
				x.onStateReq(mt)
			case *abdrc.StateMsg:
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Received recovery response from %v", msg.From), mt)
				x.onStateResponse(ctx, mt)
			default:
				logger.Warning("%v unknown protocol req %s %T from %v", x.id.ShortString(), msg.Protocol, mt, msg.From)
			}
		case req, ok := <-x.certReqCh:
			if !ok {
				logger.Warning("%v certification channel closed, exiting distributed consensus main loop", x.id.ShortString())
				return fmt.Errorf("exit drc consensus, certification channel closed")
			}
			x.onPartitionIRChangeReq(&req)
		case <-x.localTimeout.C:
			x.onLocalTimeout()
		}
	}
}

// onLocalTimeout handle local timeout, either no proposal is received or voting does not
// reach consensus. Triggers timeout voting.
func (x *ConsensusManager) onLocalTimeout() {
	// the ticker is automatically restarted, the timeout may need adjustment depending on algorithm and throttling
	logger.Info("%v round %v local timeout", x.id.ShortString(), x.pacemaker.GetCurrentRound())
	// has the validator voted in this round, if true send the same (timeout)vote
	// maybe less than quorum of nodes where operational the last time
	timeoutVoteMsg := x.pacemaker.GetTimeoutVote()
	if timeoutVoteMsg == nil {
		// create timeout vote
		timeoutVoteMsg = abdrc.NewTimeoutMsg(abtypes.NewTimeout(
			x.pacemaker.GetCurrentRound(), 0, x.blockStore.GetHighQc()), x.id.String())
		// sign
		if err := x.safety.SignTimeout(timeoutVoteMsg, x.pacemaker.LastRoundTC()); err != nil {
			logger.Warning("%v local timeout error, %v", x.id.ShortString(), err)
			return
		}
		// Record vote
		x.pacemaker.SetTimeoutVote(timeoutVoteMsg)
	}
	// in the case root chain has not made any progress (less than quorum nodes online), broadcast the same vote again
	// broadcast timeout vote
	logger.Trace("%v round %v broadcasting timeout vote", x.id.ShortString(), x.pacemaker.GetCurrentRound())
	if err := x.net.Broadcast(
		network.OutputMessage{
			Protocol: network.ProtocolRootTimeout,
			Message:  timeoutVoteMsg,
		}); err != nil {
		logger.Warning("%v failed to forward ir change message: %v", x.id.ShortString(), err)
	}
}

// onPartitionIRChangeReq handle partition change requests. Received from go routine handling
// partition communication when either partition reaches consensus or cannot reach consensus.
func (x *ConsensusManager) onPartitionIRChangeReq(req *consensus.IRChangeRequest) {
	logger.Debug("%v round %v, IR change request from partition",
		x.id.String(), x.pacemaker.GetCurrentRound())
	reason := abtypes.Quorum
	if req.Reason == consensus.QuorumNotPossible {
		reason = abtypes.QuorumNotPossible
	}
	irReq := &abdrc.IRChangeReqMsg{
		IRChangeReq: &abtypes.IRChangeReq{
			SystemIdentifier: req.SystemIdentifier,
			CertReason:       reason,
			Requests:         req.Requests},
	}
	// are we the next leader or leader in current round waiting/throttling to send proposal
	nextRound := x.pacemaker.GetCurrentRound() + 1
	nextLeader := x.leaderSelector.GetLeaderForRound(nextRound)
	if nextLeader == x.id || x.waitPropose {
		// store the request in local buffer
		x.onIRChange(irReq)
		return
	}
	// route the IR change to the next root chain leader
	logger.Debug("%v round %v forwarding IR change request to next leader in round %v - %v",
		x.id.ShortString(), x.pacemaker.GetCurrentRound(), nextRound, nextLeader.String())
	err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootIrChangeReq,
			Message:  irReq,
		}, []peer.ID{nextLeader})
	if err != nil {
		logger.Warning("%v failed to forward IR Change request: %v", x.id.ShortString(), err)
	}
}

// onIRChange handles IR change request from other root nodes
func (x *ConsensusManager) onIRChange(irChangeMsg *abdrc.IRChangeReqMsg) {
	if err := irChangeMsg.IsValid(); err != nil {
		logger.Debug("Receive IR change request is invalid, %w", err)
		return
	}
	currentLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound())
	nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
	// todo: if in recovery then forward to next?
	// if the node is leader and has not yet proposed or is the next leader - then buffer the request to include in the next block proposal
	// there is a race between proposal (received in reverse order) and IR change request, due to this the node could also be the leader in round+2
	// However, we can't reliably find +2 leader when using reputation based election!
	if (currentLeader == x.id && x.waitPropose) ||
		nextLeader == x.id ||
		x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound()+2) == x.id {
		logger.Debug("%v round %v IR change request received", x.id.ShortString(), x.pacemaker.GetCurrentRound())
		// Verify and buffer and wait for opportunity to make the next proposal
		if err := x.irReqBuffer.Add(x.pacemaker.GetCurrentRound(), irChangeMsg.IRChangeReq, x.irReqVerifier); err != nil {
			logger.Warning("%v IR change request from partition %X error: %v", x.id.String(), irChangeMsg.GetSystemID(), err)
		}
		return
	}
	// todo: AB-549 add max hop count or some sort of TTL?
	// either this is a completely lost message or because of race we just proposed, try to forward again to the next leader
	if currentLeader == x.id && !x.waitPropose {
		logger.Warning("%v is the leader in round %v, but proposal has been already sent, forward request to next leader %v",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), nextLeader.String())
	} else {
		logger.Warning("%v is not leader in next round %v, IR change req forwarded again %v",
			x.id.ShortString(), x.pacemaker.GetCurrentRound()+1, nextLeader.String())
	}
	if err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootIrChangeReq,
			Message:  irChangeMsg,
		}, []peer.ID{nextLeader}); err != nil {
		logger.Warning("%v failed to forward ir chang message: %v", x.id.ShortString(), err)
	}
}

// onVoteMsg handle votes messages from other root validators
func (x *ConsensusManager) onVoteMsg(ctx context.Context, vote *abdrc.VoteMsg) {
	if vote.VoteInfo.RoundNumber < x.pacemaker.GetCurrentRound() {
		logger.Warning("%v round %v stale vote, validator %v is behind vote for round %v, ignored",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.Author, vote.VoteInfo.RoundNumber)
		return
	}
	// verify signature on vote
	err := vote.Verify(x.trustBase.GetQuorumThreshold(), x.trustBase.GetVerifiers())
	if err != nil {
		logger.Warning("%v vote verify failed: %v", x.id.ShortString(), err)
		return
	}
	logger.Trace("%v round %v received vote for round %v from %v",
		x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.VoteInfo.RoundNumber, vote.Author)
	// if a vote is received for the next round it is intended for the node which is going to be the
	// leader in round current+2. We cache it in the voteBuffer anyway, in hope that this node will
	// be the leader then (reputation based algorithm can't predict leader for the round current+2
	// as ie round-robin can).
	if vote.VoteInfo.RoundNumber == x.pacemaker.GetCurrentRound()+1 {
		// vote received before proposal, buffer
		logger.Debug("%v round %v received vote for round %v before proposal, buffering vote",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.VoteInfo.RoundNumber)
		x.voteBuffer = append(x.voteBuffer, vote)
		// if we have received f+2 votes, but no proposal yet, then try and recover the proposal
		if len(x.voteBuffer) > x.trustBase.GetMaxFaultyNodes()+2 {
			logger.Warning("%v vote, have received %v votes, but no proposal, entering recovery", x.id.ShortString(), len(x.voteBuffer))
			if err = x.sendRecoveryRequests(vote); err != nil {
				logger.Warning("send recovery request failed, %w", err)
			}
		}
		return
	}
	// Normal votes are only sent to the next leader,
	// timeout votes are broadcast to everybody
	nextRound := vote.VoteInfo.RoundNumber + 1
	// verify that the validator is correct leader in next round
	if x.leaderSelector.GetLeaderForRound(nextRound) != x.id {
		// this might also be a stale vote, since when we have quorum the round is advanced and the node becomes
		// the new leader in the current view/round
		logger.Warning("%v received vote, validator is not leader in next round %v, vote ignored",
			x.id.ShortString(), nextRound)
		return
	}
	// SyncState, compare last handled QC
	if x.checkRecoveryNeeded(vote.HighQc) {
		// this will only happen if the node somehow gets a different state hash
		// should be quite unlikely, during normal operation
		logger.Info("%v vote, recovery needed", x.id.ShortString())
		if err = x.sendRecoveryRequests(vote); err != nil {
			logger.Warning("%v recovery requests send failed, %w", x.id.ShortString(), err)
		}
		return
	}
	// Store vote, check for QC
	qc, err := x.pacemaker.RegisterVote(vote, x.trustBase)
	if err != nil {
		logger.Warning("%v round %v vote from error, %w",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.Author, err)
		return
	}
	if qc == nil {
		logger.Trace("%v round %v processed vote for round %v, no quorum yet",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.VoteInfo.RoundNumber)
		return
	}
	logger.Debug("%v round %v quorum achieved", x.id.ShortString(), vote.VoteInfo.RoundNumber)
	// since the root chain must not run faster than block-rate, calculate
	// time from last proposal and see if we need to wait
	slowDownTime := x.pacemaker.CalcTimeTilNextProposal()
	// advance view/round on QC
	x.processQC(qc)
	if slowDownTime > 0 {
		logger.Debug("%v round %v node %v wait %v before proposing",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), x.id.String(), slowDownTime)
		x.waitPropose = true
		go func() {
			select {
			case <-time.After(slowDownTime):
				x.processNewRoundEvent()
			case <-ctx.Done():
			}
		}()
		return
	}
	// trigger new round immediately
	x.processNewRoundEvent()
}

// onTimeoutMsg handles timeout vote messages from other root validators
// Timeout votes are broadcast to all nodes on local timeout and all validators try to assemble
// timeout certificate independently.
func (x *ConsensusManager) onTimeoutMsg(vote *abdrc.TimeoutMsg) {
	if vote.Timeout.Round < x.pacemaker.GetCurrentRound() {
		logger.Trace("%v round %v stale timeout vote, validator %v voted for timeout in round %v, ignored",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.Author, vote.Timeout.Round)
		return
	}
	// verify signature on vote
	err := vote.Verify(x.trustBase.GetQuorumThreshold(), x.trustBase.GetVerifiers())
	if err != nil {
		logger.Warning("%v timeout vote verify failed: %v", x.id.ShortString(), err)
	}
	logger.Trace("%v round %v received timeout vote for round %v from %v",
		x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.Timeout.Round, vote.Author)
	// SyncState, compare last handled QC
	if x.checkRecoveryNeeded(vote.Timeout.HighQc) {
		logger.Info("%v timeout vote, recovery needed", x.id.ShortString())
		if err = x.sendRecoveryRequests(vote); err != nil {
			logger.Warning("%v recovery requests send failed, %w", x.id.ShortString(), err)
		}
		return
	}
	tc, err := x.pacemaker.RegisterTimeoutVote(vote, x.trustBase)
	if err != nil {
		logger.Warning("%v round %v vote message from %v error: %w",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.Author, err)
		return
	}
	if tc == nil {
		logger.Trace("%v round %v processed timeout vote for round %v, no quorum yet",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.Timeout.Round)
		return
	}
	logger.Debug("%v round %v timeout quorum achieved", x.id.ShortString(), vote.Timeout.Round)
	// process timeout certificate to advance to next the view/round
	x.processTC(tc)
	// if this node is the leader in this round then issue a proposal
	l := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound())
	if l == x.id {
		x.processNewRoundEvent()
	} else {
		logger.Trace("%v round %v, new leader is %v, waiting for proposal",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), l.String())
	}
}

// checkRecoveryNeeded verify current state against received state and determine if the validator needs to
// recover or not. Basically either the state is different or validator is behind (has skipped some views/rounds)
func (x *ConsensusManager) checkRecoveryNeeded(qc *abtypes.QuorumCert) bool {
	// Get block and check if we have the same state
	rootHash, err := x.blockStore.GetBlockRootHash(qc.VoteInfo.RoundNumber)
	if err != nil {
		logger.Warning("%v round %v unknown block in round %v, recover",
			x.id.ShortString(), qc.VoteInfo.RoundNumber)
		return true
	}
	if !bytes.Equal(qc.VoteInfo.CurrentRootHash, rootHash) {
		logger.Warning("%v round %v state is different expected %X, local %X, recover state",
			x.id.ShortString(), qc.VoteInfo.RoundNumber, qc.VoteInfo.CurrentRootHash, rootHash)
		return true
	}
	return false
}

// onProposalMsg handles block proposal messages from other validators.
// Only a proposal made by the leader of this view/round shall be accepted and processed
func (x *ConsensusManager) onProposalMsg(ctx context.Context, proposal *abdrc.ProposalMsg) {
	if proposal.Block.Round < x.pacemaker.GetCurrentRound() {
		logger.Debug("%v round %v stale proposal, validator %v is behind and proposed round %v, ignored",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), proposal.Block.Author, proposal.Block.Round)
		return
	}
	// verify signature on proposal (does not verify partition request signatures)
	err := proposal.Verify(x.trustBase.GetQuorumThreshold(), x.trustBase.GetVerifiers())
	if err != nil {
		logger.Warning("%v invalid Proposal message, verify failed: %v", x.id.ShortString(), err)
		return
	}
	logger.Trace("%v round %v received proposal message from %v",
		x.id.ShortString(), x.pacemaker.GetCurrentRound(), proposal.Block.Author)
	// Is from valid leader
	if x.leaderSelector.GetLeaderForRound(proposal.Block.Round).String() != proposal.Block.Author {
		logger.Warning("%v proposal author %v is not a valid leader for round %v, ignoring proposal",
			x.id.ShortString(), proposal.Block.Author, proposal.Block.Round)
		return
	}
	// Check current state against new QC
	if x.checkRecoveryNeeded(proposal.Block.Qc) {
		logger.Info("%v node starting recovery", x.id.ShortString())
		if err = x.sendRecoveryRequests(proposal); err != nil {
			logger.Warning("%v recovery requests send failed, %w", x.id.ShortString(), err)
		}
		return
	}
	// Every proposal must carry a QC or TC for previous round
	// Process QC first, update round
	x.processQC(proposal.Block.Qc)
	x.processTC(proposal.LastRoundTc)
	// execute proposed payload
	execStateId, err := x.blockStore.Add(proposal.Block, x.irReqVerifier)
	if err != nil {
		// wait for timeout, if others make progress this node will need to recover
		logger.Warning("%v failed to execute proposal: %v", x.id.ShortString(), err.Error())
		// cannot send vote, so just return and wait for local timeout or new proposal (and try to recover then)
		return
	}
	// make vote
	voteMsg, err := x.safety.MakeVote(proposal.Block, execStateId, x.blockStore.GetHighQc(), x.pacemaker.LastRoundTC())
	if err != nil {
		// wait for timeout, if others make progress this node will need to recover
		logger.Warning("%v failed to sign vote, vote not sent: %v", x.id.ShortString(), err.Error())
		return
	}

	x.pacemaker.SetVoted(voteMsg)
	// send vote to the next leader
	nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
	logger.Trace("%v sending vote to next leader %v", x.id.ShortString(), nextLeader.String())
	err = x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootVote,
			Message:  voteMsg,
		}, []peer.ID{nextLeader})
	if err != nil {
		logger.Warning("%v failed to send vote message: %v", x.id.ShortString(), err)
	}
	// process vote buffer
	// optimize? only call onVoteMsg when this node will be leader, otherwise just reset the buffer?
	if len(x.voteBuffer) > 0 {
		logger.Debug("Handling %v buffered vote messages", len(x.voteBuffer))
		for _, v := range x.voteBuffer {
			x.onVoteMsg(ctx, v)
		}
		x.voteBuffer = nil
	}
}

// processQC - handles quorum certificate
func (x *ConsensusManager) processQC(qc *abtypes.QuorumCert) {
	if qc == nil {
		return
	}
	certs, err := x.blockStore.ProcessQc(qc)
	if err != nil {
		// todo: recovery
		logger.Warning("%v round %v failed to process QC (aborting but should try to recover): %v", x.id.ShortString(), x.pacemaker.GetCurrentRound(), err)
		return
	}
	for _, uc := range certs {
		x.certResultCh <- uc
	}

	x.pacemaker.AdvanceRoundQC(qc)
	// progress is made, restart timeout (move to pacemaker)
	x.localTimeout.Reset(x.pacemaker.GetRoundTimeout())

	// in the "DiemBFT v4" pseudo-code the process_certificate_qc first calls
	// leaderSelector.Update and after that pacemaker.AdvanceRound - we do it the
	// other way around as otherwise current leader goes out of sync with peers...
	if err := x.leaderSelector.Update(qc, x.pacemaker.GetCurrentRound()); err != nil {
		logger.Error("failed to update leader selector: %v", err)
	}
}

// processTC - handles timeout certificate
func (x *ConsensusManager) processTC(tc *abtypes.TimeoutCert) {
	if tc == nil {
		return
	}
	if err := x.blockStore.ProcessTc(tc); err != nil {
		logger.Warning("%v failed to handle timeout certificate: %v", x.id.ShortString(), err)
	}
	x.pacemaker.AdvanceRoundTC(tc)
}

// processNewRoundEvent handled new view event, is called when either QC or TC is reached locally and
// triggers a new round. If this node is the leader in the new view/round, then make a proposal otherwise
// wait for a proposal from a leader in this round/view
func (x *ConsensusManager) processNewRoundEvent() {
	// to counteract throttling (find a better solution)
	x.waitPropose = false
	x.localTimeout.Reset(x.pacemaker.GetRoundTimeout())
	round := x.pacemaker.GetCurrentRound()
	if x.leaderSelector.GetLeaderForRound(round) != x.id {
		logger.Info("%v round %v new round start, not leader awaiting proposal", x.id.ShortString(), round)
		return
	}
	logger.Info("%v round %v new round start, node is leader", x.id.ShortString(), round)
	// find partitions with T2 timeouts
	timeoutIds, err := x.t2Timeouts.GetT2Timeouts(round)
	// NB! error here is not fatal, still make a proposal, hopefully the next node will generate timeout
	// requests for partitions this node failed to query
	if err != nil {
		logger.Warning("failed to check timeouts for some partitions, %v", err)
	}
	proposalMsg := &abdrc.ProposalMsg{
		Block: &abtypes.BlockData{
			Author:    x.id.String(),
			Round:     round,
			Epoch:     0,
			Timestamp: util.MakeTimestamp(),
			Payload:   x.irReqBuffer.GeneratePayload(round, timeoutIds),
			Qc:        x.blockStore.GetHighQc(),
		},
		LastRoundTc: x.pacemaker.LastRoundTC(),
	}
	// safety makes simple sanity checks and signs if everything is ok
	if err := x.safety.SignProposal(proposalMsg); err != nil {
		logger.Warning("%v failed to send proposal message, message signing failed: %v", x.id.ShortString(), err)
	}
	// broadcast proposal message (also to self)
	logger.Trace("%v broadcasting proposal msg", x.id.ShortString())
	if err := x.net.Broadcast(
		network.OutputMessage{
			Protocol: network.ProtocolRootProposal,
			Message:  proposalMsg}); err != nil {
		logger.Warning("%v failed to send proposal message, network error: %v", x.id.ShortString(), err)
	}
}

func (x *ConsensusManager) onStateReq(req *abdrc.GetStateMsg) {
	if x.recovery != nil {
		return
	}

	logger.Trace("%v round %v received state request from %v",
		x.id.ShortString(), x.pacemaker.GetCurrentRound(), req.NodeId)
	certs := x.blockStore.GetCertificates()
	ucs := make([]*types.UnicityCertificate, 0, len(certs))
	for _, c := range certs {
		ucs = append(ucs, c)
	}
	committedBlock := x.blockStore.GetRoot()
	pendingBlocks := x.blockStore.GetPendingBlocks()
	pending := make([]*abdrc.RecoveryBlock, len(pendingBlocks))
	for i, b := range pendingBlocks {
		pending[i] = &abdrc.RecoveryBlock{
			Block:    b.BlockData,
			Ir:       storage.ToRecoveryInputData(b.CurrentIR),
			Qc:       b.Qc,
			CommitQc: nil,
		}
	}
	respMsg := &abdrc.StateMsg{
		Certificates: ucs,
		CommittedHead: &abdrc.RecoveryBlock{
			Block:    committedBlock.BlockData,
			Ir:       storage.ToRecoveryInputData(committedBlock.CurrentIR),
			Qc:       committedBlock.Qc,
			CommitQc: committedBlock.CommitQc,
		},
		BlockNode: pending,
	}
	peerID, err := peer.Decode(req.NodeId)
	if err != nil {
		logger.Warning("%v invalid node identifier: '%s'", req.NodeId)
		return
	}
	if err = x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootStateResp,
			Message:  respMsg}, []peer.ID{peerID}); err != nil {
		logger.Warning("%v failed to send state response message, network error: %v", x.id.ShortString(), err)
	}
}

func (x *ConsensusManager) onStateResponse(ctx context.Context, req *abdrc.StateMsg) {
	logger.Trace("%v round %v received state response",
		x.id.String(), x.pacemaker.GetCurrentRound())
	if x.recovery == nil {
		return
	}

	// check validity
	ucs := req.Certificates
	for _, c := range ucs {
		if err := c.IsValid(x.trustBase.GetVerifiers(), x.params.HashAlgorithm, c.UnicityTreeCertificate.SystemIdentifier, c.UnicityTreeCertificate.SystemDescriptionHash); err != nil {
			logger.Warning("received invalid certificate, discarding whole message")
			return
		}
	}
	x.blockStore.UpdateCertificates(ucs)
	if err := x.blockStore.RecoverState(req.CommittedHead, req.BlockNode, x.irReqVerifier); err != nil {
		logger.Warning("state response failed, %v", err)
		return
	}

	round := x.blockStore.GetHighQc().GetRound()
	x.pacemaker = NewPacemaker(round, x.params.LocalTimeout, x.params.BlockRate)

	for ; ; round++ {
		block, err := x.blockStore.Block(round)
		if err != nil {
			logger.Error("restoring leader selector state, failed to load block for round %d: %v", round, err)
			// we could get error because blocks are not consecutive or we have reached to the last block we
			// have (ie got "block not found" error) so do not return but break ie carry on and act as if
			// restore was a success. If we managed to restore pacemaker and blockStore state we might be able
			// to fully recover by just processing proposals and votes from now on? If not then recovery will
			// be triggered again...
			break
		}
		if err := x.leaderSelector.Update(block.Qc, block.Round); err != nil {
			// failing to elect leader is not critical error here, carry on until we're on the last block in storage
			logger.Warning("restoring leader selector state, failed to update leader with block of round %d: %v", round, err)
		}
	}
	// exit recovery status and replay buffered messages
	triggerMsg := x.recovery.triggerMsg
	x.recovery = nil

	switch mt := triggerMsg.(type) {
	case *abdrc.ProposalMsg:
		x.onProposalMsg(ctx, mt)
	case *abdrc.VoteMsg:
		x.onVoteMsg(ctx, mt)
	}

	for _, v := range x.voteBuffer {
		x.onVoteMsg(ctx, v)
	}
	x.voteBuffer = nil
}

func (x *ConsensusManager) sendRecoveryRequests(triggerMsg any) error {
	info, author, signatures, err := msgToRecoveryInfo(triggerMsg)
	if err != nil {
		return fmt.Errorf("failed to extract recovery info: %w", err)
	}
	if x.recovery != nil && x.recovery.toRound >= info.toRound {
		return fmt.Errorf("already in recovery to round %d, ignoring request to recover to round %d", x.recovery.toRound, info.toRound)
	}

	authorID, err := peer.Decode(author)
	if err != nil {
		return fmt.Errorf("failed to decode message author as peer ID: %w", err)
	}
	nodes := addRandomNodeIdFromSignatureMap([]peer.ID{authorID}, signatures)

	// set recovery status - when send fails we won't check did we fail to send to just one peer or to
	// all of them... if we completely failed to send we'll (re)enter to recovery status with higher
	// destination round when receiving proposal or vote for the next round...
	x.recovery = info

	if err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootStateReq,
			Message:  &abdrc.GetStateMsg{NodeId: x.id.String()}}, nodes); err != nil {
		return fmt.Errorf("failed to send recovery request: %w", err)
	}
	return nil
}

func msgToRecoveryInfo(msg any) (info *recoveryInfo, author string, signatures map[string][]byte, err error) {
	info = &recoveryInfo{triggerMsg: msg}

	switch mt := msg.(type) {
	case *abdrc.ProposalMsg:
		info.toRound = mt.Block.Round
		author = mt.Block.Author
		signatures = mt.Block.Qc.Signatures
	case *abdrc.VoteMsg:
		info.toRound = mt.VoteInfo.RoundNumber
		author = mt.Author
		signatures = mt.HighQc.Signatures
	case *abdrc.TimeoutMsg:
		info.toRound = mt.Timeout.HighQc.VoteInfo.RoundNumber
		author = mt.Author
		signatures = mt.Timeout.HighQc.Signatures
	default:
		return nil, "", nil, fmt.Errorf("unknown message type, cannot be used for recovery: %T", mt)
	}

	return info, author, signatures, nil
}

func addRandomNodeIdFromSignatureMap(nodes []peer.ID, m map[string][]byte) []peer.ID {
	for k := range m {
		id, err := peer.Decode(k)
		if err != nil {
			continue
		}
		if slices.Contains(nodes, id) {
			continue
		}
		return append(nodes, id)
	}
	return nodes
}
