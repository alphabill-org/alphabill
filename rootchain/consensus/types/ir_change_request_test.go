package types

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/stretchr/testify/require"
)

const sysId1 types.PartitionID = 1
const sysId2 types.PartitionID = 2

var prevHash = []byte{0, 0, 0}
var inputRecord1 = &types.InputRecord{Version: 1,
	PreviousHash: prevHash,
	Hash:         []byte{1, 1, 1},
	BlockHash:    []byte{2, 2, 2},
	SummaryValue: []byte{4, 4, 4},
	RoundNumber:  2,
}
var inputRecord2 = &types.InputRecord{Version: 1,
	PreviousHash: prevHash,
	Hash:         []byte{5, 5, 5},
	BlockHash:    []byte{2, 2, 2},
	SummaryValue: []byte{4, 4, 4},
	RoundNumber:  2,
}

func TestIRChangeReqMsg_IsValid(t *testing.T) {
	t.Run("IR change req. invalid cert reason", func(t *testing.T) {
		requests := []*certification.BlockCertificationRequest{
			{
				Partition:      2,
				NodeIdentifier: "1",
				InputRecord:    inputRecord1,
				Signature:      []byte{0, 1},
			},
		}
		x := &IRChangeReq{
			Partition:  1,
			CertReason: 20,
			Requests:   requests,
		}
		require.EqualError(t, x.IsValid(), "unknown reason (20)")
	})
	t.Run("ok", func(t *testing.T) {
		x := &IRChangeReq{
			Partition:  1,
			CertReason: T2Timeout,
			Requests:   nil,
		}
		require.NoError(t, x.IsValid())
	})
}

func TestIRChangeReqMsg_BytesHash(t *testing.T) {
	ircr := &IRChangeReq{
		Partition:  sysId1,
		CertReason: QuorumNotPossible,
		Requests: []*certification.BlockCertificationRequest{
			{
				Partition:      1,
				Shard:          types.ShardID{},
				NodeIdentifier: "1",
				InputRecord: &types.InputRecord{Version: 1,
					PreviousHash: []byte{0, 1},
					Hash:         []byte{2, 3},
					BlockHash:    []byte{4, 5},
					SummaryValue: []byte{6, 7},
					RoundNumber:  3,
				},
				RootRoundNumber: 2,
				BlockSize:       4,
				StateSize:       5,
				Signature:       []byte{0, 1},
			},
		},
	}
	irHasher := gocrypto.SHA256.New()
	irHasher.Write(ircr.Bytes())

	expectedHash := gocrypto.SHA256.New()
	expectedHash.Write([]byte{
		0, 0, 0, 1, // 4 byte System identifier of IRChangeReqMsg
		0, 0, 0, 1, // cert reason quorum not possible
		// Start of the BlockCertificationRequest
		0, 0, 0, 1, // 4 byte partition identifier
		128,                                            // empty shard ID
		'1',                                            // node identifier - string is encoded without '/0'
		0, 1, 2, 3, 4, 5, 6, 7, 0, 0, 0, 0, 0, 0, 0, 3, // prev. hash, hash, block hash, summary value, round number
		0, 0, 0, 0, 0, 0, 0, 2, //root round number
		0, 0, 0, 0, 0, 0, 0, 4, // block size
		0, 0, 0, 0, 0, 0, 0, 5, // state size
		// Is serialized without signature
	})
	require.Equal(t, expectedHash.Sum(nil), irHasher.Sum(nil))
}

func TestIRChangeReqMsg_GeneralReq(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	tb := &partitions.TrustBase{
		PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2},
	}
	t.Run("trust base is nil", func(t *testing.T) {
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
		}
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{},
		}
		ir, err := x.Verify(nil, luc, 0, 0)
		require.EqualError(t, err, "trust base is nil")
		require.Nil(t, ir)
	})
	t.Run("unknown reason", func(t *testing.T) {
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
		}
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: 16,
			Requests:   []*certification.BlockCertificationRequest{},
		}
		ir, err := x.Verify(tb, luc, 0, 0)
		require.EqualError(t, err, "ir change request validation failed: unknown reason (16)")
		require.Nil(t, ir)
	})
	t.Run("partition id in request and proof do not match", func(t *testing.T) {
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
		}
		reqS1InvalidSysId := &certification.BlockCertificationRequest{
			Partition:      sysId2,
			NodeIdentifier: "1",
			InputRecord:    inputRecord1,
		}
		require.NoError(t, reqS1InvalidSysId.Sign(s1))
		reqS2 := &certification.BlockCertificationRequest{
			Partition:      sysId1,
			NodeIdentifier: "2",
			InputRecord:    inputRecord1,
		}
		require.NoError(t, reqS2.Sign(s2))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{reqS1InvalidSysId, reqS2},
		}
		ir, err := x.Verify(tb, luc, 0, 0)
		require.EqualError(t, err, "invalid partition 00000001 proof: node 1 request partition id 00000002 does not match request")
		require.Nil(t, ir)
	})
	t.Run("IR change request, signature verification error", func(t *testing.T) {
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
		}
		invalidReq := &certification.BlockCertificationRequest{
			Partition:      sysId1,
			NodeIdentifier: "1",
			InputRecord:    inputRecord1,
			Signature:      []byte{0, 19, 12},
		}
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{invalidReq},
		}
		ir, err := x.Verify(tb, luc, 0, 0)
		require.EqualError(t, err, "request proof from partition 00000001 node 1 is not valid: signature verification failed")
		require.Nil(t, ir)
	})
	t.Run("Proof contains more request, than there are partition nodes", func(t *testing.T) {
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
		}
		reqS1 := &certification.BlockCertificationRequest{
			Partition:      sysId1,
			NodeIdentifier: "1",
			InputRecord:    inputRecord1,
		}
		require.NoError(t, reqS1.Sign(s1))
		reqS2 := &certification.BlockCertificationRequest{
			Partition:      sysId1,
			NodeIdentifier: "2",
			InputRecord:    inputRecord1,
		}
		require.NoError(t, reqS2.Sign(s2))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{reqS1, reqS2},
		}
		trustBase := &partitions.TrustBase{
			PartitionTrustBase: map[string]crypto.Verifier{"1": v1},
		}
		ir, err := x.Verify(trustBase, luc, 0, 0)
		require.EqualError(t, err, "proof contains more requests than registered partition nodes")
		require.Nil(t, ir)
	})
	t.Run("Proof contains more request from unknown node", func(t *testing.T) {
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
		}
		reqS1 := &certification.BlockCertificationRequest{
			Partition:      sysId1,
			NodeIdentifier: "1",
			InputRecord:    inputRecord1,
		}
		require.NoError(t, reqS1.Sign(s1))
		reqS2 := &certification.BlockCertificationRequest{
			Partition:      sysId1,
			NodeIdentifier: "2",
			InputRecord:    inputRecord1,
		}
		require.NoError(t, reqS2.Sign(s2))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{reqS1, reqS2},
		}
		trustBase := &partitions.TrustBase{
			PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "3": v2},
		}
		ir, err := x.Verify(trustBase, luc, 0, 0)
		require.EqualError(t, err, "request proof from partition 00000001 node 2 is not valid: node 2 is not part of partition trustbase")
		require.Nil(t, ir)
	})
	t.Run("Proof contains duplicate node", func(t *testing.T) {
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
		}
		reqS1 := &certification.BlockCertificationRequest{
			Partition:      sysId1,
			NodeIdentifier: "1",
			InputRecord:    inputRecord1,
		}
		require.NoError(t, reqS1.Sign(s1))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{reqS1, reqS1},
		}
		trustBase := &partitions.TrustBase{
			PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2},
		}
		ir, err := x.Verify(trustBase, luc, 0, 0)
		require.EqualError(t, err, "invalid partition 00000001 proof: contains duplicate request from node 1")
		require.Nil(t, ir)
	})
	t.Run("IR does not extend last certified state", func(t *testing.T) {
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: []byte{0, 0, 1}, RoundNumber: 1},
		}
		reqS1 := &certification.BlockCertificationRequest{
			Partition:      sysId1,
			NodeIdentifier: "1",
			InputRecord:    inputRecord1,
		}
		require.NoError(t, reqS1.Sign(s1))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{reqS1, reqS1},
		}
		trustBase := &partitions.TrustBase{
			PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2},
		}
		ir, err := x.Verify(trustBase, luc, 0, 0)
		require.EqualError(t, err, "invalid certification request: request extends unknown state: expected hash: [0 0 1], got: [0 0 0]")
		require.Nil(t, ir)
	})
}

func TestIRChangeReqMsg_VerifyTimeoutReq(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	trustBase := &partitions.TrustBase{
		PartitionTrustBase: map[string]crypto.Verifier{"1": v1},
	}
	t.Run("IR change request in timeout proof contains requests, not valid", func(t *testing.T) {
		reqS1 := &certification.BlockCertificationRequest{
			Partition:       sysId1,
			NodeIdentifier:  "1",
			InputRecord:     inputRecord1,
			RootRoundNumber: 1,
		}
		require.NoError(t, reqS1.Sign(s1))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: T2Timeout,
			Requests:   []*certification.BlockCertificationRequest{reqS1},
		}
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
			UnicitySeal: &types.UnicitySeal{Version: 1,
				RootChainRoundNumber: 1,
			},
		}
		ir, err := x.Verify(trustBase, luc, 0, 0)
		require.EqualError(t, err, "invalid partition 00000001 timeout proof: proof contains requests")
		require.Nil(t, ir)
	})
	t.Run("invalid, not yet timeout", func(t *testing.T) {
		reqS1 := &certification.BlockCertificationRequest{
			Partition:       sysId1,
			NodeIdentifier:  "1",
			InputRecord:     inputRecord1,
			RootRoundNumber: 1,
		}
		require.NoError(t, reqS1.Sign(s1))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: T2Timeout,
			Requests:   nil,
		}
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
			UnicitySeal: &types.UnicitySeal{Version: 1,
				RootChainRoundNumber: 1,
			},
		}
		ir, err := x.Verify(trustBase, luc, 5, 5)
		require.EqualError(t, err, "invalid partition 00000001 timeout proof: time from latest UC 4, timeout in rounds 5")
		require.Nil(t, ir)
	})
	t.Run("ok", func(t *testing.T) {
		reqS1 := &certification.BlockCertificationRequest{
			Partition:       sysId1,
			NodeIdentifier:  "1",
			InputRecord:     inputRecord1,
			RootRoundNumber: 1,
		}
		require.NoError(t, reqS1.Sign(s1))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: T2Timeout,
			Requests:   nil,
		}
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
			UnicitySeal: &types.UnicitySeal{Version: 1,
				RootChainRoundNumber: 1,
			},
		}
		ir, err := x.Verify(trustBase, luc, 6, 5)
		require.NoError(t, err)
		require.NotNil(t, ir)
		repeatIR := luc.InputRecord.NewRepeatIR()
		require.Equal(t, repeatIR, ir)
	})
}

func TestIRChangeReqMsg_VerifyQuorum(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	tb := &partitions.TrustBase{
		PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3},
	}
	t.Run("Not enough requests for quorum", func(t *testing.T) {
		reqS1 := &certification.BlockCertificationRequest{
			Partition:       sysId1,
			NodeIdentifier:  "1",
			InputRecord:     inputRecord1,
			RootRoundNumber: 1,
		}
		require.NoError(t, reqS1.Sign(s1))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{reqS1},
		}
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
			UnicitySeal: &types.UnicitySeal{Version: 1, RootChainRoundNumber: 1},
		}
		ir, err := x.Verify(tb, luc, 0, 0)
		require.EqualError(t, err, "invalid partition 00000001 quorum proof: not enough requests to prove quorum (got 1, need 2)")
		require.Nil(t, ir)
	})
	t.Run("IR does not extend last certified state", func(t *testing.T) {
		reqS1 := &certification.BlockCertificationRequest{
			Partition:       sysId1,
			NodeIdentifier:  "1",
			InputRecord:     inputRecord1,
			RootRoundNumber: 1,
		}
		require.NoError(t, reqS1.Sign(s1))
		reqS2 := &certification.BlockCertificationRequest{
			Partition:       sysId1,
			NodeIdentifier:  "2",
			InputRecord:     inputRecord1,
			RootRoundNumber: 1,
		}
		require.NoError(t, reqS2.Sign(s2))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{reqS1, reqS2},
		}
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: []byte{0, 0, 1}, RoundNumber: 1},
			UnicitySeal: &types.UnicitySeal{Version: 1, RootChainRoundNumber: 1},
		}
		ir, err := x.Verify(tb, luc, 0, 0)
		require.EqualError(t, err, "invalid certification request: request extends unknown state: expected hash: [0 0 1], got: [0 0 0]")
		require.Nil(t, ir)
	})
	t.Run("IR does not extend last certified round", func(t *testing.T) {
		reqS1 := &certification.BlockCertificationRequest{
			Partition:       sysId1,
			NodeIdentifier:  "1",
			InputRecord:     inputRecord1,
			RootRoundNumber: 1,
		}
		require.NoError(t, reqS1.Sign(s1))
		reqS2 := &certification.BlockCertificationRequest{
			Partition:       sysId1,
			NodeIdentifier:  "2",
			InputRecord:     inputRecord1,
			RootRoundNumber: 1,
		}
		require.NoError(t, reqS2.Sign(s2))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{reqS1, reqS2},
		}
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
			UnicitySeal: &types.UnicitySeal{Version: 1, RootChainRoundNumber: 2},
		}
		ir, err := x.Verify(tb, luc, 0, 0)
		require.EqualError(t, err, "invalid certification request: request root round number 1 does not match luc root round 2")
		require.Nil(t, ir)
	})
	t.Run("Contains not matching proofs", func(t *testing.T) {
		reqS1 := &certification.BlockCertificationRequest{
			Partition:       sysId1,
			NodeIdentifier:  "1",
			InputRecord:     inputRecord1,
			RootRoundNumber: 1,
		}
		require.NoError(t, reqS1.Sign(s1))
		reqS2 := &certification.BlockCertificationRequest{
			Partition:       sysId1,
			NodeIdentifier:  "2",
			InputRecord:     inputRecord1,
			RootRoundNumber: 1,
		}
		require.NoError(t, reqS2.Sign(s2))
		reqS3NotMatchingIR := &certification.BlockCertificationRequest{
			Partition:       sysId1,
			NodeIdentifier:  "3",
			InputRecord:     inputRecord2,
			RootRoundNumber: 1,
		}
		require.NoError(t, reqS3NotMatchingIR.Sign(s3))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{reqS1, reqS2, reqS3NotMatchingIR},
		}
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
			UnicitySeal: &types.UnicitySeal{Version: 1, RootChainRoundNumber: 1},
		}
		ir, err := x.Verify(tb, luc, 0, 0)
		require.EqualError(t, err, "invalid partition 00000001 quorum proof: contains proofs for different state hashes")
		require.Nil(t, ir)
	})
	t.Run("OK", func(t *testing.T) {
		reqS1 := &certification.BlockCertificationRequest{
			Partition:       sysId1,
			NodeIdentifier:  "1",
			InputRecord:     inputRecord1,
			RootRoundNumber: 1,
		}
		require.NoError(t, reqS1.Sign(s1))
		reqS2 := &certification.BlockCertificationRequest{
			Partition:       sysId1,
			NodeIdentifier:  "2",
			InputRecord:     inputRecord1,
			RootRoundNumber: 1,
		}
		require.NoError(t, reqS2.Sign(s2))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{reqS1, reqS2},
		}
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
			UnicitySeal: &types.UnicitySeal{Version: 1, RootChainRoundNumber: 1},
		}
		ir, err := x.Verify(tb, luc, 0, 0)
		require.NoError(t, err)
		require.NotNil(t, ir)
		require.Equal(t, reqS1.InputRecord, ir)
	})
}

func TestIRChangeReqMsg_VerifyQuorumNotPossible(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	trustBase := &partitions.TrustBase{
		PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2},
	}
	t.Run("No proof quorum not possible", func(t *testing.T) {
		reqS1 := &certification.BlockCertificationRequest{
			Partition:      sysId1,
			NodeIdentifier: "1",
			InputRecord:    inputRecord1,
		}
		require.NoError(t, reqS1.Sign(s1))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: QuorumNotPossible,
			Requests:   []*certification.BlockCertificationRequest{reqS1},
		}
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
		}
		ir, err := x.Verify(trustBase, luc, 0, 0)
		require.EqualError(t, err, "invalid partition 00000001 no quorum proof: not enough requests to prove only no quorum is possible")
		require.Nil(t, ir)
	})
	t.Run("ok", func(t *testing.T) {
		reqS1 := &certification.BlockCertificationRequest{
			Partition:      sysId1,
			NodeIdentifier: "1",
			InputRecord:    inputRecord1,
		}
		require.NoError(t, reqS1.Sign(s1))
		reqS2 := &certification.BlockCertificationRequest{
			Partition:      1,
			NodeIdentifier: "2",
			InputRecord:    inputRecord2,
		}
		require.NoError(t, reqS2.Sign(s2))
		x := &IRChangeReq{
			Partition:  sysId1,
			CertReason: QuorumNotPossible,
			Requests:   []*certification.BlockCertificationRequest{reqS1, reqS2},
		}
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1, Hash: prevHash, RoundNumber: 1},
		}
		ir, err := x.Verify(trustBase, luc, 0, 0)
		require.NoError(t, err)
		require.NotNil(t, ir)

		repeatIR := luc.InputRecord.NewRepeatIR()
		require.Equal(t, repeatIR, ir)
	})
}

func TestIRChangeReason_String(t *testing.T) {
	r := Quorum
	require.Equal(t, "quorum", r.String())
	require.Equal(t, "timeout", T2Timeout.String())
	require.Equal(t, "no-quorum", QuorumNotPossible.String())
	require.Equal(t, "unknown IR change reason 10", IRChangeReason(10).String())
}

func TestIRChangeReq_String(t *testing.T) {
	x := &IRChangeReq{
		Partition:  sysId2,
		CertReason: T2Timeout,
	}
	require.Equal(t, "00000002->timeout", x.String())
}
