package consensus

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	abtypes "github.com/alphabill-org/alphabill-go-base/types"
	testobservability "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/rootchain/consensus/leader"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
)

func Test_ConsensusManager_sendRecoveryRequests(t *testing.T) {
	t.Parallel()

	// the sendRecoveryRequests method depends only on "id", "net", "tracer" and "recovery" fields
	// so we can use "shortcut" when creating the ConsensusManager for test (and init
	// only required fields)

	// NOP tracer can be shared between tests (most fail within method so no point tracing?)
	observe := testobservability.NOPObservability()
	tracer := observe.Tracer("")

	t.Run("invalid input msg type", func(t *testing.T) {
		cm := &ConsensusManager{tracer: tracer}
		err := cm.sendRecoveryRequests(context.Background(), "foobar")
		require.EqualError(t, err, `failed to extract recovery info: unknown message type, cannot be used for recovery: string`)
	})

	t.Run("already in recovery status", func(t *testing.T) {
		cm := &ConsensusManager{recovery: &recoveryInfo{toRound: 42, sent: time.Now()}, tracer: tracer}

		toMsg := &abdrc.TimeoutMsg{
			Author: "16Uiu2HAm2qoNCweXVbxXPHAQxdJnEXEYYQ1bRfBwEi6nUhZMhWxD",
			Timeout: &drctypes.Timeout{
				HighQc: &drctypes.QuorumCert{
					Signatures: map[string]hex.Bytes{"16Uiu2HAm4r9uRwS67kwJEFuZWByM2YUNjW9j179n9Di9z58NTBEj": {4, 3, 2, 1}},
					// very old message, 10 rounds behind current recovery status
					VoteInfo: &drctypes.RoundInfo{RoundNumber: cm.recovery.toRound - 10}},
			},
		}

		err := cm.sendRecoveryRequests(context.Background(), toMsg)
		require.EqualError(t, err, `already in recovery to round 42, ignoring request to recover to round 32`)

		// just one round behind current recovery status
		toMsg.Timeout.HighQc.VoteInfo.RoundNumber = cm.recovery.toRound - 1
		err = cm.sendRecoveryRequests(context.Background(), toMsg)
		require.EqualError(t, err, `already in recovery to round 42, ignoring request to recover to round 41`)

		// should not send recovery request for the same recovery round again, ie we expect to get error
		toMsg.Timeout.HighQc.VoteInfo.RoundNumber = cm.recovery.toRound
		err = cm.sendRecoveryRequests(context.Background(), toMsg)
		require.EqualError(t, err, `already in recovery to round 42, ignoring request to recover to round 42`)
	})

	t.Run("previous request has timed out, repeat", func(t *testing.T) {
		// scenario: already in recovery but haven't got the status response or
		// failed to recover from it so the status request should be sent again
		nodeID, _, _, _ := generatePeerData(t)
		authID, _, _, _ := generatePeerData(t)
		toMsg := &abdrc.TimeoutMsg{
			Author: authID.String(),
			Timeout: &drctypes.Timeout{
				HighQc: &drctypes.QuorumCert{
					// add node itself into signatures too - shouldn't happen in real life that
					// node sends recovery request to itself but we just need valid ID here...
					Signatures: map[string]hex.Bytes{
						authID.String(): {4, 3, 2, 1},
						nodeID.String(): {5, 6, 7, 8},
					},
					VoteInfo: &drctypes.RoundInfo{RoundNumber: 66}},
			},
		}

		nw := newMockNetwork(t)
		cm := &ConsensusManager{
			id:  nodeID,
			net: nw.Connect(nodeID),
			// init the sent time so is is older than limit
			recovery: &recoveryInfo{toRound: toMsg.Timeout.GetHqcRound(), sent: time.Now().Add(-statusReqShelfLife)},
			tracer:   tracer,
		}

		// call Connect to "register" the ID with mock network...
		require.NotNil(t, nw.Connect(authID))
		//...but use firewall to check that all expected messages are sent
		// we have two signatures in QC, status request should be sent to both
		var sawM sync.Mutex
		sawMsg := map[peer.ID]struct{}{}
		nw.SetFirewall(func(from, to peer.ID, msg any) bool {
			if _, ok := msg.(*abdrc.StateRequestMsg); !ok || from != nodeID {
				return false
			}

			sawM.Lock()
			defer sawM.Unlock()

			if _, ok := sawMsg[to]; ok {
				t.Errorf("request is already sent to %s", to)
			} else {
				sawMsg[to] = struct{}{}
			}
			return false
		})
		// trigger recovery request
		require.NoError(t, cm.sendRecoveryRequests(context.Background(), toMsg))
		require.Eventually(t,
			func() bool {
				sawM.Lock()
				defer sawM.Unlock()
				return len(sawMsg) == 2
			},
			2*time.Second, 200*time.Millisecond)
	})

	t.Run("state request is sent to the author", func(t *testing.T) {
		nodeID, _, _, _ := generatePeerData(t)
		authID, _, _, _ := generatePeerData(t)
		nw := newMockNetwork(t)
		cm := &ConsensusManager{id: nodeID, net: nw.Connect(nodeID), tracer: tracer}

		// single signature by the author so only that node should receive the request
		toMsg := &abdrc.TimeoutMsg{
			Author: authID.String(),
			Timeout: &drctypes.Timeout{
				HighQc: &drctypes.QuorumCert{
					Signatures: map[string]hex.Bytes{authID.String(): {4, 3, 2, 1}},
					VoteInfo:   &drctypes.RoundInfo{RoundNumber: 66}},
			},
		}

		// author should receive "get state" request
		authorCon := nw.Connect(authID)
		authorErr := make(chan error, 1)
		go func() {
			select {
			case msg := <-authorCon.ReceivedChannel():
				var err error
				if m, ok := msg.(*abdrc.StateRequestMsg); ok {
					if m.NodeId != nodeID.String() {
						err = errors.Join(err, fmt.Errorf("expected receiver %s got %s", nodeID.String(), m.NodeId))
					}
				} else {
					err = errors.Join(err, fmt.Errorf("unexpected message transaction type %T", msg))
				}
				authorErr <- err
			case <-time.After(time.Second):
				authorErr <- fmt.Errorf("author didn't receive get status request within timeout")
			}
		}()
		err := cm.sendRecoveryRequests(context.Background(), toMsg)
		require.NoError(t, err)

		err = <-authorErr
		require.NoError(t, err)

		require.NotNil(t, cm.recovery)
		require.Equal(t, toMsg.Timeout.HighQc.VoteInfo.RoundNumber, cm.recovery.toRound)
		require.Empty(t, nw.errs)
	})

	// nice to have tests (to increase coverage):
	// - invalid author id in the msg (decoding string to peer.ID fails)
	// - upgrade recovery round (ie already in recovery but new msg has higher round)
	// - author + one additional signer should receive status request
	// - sending msg to network fails (a: to one receiver; b: to all receivers)
}

func Test_msgToRecoveryInfo(t *testing.T) {
	t.Parallel()

	t.Run("invalid input", func(t *testing.T) {
		info, sig, err := msgToRecoveryInfo(nil)
		require.Empty(t, info)
		require.Empty(t, sig)
		require.EqualError(t, err, `unknown message type, cannot be used for recovery: <nil>`)

		info, sig, err = msgToRecoveryInfo(42)
		require.Empty(t, info)
		require.Empty(t, sig)
		require.EqualError(t, err, `unknown message type, cannot be used for recovery: int`)

		var msg = struct{ s string }{""}
		info, sig, err = msgToRecoveryInfo(msg)
		require.Empty(t, info)
		require.Empty(t, sig)
		require.EqualError(t, err, `unknown message type, cannot be used for recovery: struct { s string }`)
	})

	t.Run("valid input", func(t *testing.T) {
		nodeID := "16Uiu2HAm2qoNCweXVbxXPHAQxdJnEXEYYQ1bRfBwEi6nUhZMhWxD"
		signatures := map[string]hex.Bytes{"16Uiu2HAm4r9uRwS67kwJEFuZWByM2YUNjW9j179n9Di9z58NTBEj": {4, 3, 2, 1}}
		quorumCert := &drctypes.QuorumCert{Signatures: signatures, VoteInfo: &drctypes.RoundInfo{RoundNumber: 7, ParentRoundNumber: 6}}

		proposalMsg := &abdrc.ProposalMsg{Block: &drctypes.BlockData{Round: 5, Author: nodeID, Qc: quorumCert}}
		voteMsg := &abdrc.VoteMsg{Author: nodeID, VoteInfo: &drctypes.RoundInfo{RoundNumber: 8}, HighQc: quorumCert}
		toMsg := &abdrc.TimeoutMsg{Author: nodeID, Timeout: &drctypes.Timeout{HighQc: quorumCert}}

		var tests = []struct {
			name       string
			input      any
			info       *recoveryInfo
			signatures map[string]hex.Bytes
		}{
			{
				name:       "proposal message",
				input:      proposalMsg,
				info:       &recoveryInfo{toRound: proposalMsg.Block.Qc.GetRound(), triggerMsg: proposalMsg},
				signatures: signatures,
			},
			{
				name:       "vote message",
				input:      voteMsg,
				info:       &recoveryInfo{toRound: voteMsg.HighQc.GetRound(), triggerMsg: voteMsg},
				signatures: signatures,
			},
			{
				name:       "timeout message",
				input:      toMsg,
				info:       &recoveryInfo{toRound: toMsg.Timeout.GetHqcRound(), triggerMsg: toMsg},
				signatures: signatures,
			},
			{
				name:       "quorum certificate",
				input:      quorumCert,
				info:       &recoveryInfo{toRound: quorumCert.GetParentRound(), triggerMsg: quorumCert},
				signatures: signatures,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				info, sig, err := msgToRecoveryInfo(tc.input)
				require.NoError(t, err)
				// check that the info created by func has sent time assigned
				// but then zero it so that the equality check would not fail
				require.False(t, info.sent.IsZero())
				info.sent = time.Time{}
				require.Equal(t, tc.info, info)
				require.Equal(t, tc.signatures, sig)
			})
		}
	})
}

func Test_recoverState(t *testing.T) {
	t.Parallel()

	// consumeUC acts as a validator node consuming the UC-s generated by CM (until ctx is cancelled).
	// strictly speaking not needed as current implementation should continue working even when there
	// is no-one consuming UCs.
	consumeUC := func(ctx context.Context, cm *ConsensusManager) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-cm.certResultCh:
			}
		}
	}

	partitionRecs := []*genesis.PartitionRecord{
		createPartitionRecord(t, partitionID, partitionInputRecord, 2),
	}

	t.Run("late joiner catches up", func(t *testing.T) {
		t.Parallel()
		// test scenario requires to be able to have quorum while "stopping" exactly one manager
		// for quorum we need ⅔+1 validators to be healthy thus with 4 nodes one can be unhealthy
		var cmCount atomic.Int32
		cmCount.Store(4)
		cms, rootNet, _ := createConsensusManagers(t, int(cmCount.Load()), partitionRecs)

		// tweak configurations - use "constant leader" to take leader selection out of test
		cmLeader := cms[0]
		allNodes := cmLeader.leaderSelector.GetNodes()
		for _, v := range cms {
			v.leaderSelector = constLeader{leader: cmLeader.id, nodes: allNodes}
		}
		// launch the managers (except last one; we still have quorum)
		ctx, cancel := context.WithCancel(context.Background())
		for _, v := range cms[:len(cms)-1] {
			go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled); cmCount.Add(-1) }(v)
			go func(cm *ConsensusManager) { consumeUC(ctx, cm) }(v)
		}
		// wait few rounds so some history is created
		require.Eventually(t, func() bool { return cmLeader.pacemaker.GetCurrentRound() >= 6 }, 5*time.Second, 20*time.Millisecond, "waiting for rounds to be processed")

		// start the manager that was skipped in the beginning, it is behind other nodes but
		// should receive (usually proposal) message which will trigger recovery
		cmLate := cms[len(cms)-1]
		go func() { require.ErrorIs(t, cmLate.Run(ctx), context.Canceled); cmCount.Add(-1) }()
		go consumeUC(ctx, cmLate)

		// late starter should catch up with peer(s)
		cmPeer := cms[1]
		require.Eventually(t,
			func() bool {
				return cmPeer.pacemaker.GetCurrentRound() == cmLate.pacemaker.GetCurrentRound()
			},
			2*time.Second, 25*time.Millisecond, "waiting for sleepy consensus manager to catch up with the peers")

		// now cut off one of the other peers from the network - this means that in order to make progress
		// the late CM we wake up must participate in the consensus now. keep track of number of proposals
		// made as these indicate was the recovery success or not (ie we do not advance because of timeouts)
		var proposalCnt atomic.Int32
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool {
			if _, ok := msg.(*abdrc.ProposalMsg); ok {
				proposalCnt.Add(1)
			}
			return from == cmPeer.id || to == cmPeer.id
		})

		destRound := cmLeader.pacemaker.GetCurrentRound() + 8
		require.Eventually(t,
			func() bool {
				return cmLeader.pacemaker.GetCurrentRound() >= destRound
			}, 6*time.Second, 100*time.Millisecond, "waiting for round %d to be processed", destRound)
		cancel()
		// we have 4 nodes and we expect at least 6 successful rounds (proposal for the first round might be
		// already sent and for the last round we might not see all messages as test ends as soon as the node
		// we check reaches that round).
		require.GreaterOrEqual(t, proposalCnt.Load(), int32(4*6), "didn't see expected number of proposals")
		require.Eventually(t, func() bool { return cmCount.Load() == 0 }, 3*time.Second, 200*time.Millisecond, "waiting for the CMs to quit")
	})

	t.Run("peer drops out of network", func(t *testing.T) {
		t.Parallel()
		// test scenario requires to be able to have quorum while "stopping" exactly one manager
		// for quorum we need ⅔+1 validators to be healthy thus with 4 nodes one can be unhealthy
		var cmCount atomic.Int32
		cmCount.Store(4)
		cms, rootNet, _ := createConsensusManagers(t, int(cmCount.Load()), partitionRecs)

		ctx, cancel := context.WithCancel(context.Background())
		defer func() {
			cancel() // kill CMs
			require.Eventually(t, func() bool { return cmCount.Load() == 0 }, 3*time.Second, 200*time.Millisecond, "waiting for the CMs to quit")
		}()

		cmLeader := cms[0]
		allNodes := cmLeader.leaderSelector.GetNodes()
		for _, v := range cms {
			v.leaderSelector = constLeader{leader: cmLeader.id, nodes: allNodes}
			go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled); cmCount.Add(-1) }(v)
			go func(cm *ConsensusManager) { consumeUC(ctx, cm) }(v)
		}
		// wait few rounds so some history is created
		require.Eventually(t, func() bool { return cmLeader.pacemaker.GetCurrentRound() >= 6 }, 6*time.Second, 20*time.Millisecond, "waiting for rounds to be processed")

		// block traffic to one peer and wait it to fall few rounds behind
		cmBlocked := cms[1]
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool { return to == cmBlocked.id })
		require.Eventually(t, func() bool { return cmLeader.pacemaker.GetCurrentRound() >= cmBlocked.pacemaker.GetCurrentRound()+5 }, 3*time.Second, 20*time.Millisecond, "waiting for blocked CM to fall behind")

		// now block different peer - the one which fell behind should recover and system should
		// still make progress (have quorum of healthy nodes)
		blockedID := cms[2].id
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool { return from == blockedID || to == blockedID })

		destRound := cmLeader.pacemaker.GetCurrentRound() + 8
		require.Eventually(t,
			func() bool {
				return cmLeader.pacemaker.GetCurrentRound() >= destRound
			}, 9*time.Second, 300*time.Millisecond, "waiting for round %d to be processed", destRound)
	})

	t.Run("less than quorum nodes are live for a period", func(t *testing.T) {
		t.Parallel()
		// test scenario requires to be able to have quorum while "stopping" exactly one manager
		// for quorum we need ⅔+1 validators to be healthy thus with 4 nodes one can be unhealthy
		var cmCount atomic.Int32
		cmCount.Store(4)
		cms, rootNet, _ := createConsensusManagers(t, int(cmCount.Load()), partitionRecs)

		ctx, cancel := context.WithCancel(context.Background())
		defer func() {
			cancel() // kill CMs
			require.Eventually(t, func() bool { return cmCount.Load() == 0 }, 3*time.Second, 200*time.Millisecond, "waiting for the CMs to quit")
		}()

		allNodes := cms[0].leaderSelector.GetNodes()
		for _, v := range cms {
			v.leaderSelector = constLeader{leader: cms[0].id, nodes: allNodes} // to take leader selection out of test
			go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled); cmCount.Add(-1) }(v)
			go func(cm *ConsensusManager) { consumeUC(ctx, cm) }(v)
		}
		// wait few rounds so some history is created
		require.Eventually(t, func() bool { return cms[0].pacemaker.GetCurrentRound() >= 6 }, 6*time.Second, 20*time.Millisecond, "waiting for rounds to be processed")

		// block traffic from two nodes - this means there should be no progress possible
		// as not enough nodes participate in voting
		blockedID1 := cms[1].id
		blockedID2 := cms[2].id
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool {
			return from == blockedID1 || from == blockedID2
		})
		round := cms[0].pacemaker.GetCurrentRound()
		time.Sleep(3 * cms[0].params.LocalTimeout)
		// instead of equal check use "LessOrEqual round+1" as there is a race - sometimes quorum votes
		// is already sent (and system advances to next round) before firewall takes effect
		require.LessOrEqual(t, cms[0].pacemaker.GetCurrentRound(), round+1, "round should not have been advanced as there is not enough nodes for a quorum")
		destRound := cms[0].pacemaker.GetCurrentRound() + 8

		// allow traffic from all nodes again - system should recover and make progress again
		var proposalCnt atomic.Int32
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool {
			if _, ok := msg.(*abdrc.ProposalMsg); ok {
				proposalCnt.Add(1)
			}
			return false
		})
		require.Eventually(t,
			func() bool {
				return cms[0].pacemaker.GetCurrentRound() >= destRound
			}, 10*time.Second, 300*time.Millisecond, "waiting for round %d to be processed", destRound)
		// we have 4 nodes and we expect at least 6 successful rounds (proposal for the first round might be
		// already sent and for the last round we might not see all messages as test ends as soon as one the
		// node we check reaches that round).
		require.GreaterOrEqual(t, proposalCnt.Load(), int32(4*6), "didn't see expected number of proposals")
	})

	t.Run("dead leader", func(t *testing.T) {
		t.Parallel()
		// testing what happens when leader "goes dark". easiest to simulate is using
		// round-robin leader selector where one node is not responsive.
		// test scenario requires to be able to have quorum while "stopping" exactly one manager
		// for quorum we need ⅔+1 validators to be healthy thus with 4 nodes one can be unhealthy
		var cmCount atomic.Int32
		cmCount.Store(4)
		cms, rootNet, _ := createConsensusManagers(t, int(cmCount.Load()), partitionRecs)
		deadID := cms[1].id
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool {
			return from == deadID || to == deadID
		})

		// round-robin leader in the order nodes are in the cms slice. system is starting
		// with round 2 so leader will be: 2, 3, 0, 1, 2, 3...
		rootNodes := make([]peer.ID, 0, len(cms))
		for _, v := range cms {
			rootNodes = append(rootNodes, v.id)
		}
		ls, err := leader.NewRoundRobin(rootNodes, 1)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer func() {
			cancel() // kill CMs
			require.Eventually(t, func() bool { return cmCount.Load() == 0 }, 3*time.Second, 200*time.Millisecond, "waiting for the CMs to quit")
		}()

		for _, v := range cms {
			v.leaderSelector = ls
			go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled); cmCount.Add(-1) }(v)
			go func(cm *ConsensusManager) { consumeUC(ctx, cm) }(v)
		}

		// we start with round 2 so going till the round 8 we deal with dead leader at least once:
		// round 4 should TO as next leader is dead one (index 1) so votes sent to it "disappear".
		// round 5 should TO as the dead leader doesn't send proposal.
		// NB! the total time this will take heavily depends on consensus timeout parameter!
		cmLive := cms[0]
		require.Eventually(t, func() bool {
			return cmLive.pacemaker.GetCurrentRound() >= 8
		}, 30*time.Second, 500*time.Millisecond, "waiting for rounds to be processed")
	})

	t.Run("recovery triggered by timeout", func(t *testing.T) {
		t.Parallel()
		// test scenario requires to be able to have quorum while "stopping" exactly one manager
		// for quorum we need ⅔+1 validators to be healthy thus with 4 nodes one can be unhealthy
		var cmCount atomic.Int32
		cmCount.Store(4)
		cms, rootNet, _ := createConsensusManagers(t, int(cmCount.Load()), partitionRecs)

		ctx, cancel := context.WithCancel(context.Background())
		defer func() {
			cancel() // kill CMs
			require.Eventually(t, func() bool { return cmCount.Load() == 0 }, 3*time.Second, 200*time.Millisecond, "waiting for the CMs to quit")
		}()

		cmLeader := cms[0]
		allNodes := cmLeader.leaderSelector.GetNodes()
		for _, v := range cms {
			v.leaderSelector = constLeader{leader: cmLeader.id, nodes: allNodes} // use "const leader" to take leader selection out of test
			go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled); cmCount.Add(-1) }(v)
			go func(cm *ConsensusManager) { consumeUC(ctx, cm) }(v)
		}
		// wait few rounds so some history is created
		require.Eventually(t, func() bool { return cmLeader.pacemaker.GetCurrentRound() >= 4 }, 3*time.Second, 20*time.Millisecond, "waiting for rounds to be processed")

		// drop node 1 out of network so it falls behind
		node1 := cms[1]
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool { return from == node1.id || to == node1.id })
		require.Eventually(t, func() bool { return cmLeader.pacemaker.GetCurrentRound() >= node1.pacemaker.GetCurrentRound()+2 }, 3*time.Second, 20*time.Millisecond, "waiting for node 1 to fall behind")

		// drop node2 out of network so it doesn't participate anymore, this should trigger timeout
		// round at the same time allow only TO message to the node1 so it can start recovery (once
		// we see that TO message has been sent to node1 allow all traffic to it again)
		node2 := cms[2]
		var tomSent atomic.Bool  // has the TO message been sent to node1?
		var propCnt atomic.Int32 // number of proposals made after recovery
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool {
			block := from == node2.id || to == node2.id // always block all node2 traffic
			if !block && (from == node1.id || to == node1.id) {
				block = !tomSent.Load()
				if block {
					// TO message hasn't been sent yet, is this message it?
					if _, ok := msg.(*abdrc.TimeoutMsg); ok && to == node1.id && from != node1.id {
						tomSent.Store(true)
						block = false
					}
				}
			}
			if tomSent.Load() {
				if _, ok := msg.(*abdrc.ProposalMsg); ok {
					propCnt.Add(1)
				}
			}
			return block
		})

		destRound := cmLeader.pacemaker.GetCurrentRound() + 5
		require.Eventually(t, func() bool { return cmLeader.pacemaker.GetCurrentRound() >= destRound }, 10*time.Second, 20*time.Millisecond, "waiting for progress to be made again")
		// we have 4 nodes and we expect at least 3 successful rounds (the network should see proposal messages).
		// if we do not see these then the progress was probably made by timeouts and thus recovery wasn't success?
		require.GreaterOrEqual(t, propCnt.Load(), int32(4*3), "didn't see expected number of proposals")
	})

	t.Run("recovery triggered by vote", func(t *testing.T) {
		t.Parallel()
		// test scenario requires to be able to have quorum while "stopping" exactly one manager
		// for quorum we need ⅔+1 validators to be healthy thus with 4 nodes one can be unhealthy
		var cmCount atomic.Int32
		cmCount.Store(4)
		cms, rootNet, _ := createConsensusManagers(t, int(cmCount.Load()), partitionRecs)
		// round-robin leader in the order nodes are in the cms slice. system is starting
		// with round 2 and leader will be: 2, 3, 0, 1, 2, 3, 0, 1,...
		rootNodes := make([]peer.ID, 0, len(cms))
		for _, v := range cms {
			rootNodes = append(rootNodes, v.id)
		}
		rrLeader, err := leader.NewRoundRobin(rootNodes, 1)
		require.NoError(t, err)

		// for the first three rounds we block node 1 (causing it to go out of sync), for round 4 we
		// allow vote messages so thats what triggers recovery (node 1 will be leader of round 5
		// so round 4 votes are sent to it).
		// starting from round 5 we block node 0 but unblock node 1 so if it has recovered we still
		// have quorum and progress must be made
		node_0_id := cms[0].id // round leader: 4, 8
		node_1_id := cms[1].id // round leader: 5, 9
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool {
			var round uint64
			switch mt := msg.(type) {
			case *abdrc.VoteMsg:
				round = mt.VoteInfo.RoundNumber
			case *abdrc.ProposalMsg:
				round = mt.Block.Round
			case *abdrc.TimeoutMsg:
				round = mt.GetRound()
			case *abdrc.StateRequestMsg, *abdrc.StateMsg:
				return false
			}

			block := false
			switch {
			case round < 4:
				block = (from == node_1_id || to == node_1_id)
			case round == 4:
				// allow vote msg only for node 1 so that's what triggers recovery
				_, isVote := msg.(*abdrc.VoteMsg)
				block = (from == node_1_id || to == node_1_id) && !isVote
			case round >= 5:
				// block another peer
				block = from == node_0_id || to == node_0_id
			}

			//t.Logf("%t: %s -> %s : (%d) %T", block, from.ShortString(), to.ShortString(), round, msg.Message)
			return block
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer func() {
			cancel() // kill CMs
			require.Eventually(t, func() bool { return cmCount.Load() == 0 }, 3*time.Second, 200*time.Millisecond, "waiting for the CMs to quit")
		}()

		for _, v := range cms {
			v.leaderSelector = rrLeader
			go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled); cmCount.Add(-1) }(v)
			go func(cm *ConsensusManager) { consumeUC(ctx, cm) }(v)
		}

		// note that rounds 7 and 8 will go into timeout as the leader of the round 8 will be node_0
		// which we have blocked in the firewall after round 4 (thus no QC for R7 and no proposal for R8).
		node_2 := cms[2]
		require.Eventually(t, func() bool { return node_2.pacemaker.GetCurrentRound() >= 10 }, 15*time.Second, 100*time.Millisecond, "waiting for progress to be made")
	})
	t.Run("recovery triggered by missing proposal", func(t *testing.T) {
		t.Parallel()
		// test scenario requires to be able to have quorum while "stopping" exactly one manager
		// for quorum we need ⅔+1 validators to be healthy thus with 4 nodes one can be unhealthy
		var cmCount atomic.Int32
		cmCount.Store(4)
		cms, rootNet, _ := createConsensusManagers(t, int(cmCount.Load()), partitionRecs)

		ctx, cancel := context.WithCancel(context.Background())
		defer func() {
			cancel() // kill CMs
			require.Eventually(t, func() bool { return cmCount.Load() == 0 }, 2*time.Second, 200*time.Millisecond, "waiting for the CMs to quit")
		}()

		cmLeader := cms[0]
		allNodes := cmLeader.leaderSelector.GetNodes()
		for _, v := range cms {
			rrLeader, err := leader.NewRoundRobin(allNodes, 1)
			require.NoError(t, err)
			v.leaderSelector = rrLeader
			go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled); cmCount.Add(-1) }(v)
			go func(cm *ConsensusManager) { consumeUC(ctx, cm) }(v)
		}
		// make sure leader will not receive it's own proposal for round 4
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool {
			prop, isProposal := msg.(*abdrc.ProposalMsg)
			leaderInRound := cmLeader.leaderSelector.GetLeaderForRound(5)
			if to == leaderInRound && isProposal && prop.Block.Round == 4 {
				return true
			}
			return false
		})
		// make sure leader still issues a proposal after recovery
		require.Eventually(t, func() bool { return cmLeader.pacemaker.GetCurrentRound() >= 5 }, 3*time.Second, 20*time.Millisecond, "make progress")
	})

	t.Run("recovery triggered by missing proposal - delay proposal", func(t *testing.T) {
		t.Parallel()
		// test scenario requires to be able to have quorum while "stopping" exactly one manager
		// for quorum we need ⅔+1 validators to be healthy thus with 4 nodes one can be unhealthy
		var cmCount atomic.Int32
		cmCount.Store(4)
		cms, rootNet, _ := createConsensusManagers(t, int(cmCount.Load()), partitionRecs)

		ctx, cancel := context.WithCancel(context.Background())
		defer func() {
			cancel() // kill CMs
			require.Eventually(t, func() bool { return cmCount.Load() == 0 }, 2*time.Second, 200*time.Millisecond, "waiting for the CMs to quit")
		}()

		cmLeader := cms[0]
		allNodes := cmLeader.leaderSelector.GetNodes()
		for _, v := range cms {
			rrLeader, err := leader.NewRoundRobin(allNodes, 1)
			require.NoError(t, err)
			v.leaderSelector = rrLeader
			go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled); cmCount.Add(-1) }(v)
			go func(cm *ConsensusManager) { consumeUC(ctx, cm) }(v)
		}
		// make sure leader will not receive it's own proposal for round 4
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool {
			prop, isProposal := msg.(*abdrc.ProposalMsg)
			leaderInRound := cmLeader.leaderSelector.GetLeaderForRound(5)
			if to == leaderInRound && isProposal && prop.Block.Round == 4 {
				time.Sleep(1 * time.Millisecond)
			}
			return false
		})
		// make sure leader still issues a proposal after recovery
		require.Eventually(t, func() bool { return cmLeader.pacemaker.GetCurrentRound() >= 5 }, 3*time.Second, 20*time.Millisecond, "make progress")
	})
	roundOfMsg := func(msg any) uint64 {
		switch mt := msg.(type) {
		case *abdrc.VoteMsg:
			return mt.VoteInfo.RoundNumber
		case *abdrc.ProposalMsg:
			return mt.Block.Round
		case *abdrc.TimeoutMsg:
			return mt.GetRound()
		}
		return 0
	}

	t.Run("recover from different timeout rounds", func(t *testing.T) {
		t.Parallel()
		// system must be able to recover when consecutive rounds time out and nodes are in different
		// timeout rounds (ie node doesn't get quorum for latest round so stays in previous TO round)
		var cmCount atomic.Int32
		cmCount.Store(3)
		cms, rootNet, _ := createConsensusManagers(t, int(cmCount.Load()), partitionRecs)

		// set filter so that one node (slowID) does not see any messages and only sends TO votes
		slowID := cms[0].id
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool {
			block := to == slowID || from == slowID
			if _, tom := msg.(*abdrc.TimeoutMsg); tom {
				block = to == slowID
			}
			t.Logf("%t: %s -> %s : (%d) %T", block, from.ShortString(), to.ShortString(), roundOfMsg(msg), msg)
			return block
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer func() {
			cancel() // kill CMs
			require.Eventually(t, func() bool { return cmCount.Load() == 0 }, 3*time.Second, 200*time.Millisecond, "waiting for the CMs to quit")
		}()

		allNodes := cms[0].leaderSelector.GetNodes()
		for _, v := range cms {
			v.leaderSelector = constLeader{leader: cms[1].id, nodes: allNodes} // use "const leader" to take leader selection out of test
			go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled); cmCount.Add(-1) }(v)
		}
		// what we expect to happen:
		// round 1 is genesis, round 2 proposal is sent without votes after round matures.
		// as slowID doesn't receive any messages it stays in round 2 and broadcast timeout vote for it.
		// other CMs get quorum for round 2 timeout and go to round 3 but after that they are unable to
		// make progress as slowID is still in round 2 and sends stale votes.
		require.Eventually(t, func() bool { return cms[1].pacemaker.GetCurrentRound() == 3 && cms[2].pacemaker.GetCurrentRound() == 3 }, 8*time.Second, 20*time.Millisecond, "waiting for rounds to be processed")
		require.EqualValues(t, 2, cms[0].pacemaker.GetCurrentRound())
		require.EqualValues(t, 3, cms[1].pacemaker.GetCurrentRound())
		require.EqualValues(t, 3, cms[2].pacemaker.GetCurrentRound())

		// allow all traffic again - slowID node should receive TO vote for round 3 with TC for round 2
		// which should allow it to go to round 3 and vote for it's timeout. this means that quorum for
		// round 3 TO is achieved and progress is made again
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool {
			t.Logf("%t: %s -> %s : (%d) %T", false, from.ShortString(), to.ShortString(), roundOfMsg(msg), msg)
			return false
		})
		require.Eventually(t, func() bool { return cms[0].pacemaker.GetCurrentRound() >= 4 }, 2*cms[0].pacemaker.maxRoundLen, 100*time.Millisecond, "waiting for progress to be made")
	})
}

func createConsensusManagers(t *testing.T, count int, partitionRecs []*genesis.PartitionRecord) ([]*ConsensusManager, *mockNetwork, *genesis.RootGenesis) {
	t.Helper()

	observe := testobservability.Default(t)
	signers := map[string]abcrypto.Signer{}
	var rgr []*genesis.RootGenesis
	for i := 0; i < count; i++ {
		nodeID, signer, _, pubkey := generatePeerData(t)
		rootG, _, err := rootgenesis.NewRootGenesis(nodeID.String(), signer, pubkey, partitionRecs, rootgenesis.WithTotalNodes(uint32(count)), rootgenesis.WithBlockRate(500), rootgenesis.WithConsensusTimeout(2500))
		require.NoError(t, err, "failed to create root genesis")
		require.NotNil(t, rootG)
		rgr = append(rgr, rootG)
		signers[nodeID.String()] = signer
	}

	rootG, partG, err := rootgenesis.MergeRootGenesisFiles(rgr)
	require.NoError(t, err, "failed to merge root genesis records")
	require.NotNil(t, partG)
	require.NotNil(t, rootG)

	trustBase, err := rootG.GenerateTrustBase()
	require.NoError(t, err)

	nw := newMockNetwork(t)
	cms := make([]*ConsensusManager, 0, len(rootG.Root.RootValidators))
	for _, v := range rootG.Root.RootValidators {
		nodeID, err := peer.Decode(v.NodeIdentifier)
		require.NoError(t, err)

		cm, err := NewConsensusManager(nodeID, rootG, trustBase, partitions.NewOrchestration(rootG), nw.Connect(nodeID), signers[v.NodeIdentifier], observability.WithLogger(observe, observe.Logger().With(logger.NodeID(nodeID))))
		require.NoError(t, err)
		cms = append(cms, cm)
	}

	return cms, nw, rootG
}

func createPartitionRecord(t *testing.T, partitionID abtypes.PartitionID, ir *abtypes.InputRecord, nrOfValidators int) *genesis.PartitionRecord {
	t.Helper()
	record := &genesis.PartitionRecord{
		PartitionDescription: &abtypes.PartitionDescriptionRecord{
Version: 1,
			NetworkIdentifier:   5,
			PartitionIdentifier: partitionID,
			TypeIdLen:           8,
			UnitIdLen:           256,
			T2Timeout:           2500 * time.Millisecond,
		},
	}

	for i := 0; i < nrOfValidators; i++ {
		nodeID, signer, _, pubKey := generatePeerData(t)

		req := &certification.BlockCertificationRequest{
			Partition:      partitionID,
			NodeIdentifier: nodeID.String(),
			InputRecord:    ir,
		}
		require.NoError(t, req.Sign(signer))

		record.Validators = append(record.Validators, &genesis.PartitionNode{
			Version:                    1,
			NodeIdentifier:             nodeID.String(),
			SigningPublicKey:           pubKey,
			EncryptionPublicKey:        pubKey,
			BlockCertificationRequest:  req,
			PartitionDescriptionRecord: *record.PartitionDescription,
		})
	}

	return record
}

func generatePeerData(t *testing.T) (peer.ID, abcrypto.Signer, abcrypto.Verifier, []byte) {
	t.Helper()

	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	verifier, err := signer.Verifier()
	require.NoError(t, err)

	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	nodeID, err := network.NodeIDFromPublicKeyBytes(pubKey)
	require.NoError(t, err)

	return nodeID, signer, verifier, pubKey
}

/*
constLeader is leader selection algorithm which always returns the same leader.
*/
type constLeader struct {
	leader peer.ID
	nodes  []peer.ID
}

func (cl constLeader) GetLeaderForRound(round uint64) peer.ID { return cl.leader }

func (cl constLeader) GetNodes() []peer.ID { return cl.nodes }

func (cl constLeader) Update(qc *drctypes.QuorumCert, currentRound uint64, b leader.BlockLoader) error {
	return nil
}
