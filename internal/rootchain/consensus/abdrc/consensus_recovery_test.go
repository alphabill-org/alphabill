package abdrc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/abdrc"
	protocgenesis "github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/leader"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/rootchain/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/rootchain/testutils"
)

func Test_ConsensusManager_sendRecoveryRequests(t *testing.T) {
	t.Parallel()

	// the sendRecoveryRequests method depends only on "id", "net" and "recovery" fields
	// so we can use "shortcut" when creating the ConsensusManager for test (and init
	// only required fields)

	t.Run("invalid input msg type", func(t *testing.T) {
		cm := &ConsensusManager{}
		err := cm.sendRecoveryRequests("foobar")
		require.EqualError(t, err, `failed to extract recovery info: unknown message type, cannot be used for recovery: string`)
	})

	t.Run("already in recovery status", func(t *testing.T) {
		cm := &ConsensusManager{recovery: &recoveryInfo{toRound: 42}}

		toMsg := &abdrc.TimeoutMsg{
			Author: "16Uiu2HAm2qoNCweXVbxXPHAQxdJnEXEYYQ1bRfBwEi6nUhZMhWxD",
			Timeout: &types.Timeout{
				HighQc: &types.QuorumCert{
					Signatures: map[string][]byte{"16Uiu2HAm4r9uRwS67kwJEFuZWByM2YUNjW9j179n9Di9z58NTBEj": {4, 3, 2, 1}},
					// very old message, 10 rounds behind current recovery status
					VoteInfo: &types.RoundInfo{RoundNumber: cm.recovery.toRound - 10}},
			},
		}

		err := cm.sendRecoveryRequests(toMsg)
		require.EqualError(t, err, `already in recovery to round 42, ignoring request to recover to round 32`)

		// just one round behind current recovery status
		toMsg.Timeout.HighQc.VoteInfo.RoundNumber = cm.recovery.toRound - 1
		err = cm.sendRecoveryRequests(toMsg)
		require.EqualError(t, err, `already in recovery to round 42, ignoring request to recover to round 41`)

		// should not send recovery request for the same recovery round again, ie we expect to get error
		toMsg.Timeout.HighQc.VoteInfo.RoundNumber = cm.recovery.toRound
		err = cm.sendRecoveryRequests(toMsg)
		require.EqualError(t, err, `already in recovery to round 42, ignoring request to recover to round 42`)
	})

	t.Run("state request is sent to the author", func(t *testing.T) {
		nodeID, _, _, _ := generatePeerData(t)
		authID, _, _, _ := generatePeerData(t)
		nw := newMockNetwork()
		cm := &ConsensusManager{id: nodeID, net: nw.Connect(nodeID)}

		// single signature by the author so only that node should receive the request
		toMsg := &abdrc.TimeoutMsg{
			Author: authID.String(),
			Timeout: &types.Timeout{
				HighQc: &types.QuorumCert{
					Signatures: map[string][]byte{authID.String(): {4, 3, 2, 1}},
					VoteInfo:   &types.RoundInfo{RoundNumber: 66}},
			},
		}

		// author should receive "get state" request
		authorCon := nw.Connect(authID)
		authorErr := make(chan error, 1)
		go func() {
			select {
			case msg := <-authorCon.ReceivedChannel():
				var err error
				if msg.From != nodeID {
					err = errors.Join(err, fmt.Errorf("expected sender %s got %s", nodeID, msg.From))
				}
				if msg.Protocol != network.ProtocolRootStateReq {
					err = errors.Join(err, fmt.Errorf("unexpected message protocol: %q", msg.Protocol))
				}

				if m, ok := msg.Message.(*abdrc.GetStateMsg); ok {
					if m.NodeId != nodeID.String() {
						err = errors.Join(err, fmt.Errorf("expected receiver %s got %s", nodeID.String(), m.NodeId))
					}
				} else {
					err = errors.Join(err, fmt.Errorf("unexpected message payload type %T", msg.Message))
				}
				authorErr <- err
			case <-time.After(time.Second):
				authorErr <- fmt.Errorf("author didn't receive get status request within timeout")
			}
		}()

		err := cm.sendRecoveryRequests(toMsg)
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
		info, author, sig, err := msgToRecoveryInfo(nil)
		require.Empty(t, info)
		require.Empty(t, author)
		require.Empty(t, sig)
		require.EqualError(t, err, `unknown message type, cannot be used for recovery: <nil>`)

		info, author, sig, err = msgToRecoveryInfo(42)
		require.Empty(t, info)
		require.Empty(t, author)
		require.Empty(t, sig)
		require.EqualError(t, err, `unknown message type, cannot be used for recovery: int`)

		var msg = struct{ s string }{""}
		info, author, sig, err = msgToRecoveryInfo(msg)
		require.Empty(t, info)
		require.Empty(t, author)
		require.Empty(t, sig)
		require.EqualError(t, err, `unknown message type, cannot be used for recovery: struct { s string }`)
	})

	t.Run("valid input", func(t *testing.T) {
		nodeID := "16Uiu2HAm2qoNCweXVbxXPHAQxdJnEXEYYQ1bRfBwEi6nUhZMhWxD"
		signatures := map[string][]byte{"16Uiu2HAm4r9uRwS67kwJEFuZWByM2YUNjW9j179n9Di9z58NTBEj": {4, 3, 2, 1}}
		quorumCert := &types.QuorumCert{Signatures: signatures, VoteInfo: &types.RoundInfo{RoundNumber: 7}}

		proposalMsg := &abdrc.ProposalMsg{Block: &types.BlockData{Round: 5, Author: nodeID, Qc: quorumCert}}
		voteMsg := &abdrc.VoteMsg{Author: nodeID, VoteInfo: &types.RoundInfo{RoundNumber: 8}, HighQc: quorumCert}
		toMsg := &abdrc.TimeoutMsg{Author: nodeID, Timeout: &types.Timeout{HighQc: quorumCert}}

		var tests = []struct {
			name       string
			input      any
			info       *recoveryInfo
			author     string
			signatures map[string][]byte
		}{
			{
				name:       "proposal message",
				input:      proposalMsg,
				info:       &recoveryInfo{toRound: proposalMsg.Block.Qc.GetRound(), triggerMsg: proposalMsg},
				author:     proposalMsg.Block.Author,
				signatures: signatures,
			},
			{
				name:       "vote message",
				input:      voteMsg,
				info:       &recoveryInfo{toRound: voteMsg.HighQc.GetRound(), triggerMsg: voteMsg},
				author:     voteMsg.Author,
				signatures: signatures,
			},
			{
				name:       "timeout message",
				input:      toMsg,
				info:       &recoveryInfo{toRound: toMsg.Timeout.GetHqcRound(), triggerMsg: toMsg},
				author:     toMsg.Author,
				signatures: signatures,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				info, author, sig, err := msgToRecoveryInfo(tc.input)
				require.NoError(t, err)
				require.Equal(t, tc.info, info)
				require.Equal(t, tc.author, author)
				require.Equal(t, tc.signatures, sig)
			})
		}
	})
}

func Test_recoverState(t *testing.T) {
	t.Parallel()

	// consumeUC acts as a validator node consuming the UC-s generated by CM (until ctx is cancelled).
	// UC-s need to be consumed as otherwise CM blocks where it tries to send them, ie T2 timeout!
	consumeUC := func(ctx context.Context, cm *ConsensusManager) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-cm.certResultCh:
			}
		}
	}

	t.Run("late joiner catches up", func(t *testing.T) {
		t.Parallel()
		// for quorum we need ⅔+1 validators to be healthy thus with 4 nodes one can be unhealthy
		cms, rootNet := createConsensusManagers(t, 4)

		// tweak configurations - use "constant leader" to take leader selection out of test
		cmLeader := cms[0]
		for _, v := range cms {
			v.leaderSelector = constLeader(cmLeader.id)
		}
		// launch the managers (except last one; we still have quorum)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		for _, v := range cms[:len(cms)-1] {
			go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }(v)
			go func(cm *ConsensusManager) { consumeUC(ctx, cm) }(v)
		}
		// wait few rounds so some history is created
		require.Eventually(t, func() bool { return cmLeader.pacemaker.GetCurrentRound() >= 6 }, 5*time.Second, 20*time.Millisecond, "waiting for rounds to be processed")

		// start the manager that was skipped in the beginning, it is behind other nodes but
		// should receive (usually proposal) message which will trigger recovery
		cmLate := cms[len(cms)-1]
		go func() { require.ErrorIs(t, cmLate.Run(ctx), context.Canceled) }()
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
		rootNet.SetFirewall(func(from, to peer.ID, msg network.OutputMessage) bool {
			if _, ok := msg.Message.(*abdrc.ProposalMsg); ok {
				proposalCnt.Add(1)
			}
			return from == cmPeer.id || to == cmPeer.id
		})

		destRound := cmLeader.pacemaker.GetCurrentRound() + 8
		require.Eventually(t,
			func() bool {
				return cmLeader.pacemaker.GetCurrentRound() >= destRound
			}, 6*time.Second, 100*time.Millisecond, "waiting for round %d to be processed", destRound)
		// we have 4 nodes and we expect at least 8 successful rounds so the network should see at least 32 proposal messages.
		// if we do not see these then the progress was probably made by timeouts and thus recovery wasn't success?
		require.GreaterOrEqual(t, proposalCnt.Load(), int32(4*8), "didn't see expected number of proposals")
	})

	t.Run("peer drops out of network", func(t *testing.T) {
		t.Parallel()
		// we need ⅔+1 validators to be healthy, with 4 nodes one can be unhealthy
		cms, rootNet := createConsensusManagers(t, 4)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cmLeader := cms[0]
		for _, v := range cms {
			v.leaderSelector = constLeader(cmLeader.id) // to take leader selection out of test
			go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }(v)
			go func(cm *ConsensusManager) { consumeUC(ctx, cm) }(v)
		}
		// wait few rounds so some history is created
		require.Eventually(t, func() bool { return cmLeader.pacemaker.GetCurrentRound() >= 6 }, 6*time.Second, 20*time.Millisecond, "waiting for rounds to be processed")

		// block traffic to one peer and wait it to fall few rounds behind
		cmBlocked := cms[1]
		rootNet.SetFirewall(func(from, to peer.ID, msg network.OutputMessage) bool { return to == cmBlocked.id })
		require.Eventually(t, func() bool { return cmLeader.pacemaker.GetCurrentRound() >= cmBlocked.pacemaker.GetCurrentRound()+5 }, 3*time.Second, 20*time.Millisecond, "waiting for blocked CM to fall behind")

		// now block different peer - the one which fell behind should recover and system should
		// still make progress (have quorum of healthy nodes)
		blockedID := cms[2].id
		rootNet.SetFirewall(func(from, to peer.ID, msg network.OutputMessage) bool { return from == blockedID || to == blockedID })

		destRound := cmLeader.pacemaker.GetCurrentRound() + 8
		require.Eventually(t,
			func() bool {
				return cmLeader.pacemaker.GetCurrentRound() >= destRound
			}, 9*time.Second, 300*time.Millisecond, "waiting for round %d to be processed", destRound)
	})

	t.Run("less than quorum nodes are live for a period", func(t *testing.T) {
		t.Parallel()
		// we need ⅔+1 validators to be healthy, with 4 nodes one can be unhealthy
		cms, rootNet := createConsensusManagers(t, 4)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		for _, v := range cms {
			v.leaderSelector = constLeader(cms[0].id) // to take leader selection out of test
			go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }(v)
			go func(cm *ConsensusManager) { consumeUC(ctx, cm) }(v)
		}
		// wait few rounds so some history is created
		require.Eventually(t, func() bool { return cms[0].pacemaker.GetCurrentRound() >= 6 }, 6*time.Second, 20*time.Millisecond, "waiting for rounds to be processed")

		// block traffic from two nodes - this means there should be no progress possible
		// as not enough nodes participate in voting
		blockedID1 := cms[1].id
		blockedID2 := cms[2].id
		rootNet.SetFirewall(func(from, to peer.ID, msg network.OutputMessage) bool {
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
		rootNet.SetFirewall(func(from, to peer.ID, msg network.OutputMessage) bool {
			if _, ok := msg.Message.(*abdrc.ProposalMsg); ok {
				proposalCnt.Add(1)
			}
			return false
		})
		require.Eventually(t,
			func() bool {
				return cms[0].pacemaker.GetCurrentRound() >= destRound
			}, 10*time.Second, 300*time.Millisecond, "waiting for round %d to be processed", destRound)
		// we have 4 nodes and we expect at least 7 successful rounds (depending on the timing occasionally
		// round 7 times out too so thats one less proposed round out of the 8 we wait after re-enabling traffic).
		// if we do not see these then the progress was probably made by timeouts and thus recovery wasn't success?
		require.GreaterOrEqual(t, proposalCnt.Load(), int32(4*7), "didn't see expected number of proposals")
	})

	t.Run("dead leader", func(t *testing.T) {
		t.Parallel()
		// testing what happens when leader "goes dark". easiest to simulate is using
		// round-robin leader selector where one node is not responsive.
		// for quorum we need ⅔+1 validators to be healthy thus with 4 nodes one can be unhealthy
		cms, rootNet := createConsensusManagers(t, 4)
		deadID := cms[1].id
		rootNet.SetFirewall(func(from, to peer.ID, msg network.OutputMessage) bool {
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
		defer cancel()
		for _, v := range cms {
			v.leaderSelector = ls
			go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }(v)
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
		// we need ⅔+1 validators to be healthy, with 3 nodes none can be unhealthy
		cms, rootNet := createConsensusManagers(t, 4)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cmLeader := cms[0]
		for _, v := range cms {
			v.leaderSelector = constLeader(cmLeader.id) // use "const leader" to take leader selection out of test
			go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }(v)
			go func(cm *ConsensusManager) { consumeUC(ctx, cm) }(v)
		}
		// wait few rounds so some history is created
		require.Eventually(t, func() bool { return cmLeader.pacemaker.GetCurrentRound() >= 4 }, 3*time.Second, 20*time.Millisecond, "waiting for rounds to be processed")

		// drop node 1 out of network so it falls behind
		node1 := cms[1]
		rootNet.SetFirewall(func(from, to peer.ID, msg network.OutputMessage) bool { return from == node1.id || to == node1.id })
		require.Eventually(t, func() bool { return cmLeader.pacemaker.GetCurrentRound() >= node1.pacemaker.GetCurrentRound()+2 }, 3*time.Second, 20*time.Millisecond, "waiting for node 1 to fall behind")

		// drop node2 out of network so it doesn't participate anymore, this should trigger timeout
		// round at the same time allow only TO message to the node1 so it can start recovery (once
		// we see that TO message has been sent to node1 allow all traffic to it again)
		node2 := cms[2]
		var tomSent atomic.Bool  // has the TO message been sent to node1?
		var propCnt atomic.Int32 // number of proposals made after recovery
		rootNet.SetFirewall(func(from, to peer.ID, msg network.OutputMessage) bool {
			block := from == node2.id || to == node2.id // always block all node2 traffic
			if !block && (from == node1.id || to == node1.id) {
				block = !tomSent.Load()
				if block {
					// TO message hasn't been sent yet, is this message it?
					if _, ok := msg.Message.(*abdrc.TimeoutMsg); ok && to == node1.id && from != node1.id {
						tomSent.Store(true)
						block = false
					}
				}
			}
			if tomSent.Load() {
				if _, ok := msg.Message.(*abdrc.ProposalMsg); ok {
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
}

func createConsensusManagers(t *testing.T, count int) ([]*ConsensusManager, *mockNetwork) {
	t.Helper()

	// shortcut to create partition records, we do not need partition nodes...
	_, partitionRecord := testutils.CreatePartitionNodesAndPartitionRecord(t, partitionInputRecord, partitionID, 1)
	require.NotNil(t, partitionRecord, "unexpectedly got nil partition record")

	signers := map[string]crypto.Signer{}
	var rgr []*protocgenesis.RootGenesis
	for i := 0; i < count; i++ {
		nodeID, signer, _, pubKey := generatePeerData(t)
		rootG, _, err := genesis.NewRootGenesis(nodeID.String(), signer, pubKey, []*protocgenesis.PartitionRecord{partitionRecord}, genesis.WithTotalNodes(uint32(count)), genesis.WithConsensusTimeout(2500))
		require.NoError(t, err, "failed to create root genesis")
		require.NotNil(t, rootG)
		rgr = append(rgr, rootG)
		signers[nodeID.String()] = signer
	}

	rootG, partG, err := genesis.MergeRootGenesisFiles(rgr)
	require.NoError(t, err, "failed to merge root genesis records")
	require.NotNil(t, partG)
	require.NotNil(t, rootG)
	// we need to be able to have quorum while "stopping" one manager
	require.LessOrEqual(t, rootG.Root.Consensus.QuorumThreshold, uint32(count-1), "QuorumThreshold too high")

	nw := newMockNetwork()
	cms := make([]*ConsensusManager, 0, len(rootG.Root.RootValidators))
	for _, v := range rootG.Root.RootValidators {
		nodeID, err := peer.Decode(v.NodeIdentifier)
		require.NoError(t, err)
		pStore, err := partitions.NewPartitionStoreFromGenesis(rootG.Partitions)
		require.NoError(t, err)

		cm, err := NewDistributedAbConsensusManager(nodeID, rootG, pStore, nw.Connect(nodeID), signers[v.NodeIdentifier])
		require.NoError(t, err)
		cms = append(cms, cm)
	}

	return cms, nw
}

func generatePeerData(t *testing.T) (peer.ID, crypto.Signer, crypto.Verifier, []byte) {
	t.Helper()

	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	verifier, err := signer.Verifier()
	require.NoError(t, err)

	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	nodeID, err := network.NodeIDFromPublicKeyBytes(pubKey)
	require.NoError(t, err)

	return nodeID, signer, verifier, pubKey
}

func newMockNetwork() *mockNetwork {
	mnw := &mockNetwork{
		cons:   make(map[peer.ID]chan network.ReceivedMessage),
		sendTO: 70 * time.Millisecond,
	}
	mnw.firewall.Store(fwFunc(nil))
	return mnw
}

type fwFunc func(from, to peer.ID, msg network.OutputMessage) bool

type mockNetwork struct {
	cons   map[peer.ID]chan network.ReceivedMessage
	sendTO time.Duration // timeout for send operation

	// firewall stores func (of type fwFunc) which returns "true" when the message
	// should be blocked and "false" when msg should pass through FW
	firewall atomic.Value

	m    sync.Mutex
	errs []error
}

/*
Connect creates mocked RootNet for given peer, the peer is "connected" to the
same network with other peers in the mockNetwork.
NB! not concurrency safe, register peers before concurrent action!
*/
func (mnw *mockNetwork) Connect(node peer.ID) RootNet {
	con := &mockNwConnection{nw: mnw, id: node, rcv: make(chan network.ReceivedMessage)}
	mnw.cons[node] = con.rcv
	return con
}

/*
SetFirewall replaces current FW func.

When the func returns "true" the message will be blocked, when "false" the msg is passed on.
Use "nil" to disable FW (ie all messages will be passed on without filter).
Message blocked by FW just disappears, no error is returned to the sender.
*/
func (mnw *mockNetwork) SetFirewall(fw fwFunc) {
	mnw.firewall.Store(fw)
}

func (mnw *mockNetwork) logError(err error) {
	mnw.m.Lock()
	defer mnw.m.Unlock()
	mnw.errs = append(mnw.errs, err)
}

func (mnw *mockNetwork) send(from, to peer.ID, msg network.OutputMessage) error {
	con, ok := mnw.cons[to]
	if !ok {
		return fmt.Errorf("unknown receiver %s for message %#v sent by %s", to, msg, from)
	}

	if fw := mnw.firewall.Load().(fwFunc); fw != nil {
		if fw(from, to, msg) {
			return nil
		}
	}

	go func() {
		select {
		case <-time.After(mnw.sendTO):
			mnw.logError(fmt.Errorf("send operation timed out %s -> %s : %#v", from, to, msg))
		case con <- network.ReceivedMessage{From: from, Protocol: msg.Protocol, Message: msg.Message}:
		}
	}()

	return nil
}

func (mnw *mockNetwork) broadcast(from peer.ID, msg network.OutputMessage) error {
	var errs []error
	for id := range mnw.cons {
		if err := mnw.send(from, id, msg); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type mockNwConnection struct {
	nw  *mockNetwork
	id  peer.ID
	rcv chan network.ReceivedMessage
}

func (nc *mockNwConnection) Send(msg network.OutputMessage, receivers []peer.ID) error {
	var errs []error
	for _, id := range receivers {
		if err := nc.nw.send(nc.id, id, msg); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (nc *mockNwConnection) Broadcast(msg network.OutputMessage) error {
	return nc.nw.broadcast(nc.id, msg)
}

func (nc *mockNwConnection) ReceivedChannel() <-chan network.ReceivedMessage { return nc.rcv }

/*
constLeader is leader selection algorithm which always returns the same leader.
*/
type constLeader peer.ID

func (cl constLeader) GetLeaderForRound(round uint64) peer.ID { return peer.ID(cl) }

func (cl constLeader) Update(qc *types.QuorumCert, currentRound uint64) error { return nil }
