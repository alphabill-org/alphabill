package abdrc

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/exp/slices"

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
		pacemaker:      NewPacemaker(cParams.BlockRate/2, cParams.LocalTimeout),
		leaderSelector: leader,
		trustBase:      tb,
		irReqBuffer:    NewIrReqBuffer(),
		safety:         safetyModule,
		blockStore:     bStore,
		partitions:     partitionStore,
		irReqVerifier:  reqVerifier,
		t2Timeouts:     t2TimeoutGen,
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
	defer x.pacemaker.Stop()
	x.pacemaker.Reset(x.blockStore.GetHighQc().VoteInfo.RoundNumber)
	currentRound := x.pacemaker.GetCurrentRound()
	logger.Info("%s round %d root node starting, leader is %s", x.id.ShortString(), currentRound, x.leaderSelector.GetLeaderForRound(currentRound))
	err := x.loop(ctx)
	logger.Info("%v exited distributed consensus manager main loop: %v", x.id.ShortString(), err)
	return err
}

func (x *ConsensusManager) loop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-x.net.ReceivedChannel():
			if !ok {
				return fmt.Errorf("root network received channel has been closed")
			}
			util.WriteTraceJsonLog(logger, fmt.Sprintf("%s received %T from %v", x.id.ShortString(), msg.Message, msg.From), msg.Message)
			switch mt := msg.Message.(type) {
			case *abtypes.IRChangeReq:
				if err := x.onIRChange(mt); err != nil {
					logger.Warning("%v failed to process IR change message: %v", x.id.ShortString(), err)
				}
			case *abdrc.ProposalMsg:
				x.onProposalMsg(ctx, mt)
			case *abdrc.VoteMsg:
				x.onVoteMsg(ctx, mt)
			case *abdrc.TimeoutMsg:
				x.onTimeoutMsg(mt)
			case *abdrc.GetStateMsg:
				x.onStateReq(mt)
			case *abdrc.StateMsg:
				x.onStateResponse(ctx, mt)
			default:
				logger.Warning("%v unknown protocol req %s %T from %v", x.id.ShortString(), msg.Protocol, mt, msg.From)
			}
		case req := <-x.certReqCh:
			if err := x.onPartitionIRChangeReq(&req); err != nil {
				logger.Warning("%v failed to process IR change request from partition: %v", x.id.ShortString(), err)
			}
		case event := <-x.pacemaker.StatusEvents():
			switch event {
			case pmsRoundMatured:
				currentRound := x.pacemaker.GetCurrentRound()
				leader := x.leaderSelector.GetLeaderForRound(currentRound + 1)
				logger.Debug("%v round %d has lasted minimum required duration; next leader %s", x.id.ShortString(), currentRound, leader.ShortString())
				// round 2 is system bootstrap and is a special case - as there is no proposal no-one is sending
				// votes and thus leader won't see quorum and make next proposal (and round would time out).
				// So the round 2 leader has to trigger next round when it's mature without having QC.
				if leader == x.id || (currentRound == 2 && x.id == x.leaderSelector.GetLeaderForRound(2)) {
					if qc := x.pacemaker.RoundQC(); qc != nil || currentRound == 2 {
						x.processQC(qc)
						x.processNewRoundEvent()
					}
				}
			case pmsRoundTimeout:
				x.onLocalTimeout()
			}
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
func (x *ConsensusManager) onPartitionIRChangeReq(req *consensus.IRChangeRequest) error {
	logger.Debug("%v round %v, IR change request from partition", x.id.String(), x.pacemaker.GetCurrentRound())
	irReq := &abtypes.IRChangeReq{
		SystemIdentifier: req.SystemIdentifier,
		Requests:         req.Requests,
	}
	switch req.Reason {
	case consensus.Quorum:
		irReq.CertReason = abtypes.Quorum
	case consensus.QuorumNotPossible:
		irReq.CertReason = abtypes.QuorumNotPossible
	default:
		return fmt.Errorf("unexpected IR change request reason: %v", req.Reason)
	}

	return x.onIRChange(irReq)
}

// onIRChange handles IR change request
func (x *ConsensusManager) onIRChange(irChangeMsg *abtypes.IRChangeReq) error {
	if err := irChangeMsg.IsValid(); err != nil {
		return fmt.Errorf("invalid IR change request: %w", err)
	}
	currentLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound())
	nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
	// todo: if in recovery then forward to next?
	// if the node is leader or will be the next leader then buffer the request to be included in the block proposal
	if currentLeader == x.id || nextLeader == x.id {
		logger.Debug("%v round %v IR change request received", x.id.ShortString(), x.pacemaker.GetCurrentRound())
		if err := x.irReqBuffer.Add(x.pacemaker.GetCurrentRound(), irChangeMsg, x.irReqVerifier); err != nil {
			return fmt.Errorf("failed to add IR change request into buffer: %w", err)
		}
		return nil
	}
	// todo: AB-549 add max hop count or some sort of TTL?
	// either this is a completely lost message or because of race we just proposed, try to forward again to the next leader
	logger.Warning("%v is not leader in next round %v, IR change req forwarded again %v",
		x.id.ShortString(), x.pacemaker.GetCurrentRound()+1, nextLeader.String())
	if err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootIrChangeReq,
			Message:  irChangeMsg,
		}, []peer.ID{nextLeader}); err != nil {
		return fmt.Errorf("failed to forward IR change message to the next leader: %w", err)
	}
	return nil
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
		// NB! we should check that the same peer doesn't send multiple votes, otherwise it is possible
		// to trick instance into recovery mode?
		logger.Debug("%v round %v received vote for round %v before proposal, buffering vote",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.VoteInfo.RoundNumber)
		x.voteBuffer = append(x.voteBuffer, vote)
		// if we have received f+2 votes, but no proposal yet, then try and recover the proposal
		if len(x.voteBuffer) > x.trustBase.GetMaxFaultyNodes()+2 {
			logger.Warning("%v vote, have received %v votes, but no proposal, entering recovery", x.id.ShortString(), len(x.voteBuffer))
			if err = x.sendRecoveryRequests(vote); err != nil {
				logger.Warning("send recovery request failed: %v", err)
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
	if err := x.checkRecoveryNeeded(vote.HighQc); err != nil {
		// this will only happen if the node somehow gets a different state hash
		// should be quite unlikely, during normal operation
		logger.Warning("%v vote triggers recovery: %v", x.id.ShortString(), err)
		if err = x.sendRecoveryRequests(vote); err != nil {
			logger.Warning("%v sending recovery requests failed: %v", x.id.ShortString(), err)
		}
		return
	}

	qc, mature, err := x.pacemaker.RegisterVote(vote, x.trustBase)
	if err != nil {
		logger.Warning("%v round %v failed to register vote from %v: %v",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.Author, err)
		return
	}
	logger.Trace("%v round %v processed vote for round %v, quorum: %t, mature: %t",
		x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.VoteInfo.RoundNumber, qc != nil, mature)
	if qc != nil && mature {
		x.processQC(qc)
		x.processNewRoundEvent()
	}
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
	if err := x.checkRecoveryNeeded(vote.Timeout.HighQc); err != nil {
		logger.Warning("%v timeout vote triggers recovery: %v", x.id.ShortString(), err)
		if err = x.sendRecoveryRequests(vote); err != nil {
			logger.Warning("%v sending recovery requests failed: %v", x.id.ShortString(), err)
		}
		return
	}
	tc, err := x.pacemaker.RegisterTimeoutVote(vote, x.trustBase)
	if err != nil {
		logger.Warning("%v round %v vote message from %v error: %v",
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

/*
checkRecoveryNeeded verify current state against received state and determine if the validator needs to
recover or not. Basically either the state is different or validator is behind (has skipped some views/rounds).
Returns nil when no recovery is needed and error describing the reason to trigger the recovery otherwise.
*/
func (x *ConsensusManager) checkRecoveryNeeded(qc *abtypes.QuorumCert) error {
	rootHash, err := x.blockStore.GetBlockRootHash(qc.VoteInfo.RoundNumber)
	if err != nil {
		return fmt.Errorf("failed to read root hash for round %d from local block store: %w", qc.VoteInfo.RoundNumber, err)
	}
	if !bytes.Equal(qc.VoteInfo.CurrentRootHash, rootHash) {
		return fmt.Errorf("unexpected round %d state - expected %X, local %X", qc.VoteInfo.RoundNumber, qc.VoteInfo.CurrentRootHash, rootHash)
	}
	return nil
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
	if err := x.checkRecoveryNeeded(proposal.Block.Qc); err != nil {
		logger.Warning("%v proposal triggers recovery: %v", x.id.ShortString(), err)
		if err = x.sendRecoveryRequests(proposal); err != nil {
			logger.Warning("%v sending recovery requests failed: %v", x.id.ShortString(), err)
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

	if !x.pacemaker.AdvanceRoundQC(qc) {
		return
	}

	// in the "DiemBFT v4" pseudo-code the process_certificate_qc first calls
	// leaderSelector.Update and after that pacemaker.AdvanceRound - we do it the
	// other way around as otherwise current leader goes out of sync with peers...
	if err := x.leaderSelector.Update(qc, x.pacemaker.GetCurrentRound()); err != nil {
		logger.Error("%v failed to update leader selector: %v", x.id.ShortString(), err)
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
	logger.Trace("%v round %v received state response", x.id.String(), x.pacemaker.GetCurrentRound())
	if x.recovery == nil || req.CommittedHead == nil || req.CommittedHead.CommitQc == nil || x.recovery.toRound > req.CommittedHead.CommitQc.GetRound() {
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

	// exit recovery status and replay buffered messages
	triggerMsg := x.recovery.triggerMsg
	x.recovery = nil

	switch mt := triggerMsg.(type) {
	case *abdrc.ProposalMsg:
		x.onProposalMsg(ctx, mt)
	case *abdrc.VoteMsg:
		x.onVoteMsg(ctx, mt)
	case *abdrc.TimeoutMsg:
		x.pacemaker.Reset(x.blockStore.GetHighQc().GetRound())
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
		info.toRound = mt.Block.Qc.GetRound()
		author = mt.Block.Author
		signatures = mt.Block.Qc.Signatures
	case *abdrc.VoteMsg:
		info.toRound = mt.HighQc.GetRound()
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

func (ri *recoveryInfo) String() string {
	if ri == nil {
		return "<nil>"
	}
	return fmt.Sprintf("toRound: %d trigger %T", ri.toRound, ri.triggerMsg)
}
