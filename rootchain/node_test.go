package rootchain

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"testing"
	"time"

	p2peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testobservability "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/internal/testutils/peer"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	"github.com/alphabill-org/alphabill/rootchain/consensus/storage"
)

func Test_rootNode(t *testing.T) {
	nopObs := testobservability.NOPObservability()

	t.Run("constructor", func(t *testing.T) {
		nwPeer := network.Peer{}
		cm := mockConsensusManager{}
		partNet := mockPartitionNet{}

		node, err := New(nil, partNet, cm, nopObs)
		require.Nil(t, node)
		require.EqualError(t, err, `partition listener is nil`)

		node, err = New(&nwPeer, nil, cm, nopObs)
		require.Nil(t, node)
		require.EqualError(t, err, `network is nil`)

		node, err = New(&nwPeer, partNet, cm, nopObs)
		require.NoError(t, err)
		require.NotNil(t, node)
		require.Equal(t, &nwPeer, node.GetPeer())
	})

	t.Run("Run, context cancel", func(t *testing.T) {
		nwPeer := peer.CreatePeer(t, peer.CreatePeerConfiguration(t))
		cm := mockConsensusManager{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
		}

		partMsg := make(chan any)
		partNet := mockPartitionNet{recCh: func() <-chan any { return partMsg }}

		node, err := New(nwPeer, partNet, cm, nopObs)
		require.NoError(t, err)
		require.NotNil(t, node)
		require.Equal(t, nwPeer, node.GetPeer())

		done := make(chan struct{})
		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			defer close(done)
			require.ErrorIs(t, node.Run(ctx), context.Canceled)
		}()

		// wait until partition network message is accepted
		partMsg <- "it's running"
		// cancel the ctx, Run should exit
		cancel()

		select {
		case <-done:
		case <-time.After(1000 * time.Millisecond):
			t.Error("not done within timeout")
		}
	})

	t.Run("Run, CM exits with error", func(t *testing.T) {
		nwPeer := peer.CreatePeer(t, peer.CreatePeerConfiguration(t))
		cmResult := make(chan error)
		cm := mockConsensusManager{
			run: func(ctx context.Context) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case err := <-cmResult:
					return err
				}
			},
		}

		partNet, _ := newMockPartitionNet()

		node, err := New(nwPeer, partNet, cm, nopObs)
		require.NoError(t, err)
		require.NotNil(t, node)
		require.Equal(t, nwPeer, node.GetPeer())

		expErr := fmt.Errorf("something wrong in CM")
		done := make(chan struct{})
		go func() {
			defer close(done)
			require.ErrorIs(t, node.Run(t.Context()), expErr)
		}()

		// trigger error in the Consensus Manager's Run
		cmResult <- expErr

		select {
		case <-done:
		case <-time.After(1000 * time.Millisecond):
			t.Error("not done within timeout")
		}
	})
}

func Test_sendResponse(t *testing.T) {
	// node constructor requires non nil arguments but the tests
	// actually do not depend on peer and ConsensusManager.
	// The sendResponse method only depends on partitionNet.Send
	nwPeer := network.Peer{}
	cm := mockConsensusManager{}
	nopObs := testobservability.NOPObservability()

	nodeID := generateNodeID(t).String()
	certResp := validCertificationResponse(t)

	t.Run("invalid peer ID", func(t *testing.T) {
		node, err := New(&nwPeer, mockPartitionNet{}, cm, nopObs)
		require.NoError(t, err)

		err = node.sendResponse(t.Context(), "", &certResp)
		require.EqualError(t, err, `invalid receiver id: failed to parse peer ID: invalid cid: cid too short`)

		err = node.sendResponse(t.Context(), "not valid node ID", &certResp)
		require.EqualError(t, err, `invalid receiver id: failed to parse peer ID: invalid cid: selected encoding not supported`)
	})

	t.Run("invalid CertificationResponse", func(t *testing.T) {
		// CertResp is coming from ConsensusManager so this should be impossible?
		// just send it out and shard nodes should be able to ignore invalid CRsp?
		node, err := New(&nwPeer, mockPartitionNet{}, cm, nopObs)
		require.NoError(t, err)

		cr := certResp
		cr.Partition = 0
		err = node.sendResponse(t.Context(), nodeID, &cr)
		require.EqualError(t, err, `invalid certification response: partition ID is unassigned`)
	})

	t.Run("send fails", func(t *testing.T) {
		expErr := fmt.Errorf("not sending")
		partNet := mockPartitionNet{
			send: func(ctx context.Context, msg any, receivers ...p2peer.ID) error {
				return expErr
			},
		}
		node, err := New(&nwPeer, partNet, cm, nopObs)
		require.NoError(t, err)
		err = node.sendResponse(t.Context(), nodeID, &certResp)
		require.ErrorIs(t, err, expErr)
	})

	t.Run("success", func(t *testing.T) {
		partNet := mockPartitionNet{
			send: func(ctx context.Context, msg any, receivers ...p2peer.ID) error {
				require.Equal(t, &certResp, msg)
				require.Len(t, receivers, 1)
				require.Equal(t, nodeID, receivers[0].String())
				return nil
			},
		}
		node, err := New(&nwPeer, partNet, cm, nopObs)
		require.NoError(t, err)
		require.NoError(t, node.sendResponse(t.Context(), nodeID, &certResp))
	})
}

func Test_onHandshake(t *testing.T) {
	// node constructor requires non nil peer but the tests
	// actually do not depend on valid peer
	// depends on consensusManager.ShardInfo and partitionNet.Send
	nwPeer := network.Peer{}
	nodeID := generateNodeID(t)
	certResp := validCertificationResponse(t)
	nopObs := testobservability.NOPObservability()
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	verifier, err := signer.Verifier()
	require.NoError(t, err)
	publicKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	t.Run("invalid handshake msg", func(t *testing.T) {
		partNet := mockPartitionNet{}
		cm := mockConsensusManager{}
		node, err := New(&nwPeer, partNet, cm, nopObs)
		require.NoError(t, err)
		msg := handshake.Handshake{
			PartitionID: 0, // invalid partition ID
			ShardID:     certResp.Shard,
			NodeID:      nodeID.String(),
		}
		err = node.onHandshake(t.Context(), &msg)
		require.EqualError(t, err, `invalid handshake request: invalid partition identifier`)
	})

	t.Run("no ShardInfo", func(t *testing.T) {
		partNet := mockPartitionNet{}
		msg := handshake.Handshake{
			PartitionID: certResp.Partition + 1,
			ShardID:     certResp.Shard,
			NodeID:      nodeID.String(),
		}
		expErr := fmt.Errorf("nope, no such shard")
		cm := mockConsensusManager{
			shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
				require.EqualValues(t, msg.PartitionID, partition)
				require.EqualValues(t, msg.ShardID, shard)
				return nil, expErr
			},
		}
		node, err := New(&nwPeer, partNet, cm, nopObs)
		require.NoError(t, err)
		err = node.onHandshake(t.Context(), &msg)
		require.EqualError(t, err, fmt.Errorf(`reading partition %s certificate: %w`, msg.PartitionID, expErr).Error())
	})

	t.Run("failure to send response", func(t *testing.T) {
		partNet := mockPartitionNet{
			send: func(ctx context.Context, msg any, receivers ...p2peer.ID) error {
				return fmt.Errorf("not sending")
			},
		}
		cm := mockConsensusManager{
			shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
				return newMockShardInfo(t, nodeID.String(), publicKey, certResp), nil
			},
		}
		node, err := New(&nwPeer, partNet, cm, nopObs)
		require.NoError(t, err)

		msg := handshake.Handshake{
			PartitionID: certResp.Partition,
			ShardID:     certResp.Shard,
			NodeID:      nodeID.String(),
		}
		err = node.onHandshake(t.Context(), &msg)
		require.EqualError(t, err, `failed to send response: not sending`)
	})

	t.Run("success", func(t *testing.T) {
		partNet := mockPartitionNet{
			send: func(ctx context.Context, msg any, receivers ...p2peer.ID) error {
				require.Equal(t, &certResp, msg)
				require.Len(t, receivers, 1)
				require.Equal(t, nodeID, receivers[0])
				return nil
			},
		}
		cm := mockConsensusManager{
			shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
				return newMockShardInfo(t, nodeID.String(), publicKey, certResp), nil
			},
		}
		node, err := New(&nwPeer, partNet, cm, nopObs)
		require.NoError(t, err)

		msg := handshake.Handshake{
			PartitionID: certResp.Partition,
			ShardID:     certResp.Shard,
			NodeID:      nodeID.String(),
		}
		require.NoError(t, node.onHandshake(t.Context(), &msg))
	})
}

func Test_handlePartitionMsg(t *testing.T) {
	nopObs := testobservability.NOPObservability()
	nwPeer := network.Peer{}
	nodeID := generateNodeID(t)
	certResp := validCertificationResponse(t)
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	verifier, err := signer.Verifier()
	require.NoError(t, err)
	publicKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	t.Run("unsupported message", func(t *testing.T) {
		partNet := mockPartitionNet{}
		cm := mockConsensusManager{}
		node, err := New(&nwPeer, partNet, cm, nopObs)
		require.NoError(t, err)
		err = node.handlePartitionMsg(t.Context(), 555)
		require.EqualError(t, err, `unknown message type int`)
	})

	t.Run("handshake", func(t *testing.T) {
		sendCalled := 0
		partNet := mockPartitionNet{
			send: func(ctx context.Context, msg any, receivers ...p2peer.ID) error {
				require.Equal(t, &certResp, msg)
				require.Len(t, receivers, 1)
				require.Equal(t, nodeID, receivers[0])
				sendCalled++
				return nil
			},
		}
		cm := mockConsensusManager{
			shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
				return newMockShardInfo(t, nodeID.String(), publicKey, certResp), nil
			},
		}
		node, err := New(&nwPeer, partNet, cm, nopObs)
		require.NoError(t, err)

		msg := handshake.Handshake{
			PartitionID: certResp.Partition,
			ShardID:     certResp.Shard,
			NodeID:      nodeID.String(),
		}
		require.NoError(t, node.handlePartitionMsg(t.Context(), &msg))
		require.EqualValues(t, 1, sendCalled)
	})

	t.Run("Certification Request", func(t *testing.T) {
		// calls onBlockCertificationRequest with the message and one of
		// the first things there ShardInfo is requested from CM.
		// Return known error so we know expected calls did happen...
		partNet := mockPartitionNet{}
		expErr := errors.New("nope")
		cm := mockConsensusManager{
			shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
				return nil, expErr
			},
		}
		node, err := New(&nwPeer, partNet, cm, nopObs)
		require.NoError(t, err)

		msg := certification.BlockCertificationRequest{
			PartitionID: certResp.Partition,
			ShardID:     certResp.Shard,
			NodeID:      nodeID.String(),
		}
		require.ErrorIs(t, node.handlePartitionMsg(t.Context(), &msg), expErr)
	})
}

func Test_partitionMsgLoop(t *testing.T) {
	nwPeer := network.Peer{}

	t.Run("cancel ctx", func(t *testing.T) {
		partNet, _ := newMockPartitionNet()
		cm := mockConsensusManager{}

		done := make(chan struct{})
		node, err := New(&nwPeer, partNet, cm, testobservability.Default(t))
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			defer close(done)
			require.ErrorIs(t, node.partitionMsgLoop(ctx), context.Canceled)
		}()

		time.AfterFunc(100*time.Millisecond, cancel)
		select {
		case <-done:
		case <-time.After(1000 * time.Millisecond):
			t.Error("msg loop didn't quit within timeout")
		}
	})

	t.Run("msg chan closed", func(t *testing.T) {
		partNet, partMsg := newMockPartitionNet()
		cm := mockConsensusManager{}

		done := make(chan struct{})
		node, err := New(&nwPeer, partNet, cm, testobservability.Default(t))
		require.NoError(t, err)

		go func() {
			defer close(done)
			require.EqualError(t, node.partitionMsgLoop(t.Context()), `partition channel closed`)
		}()

		close(partMsg)
		select {
		case <-done:
		case <-time.After(1000 * time.Millisecond):
			t.Error("msg loop didn't quit within timeout")
		}
	})

	t.Run("unsupported message", func(t *testing.T) {
		// could check that warning was logged but we just make sure it doesn't crash...
		partMsg := make(chan any)
		partNet := mockPartitionNet{
			recCh: func() <-chan any { return partMsg },
		}
		cm := mockConsensusManager{}

		done := make(chan struct{})
		node, err := New(&nwPeer, partNet, cm, testobservability.Default(t))
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			defer close(done)
			require.ErrorIs(t, node.partitionMsgLoop(ctx), context.Canceled)
		}()

		partMsg <- "not a valid message"

		cancel()
		select {
		case <-done:
		case <-time.After(1000 * time.Millisecond):
			t.Error("msg loop didn't quit within timeout")
		}
	})
}

func Test_onBlockCertificationRequest(t *testing.T) {
	// network peer and partition network are not used by the test
	// but we need non nil values for the constructor
	nwPeer := network.Peer{}
	partNet := mockPartitionNet{}

	nodeID := generateNodeID(t).String()
	nodeID2 := generateNodeID(t).String()

	// create "valid looking" (ie satisfy the checks performed by the tests here) data
	shardConf := &types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   5,
		PartitionID: 1,
		T2Timeout:   2000 * time.Millisecond,
	}
	partitionIR := types.InputRecord{
		Version:      1,
		PreviousHash: test.RandomBytes(32),
		Hash:         []byte{0, 0, 0, 1},
		BlockHash:    []byte{0, 0, 1, 2},
		SummaryValue: []byte{0, 0, 1, 3},
		RoundNumber:  3864,
		Epoch:        2,
		Timestamp:    types.NewTimestamp(),
	}

	// valid certification response
	certResp := certification.CertificationResponse{
		Partition: shardConf.PartitionID,
		Shard:     types.ShardID{},
		UC: types.UnicityCertificate{
			InputRecord: &types.InputRecord{
				Hash:      test.RandomBytes(32),
				BlockHash: test.RandomBytes(32),
			},
			UnicityTreeCertificate: &types.UnicityTreeCertificate{
				Version:   1,
				Partition: shardConf.PartitionID,
			},
			UnicitySeal: &types.UnicitySeal{
				Timestamp: types.NewTimestamp(),
			},
		},
	}
	require.NoError(t,
		certResp.SetTechnicalRecord(certification.TechnicalRecord{
			Round:    partitionIR.RoundNumber,
			Epoch:    partitionIR.Epoch,
			Leader:   nodeID,
			StatHash: []byte("state hash"),
			FeeHash:  []byte("fee hash"),
		}))
	require.NoError(t, certResp.IsValid())

	// create ShardInfo for the shard
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	sigKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	shardConf.Validators = []*types.NodeInfo{
		{NodeID: nodeID, SigKey: sigKey},
		{NodeID: nodeID2, SigKey: sigKey},
	}
	si, err := storage.NewShardInfo(shardConf, crypto.SHA256)
	require.NoError(t, err)
	si.LastCR = &certResp
	require.NoError(t, si.IsValid())

	// certification request which is valid for the ShardInfo
	validCertRequest := certification.BlockCertificationRequest{
		PartitionID: shardConf.PartitionID,
		ShardID:     types.ShardID{},
		NodeID:      nodeID,
		InputRecord: partitionIR.NewRepeatIR(),
		BlockSize:   10,
		StateSize:   1024,
	}
	validCertRequest.InputRecord.PreviousHash = si.RootHash
	validCertRequest.InputRecord.Hash = test.RandomBytes(32)
	require.NoError(t, validCertRequest.Sign(signer))
	require.NoError(t, si.ValidRequest(&validCertRequest))

	t.Run("invalid request - no ShardInfo", func(t *testing.T) {
		cm := mockConsensusManager{
			shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
				require.Equal(t, validCertRequest.PartitionID, partition)
				require.Equal(t, validCertRequest.ShardID, shard)
				return nil, errors.New("no SI")
			},
		}

		node, err := New(&nwPeer, partNet, cm, testobservability.Default(t))
		require.NoError(t, err)
		err = node.onBlockCertificationRequest(t.Context(), &validCertRequest)
		require.EqualError(t, err, `acquiring shard 00000001 -  info: no SI`)
	})

	t.Run("invalid request - invalid NodeID", func(t *testing.T) {
		cm := mockConsensusManager{
			shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
				// just return non nil SI - before the SI is used the request node
				// is subscribed for responses and that's where we expect failure as
				// the node ID is not valid (invalid encoding)
				return &storage.ShardInfo{}, nil
			},
		}

		node, err := New(&nwPeer, partNet, cm, testobservability.Default(t))
		require.NoError(t, err)
		cr := validCertRequest
		cr.NodeID = "not valid ID"
		err = node.onBlockCertificationRequest(t.Context(), &cr)
		require.EqualError(t, err, `subscribing the sender: invalid receiver id: failed to parse peer ID: invalid cid: selected encoding not supported`)
	})

	t.Run("invalid request", func(t *testing.T) {
		// in case of invalid request we respond with the latest cert of the shard
		sendCallCnt := 0
		partNet := mockPartitionNet{
			send: func(ctx context.Context, msg any, receivers ...p2peer.ID) error {
				require.Equal(t, &certResp, msg)
				sendCallCnt++
				return nil
			},
		}

		/*** case 1: request from unknown node ***/

		cm := &mockConsensusManager{
			shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
				// return SI which has empty TrustBase so the request is invalid (unknown node)
				// however, we still send response with current certificate!
				// should we change it so that when the node is not in the TB we just ignore the request?
				return &storage.ShardInfo{LastCR: &certResp}, nil
			},
		}
		node, err := New(&nwPeer, partNet, cm, testobservability.Default(t))
		require.NoError(t, err)

		err = node.onBlockCertificationRequest(t.Context(), &validCertRequest)
		require.EqualError(t, err, `invalid block certification request: invalid certification request: node "`+nodeID+`" is not in the trustbase of the shard`)
		require.EqualValues(t, 1, sendCallCnt)

		/*** case 2: invalid request from a node in the trustbase ***/

		// return SI where the node is in the TB
		cm.shardInfo = func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
			return si, nil
		}
		// invalidate CertRequest - any change invalidates the signature
		cr := validCertRequest
		cr.BlockSize++
		err = node.onBlockCertificationRequest(t.Context(), &cr)
		require.EqualError(t, err, `invalid block certification request: invalid certification request: signature verification: verification failed`)
		require.EqualValues(t, 2, sendCallCnt, "expected that the latest Cert is sent to the node")
	})

	t.Run("Equivocating Request", func(t *testing.T) {
		cm := &mockConsensusManager{
			shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
				return si, nil
			},
		}
		node, err := New(&nwPeer, partNet, cm, testobservability.Default(t))
		require.NoError(t, err)

		err = node.onBlockCertificationRequest(t.Context(), &validCertRequest)
		require.NoError(t, err)
		// second request from the same node - as two votes is needed shard should be
		// in the state "in progress" and another request from the same node must return error
		err = node.onBlockCertificationRequest(t.Context(), &validCertRequest)
		require.EqualError(t, err, `storing request: request of the node in this round already stored`)
	})

	t.Run("failure to RequestCertification", func(t *testing.T) {
		expErr := errors.New("CM out of order")
		cm := &mockConsensusManager{
			shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
				return si, nil
			},
			requestCert: func(ctx context.Context, cr consensus.IRChangeRequest) error {
				return expErr
			},
		}
		key := partitionShard{validCertRequest.PartitionID, validCertRequest.ShardID.Key()}

		node, err := New(&nwPeer, partNet, cm, testobservability.Default(t))
		require.NoError(t, err)
		require.NotContains(t, node.incomingRequests.store, key)

		// the quorum is two votes so we now should have "quorum in progress" state for the shard
		err = node.onBlockCertificationRequest(t.Context(), &validCertRequest)
		require.NoError(t, err)
		if assert.Contains(t, node.incomingRequests.store, key) {
			irs := node.incomingRequests.store[key]
			require.NotEmpty(t, irs.nodeRequest)
			require.NotEmpty(t, irs.requests)
		}

		// add valid request from another node, should reach consensus...
		cr := validCertRequest
		cr.NodeID = nodeID2
		require.NoError(t, cr.Sign(signer))
		// ...but CM returns error
		err = node.onBlockCertificationRequest(t.Context(), &cr)
		require.ErrorIs(t, err, expErr)
		// failure to send to CM should clear the Incoming Request Buffer
		if assert.Contains(t, node.incomingRequests.store, key) {
			irs := node.incomingRequests.store[key]
			require.Empty(t, irs.nodeRequest)
			require.Empty(t, irs.requests)
		}
	})

	t.Run("success, quorum", func(t *testing.T) {
		certReqCalls := 0
		cm := &mockConsensusManager{
			shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
				return si, nil
			},
			requestCert: func(ctx context.Context, cr consensus.IRChangeRequest) error {
				require.Equal(t, validCertRequest.PartitionID, cr.Partition)
				require.Equal(t, validCertRequest.ShardID, cr.Shard)
				require.Equal(t, consensus.Quorum, cr.Reason)
				certReqCalls++
				return nil
			},
		}
		key := partitionShard{validCertRequest.PartitionID, validCertRequest.ShardID.Key()}

		node, err := New(&nwPeer, partNet, cm, testobservability.Default(t))
		require.NoError(t, err)
		require.NotContains(t, node.incomingRequests.store, key)

		// the quorum is two votes so we now should have "quorum in progress" state for the shard
		err = node.onBlockCertificationRequest(t.Context(), &validCertRequest)
		require.NoError(t, err)
		require.EqualValues(t, 0, certReqCalls, "unexpected request to certify")
		if assert.Contains(t, node.incomingRequests.store, key) {
			irs := node.incomingRequests.store[key]
			require.Len(t, irs.nodeRequest, 1)
			require.Len(t, irs.requests, 1)
		}

		// second request
		cr := validCertRequest
		cr.NodeID = nodeID2
		require.NoError(t, cr.Sign(signer))
		err = node.onBlockCertificationRequest(t.Context(), &cr)
		require.NoError(t, err)
		require.EqualValues(t, 1, certReqCalls, "expected request to be sent to CM")
		// the Request Buffer should still have data for the shard - will be cleared when CResp is sent
		if assert.Contains(t, node.incomingRequests.store, key) {
			irs := node.incomingRequests.store[key]
			assert.Len(t, irs.nodeRequest, 2)
			assert.Len(t, irs.requests, 1)
		}

		// send one more request - but quorum is already achieved
		// should log following DBG message and return nil error
		// dropping stale block certification request (<status>) for partition %s
		err = node.onBlockCertificationRequest(t.Context(), &cr)
		require.NoError(t, err)
		require.EqualValues(t, 1, certReqCalls, "unexpected request to certify when quorum is already achieved")
		if assert.Contains(t, node.incomingRequests.store, key) {
			irs := node.incomingRequests.store[key]
			assert.Len(t, irs.nodeRequest, 2)
			assert.Len(t, irs.requests, 1)
		}
	})

	t.Run("success, no quorum", func(t *testing.T) {
		certReqCalls := 0
		cm := &mockConsensusManager{
			shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
				return si, nil
			},
			requestCert: func(ctx context.Context, cr consensus.IRChangeRequest) error {
				require.Equal(t, validCertRequest.PartitionID, cr.Partition)
				require.Equal(t, validCertRequest.ShardID, cr.Shard)
				require.Equal(t, consensus.QuorumNotPossible, cr.Reason, "certification reason")
				certReqCalls++
				return nil
			},
		}
		key := partitionShard{validCertRequest.PartitionID, validCertRequest.ShardID.Key()}

		node, err := New(&nwPeer, partNet, cm, testobservability.Default(t))
		require.NoError(t, err)
		require.NotContains(t, node.incomingRequests.store, key)

		// the quorum is two votes so we now should have "quorum in progress" state for the shard
		err = node.onBlockCertificationRequest(t.Context(), &validCertRequest)
		require.NoError(t, err)
		require.EqualValues(t, 0, certReqCalls, "unexpected request to certify")
		if assert.Contains(t, node.incomingRequests.store, key) {
			irs := node.incomingRequests.store[key]
			assert.Len(t, irs.nodeRequest, 1)
			assert.Len(t, irs.requests, 1)
			assert.Equal(t, QuorumInProgress, irs.qState)
		}

		// send different cert request, should make it impossible to get quorum
		cr := validCertRequest
		cr.BlockSize++
		cr.NodeID = nodeID2
		require.NoError(t, cr.Sign(signer))
		err = node.onBlockCertificationRequest(t.Context(), &cr)
		require.NoError(t, err)
		require.EqualValues(t, 1, certReqCalls, "expected request to certify")
		if assert.Contains(t, node.incomingRequests.store, key) {
			irs := node.incomingRequests.store[key]
			assert.Len(t, irs.nodeRequest, 2)
			assert.Len(t, irs.requests, 2)
			assert.Equal(t, QuorumNotPossible, irs.qState)
		}
	})
}

func Test_handleConsensus(t *testing.T) {
	nwPeer := network.Peer{}
	partNet := mockPartitionNet{}

	t.Run("context cancelled", func(t *testing.T) {
		cm := mockConsensusManager{}

		done := make(chan struct{})
		node, err := New(&nwPeer, partNet, cm, testobservability.Default(t))
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			defer close(done)
			require.ErrorIs(t, node.handleConsensus(ctx), context.Canceled)
		}()

		time.AfterFunc(100*time.Millisecond, cancel)
		select {
		case <-done:
		case <-time.After(1000 * time.Millisecond):
			t.Error("msg loop didn't quit within timeout")
		}
	})

	t.Run("message chan closed", func(t *testing.T) {
		cm := mockConsensusManager{certificationResult: make(chan *certification.CertificationResponse)}

		done := make(chan struct{})
		node, err := New(&nwPeer, partNet, cm, testobservability.Default(t))
		require.NoError(t, err)

		go func() {
			defer close(done)
			require.EqualError(t, node.handleConsensus(t.Context()), `consensus channel closed`)
		}()

		close(cm.certificationResult)
		select {
		case <-done:
		case <-time.After(1000 * time.Millisecond):
			t.Error("msg loop didn't quit within timeout")
		}
	})

	t.Run("success", func(t *testing.T) {
		nodeID := generateNodeID(t).String()
		cr := validCertificationResponse(t)

		cm := mockConsensusManager{certificationResult: make(chan *certification.CertificationResponse)}

		rspSent := make(chan struct{})
		partNet := mockPartitionNet{
			send: func(ctx context.Context, msg any, receivers ...p2peer.ID) error {
				defer close(rspSent)
				require.Equal(t, &cr, msg)
				return nil
			},
		}

		node, err := New(&nwPeer, partNet, cm, testobservability.Default(t))
		require.NoError(t, err)

		go func() {
			require.ErrorIs(t, node.handleConsensus(t.Context()), context.Canceled)
		}()

		// normally node gets subscribed by sending Certification Request, here we
		// use shortcut to subscribe directly so that the partitionNet.Send gets called
		require.NoError(t, node.subscription.Subscribe(cr.Partition, cr.Shard, nodeID))

		// manually add the node into request buffer so that we can check was it reset
		req := certification.BlockCertificationRequest{
			PartitionID: cr.Partition,
			ShardID:     cr.Shard,
			NodeID:      nodeID,
			InputRecord: &types.InputRecord{},
		}
		tb := mockQuorumInfo{nodeCount: 1, quorum: 1}
		qs, _, err := node.incomingRequests.Add(t.Context(), &req, tb)
		require.NoError(t, err)
		require.Equal(t, QuorumAchieved, qs)

		// trigger the consensus handler (ie the Consensus Manager sends Cert Response)
		cm.certificationResult <- &cr

		select {
		case <-rspSent:
			// check that the incoming requests status has been reset
			qs = node.incomingRequests.IsConsensusReceived(cr.Partition, cr.Shard, tb)
			require.Equal(t, QuorumInProgress, qs)
		case <-time.After(1000 * time.Millisecond):
			t.Error("msg loop didn't quit within timeout")
		}
	})
}

type mockQuorumInfo struct {
	nodeCount, quorum uint64
}

func (qi mockQuorumInfo) GetQuorum() uint64     { return qi.quorum }
func (qi mockQuorumInfo) GetTotalNodes() uint64 { return qi.nodeCount }

func newMockPartitionNet() (mockPartitionNet, chan any) {
	nwc := make(chan any, 1)
	return mockPartitionNet{
		recCh: func() <-chan any { return nwc },
	}, nwc
}

type mockPartitionNet struct {
	send  func(ctx context.Context, msg any, receivers ...p2peer.ID) error
	recCh func() <-chan any
}

func (pn mockPartitionNet) Send(ctx context.Context, msg any, receivers ...p2peer.ID) error {
	return pn.send(ctx, msg, receivers...)
}

func (pn mockPartitionNet) ReceivedChannel() <-chan any { return pn.recCh() }

type mockConsensusManager struct {
	certificationResult chan *certification.CertificationResponse
	shardInfo           func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error)
	requestCert         func(ctx context.Context, cr consensus.IRChangeRequest) error
	run                 func(ctx context.Context) error
}

func (cm mockConsensusManager) ShardInfo(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
	return cm.shardInfo(partition, shard)
}

func (cm mockConsensusManager) CertificationResult() <-chan *certification.CertificationResponse {
	return cm.certificationResult
}

func (cm mockConsensusManager) RequestCertification(ctx context.Context, cr consensus.IRChangeRequest) error {
	return cm.requestCert(ctx, cr)
}

func (cm mockConsensusManager) Run(ctx context.Context) error { return cm.run(ctx) }

// creates "valid looking" certification response
func validCertificationResponse(t *testing.T) certification.CertificationResponse {
	certResp := certification.CertificationResponse{
		Partition: 8,
		Shard:     types.ShardID{},
		UC: types.UnicityCertificate{
			InputRecord: &types.InputRecord{
				Hash:      test.RandomBytes(32),
				BlockHash: test.RandomBytes(32),
			},
			UnicityTreeCertificate: &types.UnicityTreeCertificate{
				Version:   1,
				Partition: 8,
			},
		},
	}
	require.NoError(t,
		certResp.SetTechnicalRecord(certification.TechnicalRecord{
			Round:    99,
			Epoch:    8,
			Leader:   "1",
			StatHash: []byte{1},
			FeeHash:  []byte{2},
		}))
	require.NoError(t, certResp.IsValid())
	return certResp
}

func newMockShardInfo(t *testing.T, nodeID string, nodeSigningPubKey []byte, certResp certification.CertificationResponse) *storage.ShardInfo {
	shardConf := &types.PartitionDescriptionRecord{
		Validators: []*types.NodeInfo{{NodeID: nodeID, SigKey: nodeSigningPubKey}},
	}
	si, err := storage.NewShardInfo(shardConf, crypto.SHA256)
	require.NoError(t, err)
	si.LastCR = &certResp
	return si
}
