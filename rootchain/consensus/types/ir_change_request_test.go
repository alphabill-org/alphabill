package types

import (
	"crypto"
	"errors"
	"testing"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
)

func TestIRChangeReqMsg_IsValid(t *testing.T) {
	inputRecord1 := &types.InputRecord{
		Version:      1,
		PreviousHash: []byte{0, 0, 0},
		Hash:         []byte{1, 1, 1},
		BlockHash:    []byte{2, 2, 2},
		SummaryValue: []byte{4, 4, 4},
		RoundNumber:  2,
		Timestamp:    types.NewTimestamp(),
	}

	t.Run("IR change req. invalid cert reason", func(t *testing.T) {
		requests := []*certification.BlockCertificationRequest{
			{
				Partition:   2,
				NodeID:      "1",
				InputRecord: inputRecord1,
				Signature:   []byte{0, 1},
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

func TestIRChangeReqMsg_Marshal(t *testing.T) {
	ircr := &IRChangeReq{
		Partition:  1,
		CertReason: QuorumNotPossible,
		Requests: []*certification.BlockCertificationRequest{
			{
				Partition:   1,
				Shard:       types.ShardID{},
				NodeID:      "1",
				InputRecord: &types.InputRecord{
					Version:      1,
					PreviousHash: []byte{0, 1},
					Hash:         []byte{2, 3},
					BlockHash:    []byte{4, 5},
					SummaryValue: []byte{6, 7},
					RoundNumber:  3,
					Timestamp:    6,
				},
				BlockSize: 4,
				StateSize: 5,
				Signature: []byte{0, 1},
			},
		},
	}
	irHasher := abhash.New(crypto.SHA256.New())
	irHasher.Write(ircr)
	hash, err := irHasher.Sum()
	require.NoError(t, err)

	bs, err := types.Cbor.Marshal(ircr)
	require.NoError(t, err)
	hshr := crypto.SHA256.New()
	hshr.Write(bs)
	require.Equal(t, hash, hshr.Sum(nil))

	ircr2 := &IRChangeReq{}
	require.NoError(t, types.Cbor.Unmarshal(bs, ircr2))
	require.Equal(t, ircr, ircr2)
}

func Test_IRChangeReq_Verify(t *testing.T) {
	// it's OK not to sign BlockCertificationRequest here - the signature check is
	// supposed to be done by the RequestVerifier which we mock. IRL the verification
	// is done by ShardInfo.ValidRequest which does verify author ID

	bcrVerifier := mockReqVerifier{
		nodeCnt:  3, // for quorum 2 requests is needed
		validReq: func(req *certification.BlockCertificationRequest) error { return nil },
	}

	const partition1 types.PartitionID = 1
	const partition2 types.PartitionID = 2

	luc := &types.UnicityCertificate{
		Version: 1,
		InputRecord: &types.InputRecord{
			Version:     1,
			Hash:        []byte{0, 0, 0},
			RoundNumber: 1,
			Timestamp:   types.NewTimestamp(),
		},
		UnicitySeal: &types.UnicitySeal{
			Version:              1,
			RootChainRoundNumber: 1,
			Timestamp:            types.NewTimestamp(),
		},
	}

	inputRecord1 := &types.InputRecord{
		Version:      1,
		PreviousHash: luc.InputRecord.Hash,
		Hash:         []byte{1, 1, 1},
		BlockHash:    []byte{2, 2, 2},
		SummaryValue: []byte{4, 4, 4},
		RoundNumber:  2,
		Timestamp:    luc.UnicitySeal.Timestamp,
	}
	inputRecord2 := &types.InputRecord{
		Version:      1,
		PreviousHash: luc.InputRecord.Hash,
		Hash:         []byte{5, 5, 5},
		BlockHash:    []byte{2, 2, 2},
		SummaryValue: []byte{4, 4, 4},
		RoundNumber:  2,
		Timestamp:    luc.UnicitySeal.Timestamp,
	}

	// pair of matching certification requests (from different nodes)
	reqS1 := certification.BlockCertificationRequest{
		Partition:   partition1,
		NodeID:      "1",
		InputRecord: inputRecord1,
	}
	reqS2 := certification.BlockCertificationRequest{
		Partition:   partition1,
		NodeID:      "2",
		InputRecord: inputRecord1,
	}

	t.Run("RequestVerifier is nil", func(t *testing.T) {
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{},
		}
		ir, err := x.Verify(nil, luc, 0, 0)
		require.EqualError(t, err, "RequestVerifier is unassigned")
		require.Nil(t, ir)
	})

	t.Run("unknown reason", func(t *testing.T) {
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: 16,
			Requests:   []*certification.BlockCertificationRequest{},
		}
		ir, err := x.Verify(bcrVerifier, luc, 0, 0)
		require.EqualError(t, err, "invalid IR Change Request: unknown reason (16)")
		require.Nil(t, ir)
	})

	t.Run("partition id in request and proof do not match", func(t *testing.T) {
		reqS1InvalidSysId := reqS1
		reqS1InvalidSysId.Partition = partition2
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{&reqS1InvalidSysId, &reqS2},
		}
		ir, err := x.Verify(bcrVerifier, luc, 0, 0)
		require.EqualError(t, err, "shard of the change request is 00000001- but block certification request is for 00000002=")
		require.Nil(t, ir)

		reqS1InvalidSysId.Partition = x.Partition
		reqS1InvalidSysId.Shard, _ = x.Shard.Split()
		x.Requests = []*certification.BlockCertificationRequest{&reqS1InvalidSysId, &reqS2}
		ir, err = x.Verify(bcrVerifier, luc, 0, 0)
		require.EqualError(t, err, "shard of the change request is 00000001- but block certification request is for 00000001=0")
		require.Nil(t, ir)
	})

	t.Run("Proof contains more request, than there are partition nodes", func(t *testing.T) {
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{&reqS1, &reqS2},
		}
		// verifier with just one node
		verifier := mockReqVerifier{
			nodeCnt:  1,
			validReq: func(req *certification.BlockCertificationRequest) error { return nil },
		}
		ir, err := x.Verify(verifier, luc, 0, 0)
		require.EqualError(t, err, "IR Change Request contains more requests than registered partition nodes")
		require.Nil(t, ir)
	})

	t.Run("Proof contains duplicate node", func(t *testing.T) {
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{&reqS1, &reqS1},
		}
		verifier := mockReqVerifier{
			nodeCnt:  2,
			validReq: func(req *certification.BlockCertificationRequest) error { return nil },
		}
		ir, err := x.Verify(verifier, luc, 0, 0)
		require.EqualError(t, err, "invalid partition 00000001 proof: contains duplicate request from node 1")
		require.Nil(t, ir)
	})

	t.Run("invalid BlockCertificationRequest", func(t *testing.T) {
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{&reqS1, &reqS2},
		}
		expErr := errors.New("this just won't do")
		verifier := mockReqVerifier{
			nodeCnt:  2,
			validReq: func(req *certification.BlockCertificationRequest) error { return expErr },
		}
		ir, err := x.Verify(verifier, luc, 0, 0)
		require.EqualError(t, err, "invalid certification request: this just won't do")
		require.Nil(t, ir)
	})

	/* timeout request verifications */

	t.Run("IR change request in timeout proof contains requests, not valid", func(t *testing.T) {
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: T2Timeout,
			Requests:   []*certification.BlockCertificationRequest{&reqS1},
		}
		ir, err := x.Verify(bcrVerifier, luc, 0, 0)
		require.EqualError(t, err, "invalid partition 00000001 timeout proof: proof contains requests")
		require.Nil(t, ir)
	})

	t.Run("invalid, not yet timeout", func(t *testing.T) {
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: T2Timeout,
			Requests:   nil,
		}
		ir, err := x.Verify(bcrVerifier, luc, 5, 5)
		require.EqualError(t, err, "invalid partition 00000001 timeout proof: time from latest UC 4, timeout in rounds 5")
		require.Nil(t, ir)
	})

	t.Run("Timeout rules OK", func(t *testing.T) {
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: T2Timeout,
			Requests:   nil,
		}
		ir, err := x.Verify(bcrVerifier, luc, 6, 5)
		require.NoError(t, err)
		require.NotNil(t, ir)
		repeatIR := luc.InputRecord.NewRepeatIR()
		require.Equal(t, repeatIR, ir)
	})

	/* Quorum rules verification tests */

	t.Run("Not enough requests for quorum", func(t *testing.T) {
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{&reqS1},
		}
		ir, err := x.Verify(bcrVerifier, luc, 0, 0)
		require.EqualError(t, err, "invalid partition 00000001 quorum proof: not enough requests to prove quorum (got 1, need 2)")
		require.Nil(t, ir)
	})

	t.Run("Contains not matching proofs", func(t *testing.T) {
		reqS3NotMatchingIR := &certification.BlockCertificationRequest{
			Partition:   partition1,
			NodeID:      "3",
			InputRecord: inputRecord2, // use different IR
		}
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{&reqS1, &reqS2, reqS3NotMatchingIR},
		}
		ir, err := x.Verify(bcrVerifier, luc, 0, 0)
		require.EqualError(t, err, "invalid partition 00000001 quorum proof: contains proofs for different state hashes")
		require.Nil(t, ir)
	})

	t.Run("Quorum rules OK", func(t *testing.T) {
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: Quorum,
			Requests:   []*certification.BlockCertificationRequest{&reqS1, &reqS2},
		}
		ir, err := x.Verify(bcrVerifier, luc, 0, 0)
		require.NoError(t, err)
		require.NotNil(t, ir)
		require.Equal(t, reqS1.InputRecord, ir)
	})

	/* Quorum not possible rules */

	t.Run("No proof quorum not possible", func(t *testing.T) {
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: QuorumNotPossible,
			Requests:   []*certification.BlockCertificationRequest{&reqS1},
		}
		ir, err := x.Verify(bcrVerifier, luc, 0, 0)
		require.EqualError(t, err, "not enough votes to prove 'no quorum' - it is possible to get 3 votes, quorum is 2")
		require.Nil(t, ir)
	})

	t.Run("there is actually quorum", func(t *testing.T) {
		// votes do have quorum but we're asked to certify "no quorum"
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: QuorumNotPossible,
			Requests:   []*certification.BlockCertificationRequest{&reqS1, &reqS2},
		}
		ir, err := x.Verify(bcrVerifier, luc, 0, 0)
		require.EqualError(t, err, "can't certify 'no quorum' as one input already does have quorum (2 votes, quorum is 2)")
		require.Nil(t, ir)
	})

	t.Run("QuorumNotPossible", func(t *testing.T) {
		reqS3 := &certification.BlockCertificationRequest{
			Partition:   1,
			NodeID:      "3",
			InputRecord: inputRecord2, // different IR than other requests
		}
		x := &IRChangeReq{
			Partition:  partition1,
			CertReason: QuorumNotPossible,
			Requests:   []*certification.BlockCertificationRequest{&reqS1, reqS3},
		}
		verifier := mockReqVerifier{
			nodeCnt:  2,
			validReq: func(req *certification.BlockCertificationRequest) error { return nil },
		}
		ir, err := x.Verify(verifier, luc, 0, 0)
		require.NoError(t, err)
		require.NotNil(t, ir)

		repeatIR := luc.InputRecord.NewRepeatIR()
		require.Equal(t, repeatIR, ir)
	})
}

func TestIRChangeReason_String(t *testing.T) {
	require.Equal(t, "quorum", Quorum.String())
	require.Equal(t, "timeout", T2Timeout.String())
	require.Equal(t, "no-quorum", QuorumNotPossible.String())
	require.Equal(t, "unknown IR change reason 10", IRChangeReason(10).String())
}

func TestIRChangeReq_String(t *testing.T) {
	x := &IRChangeReq{
		Partition:  2,
		CertReason: T2Timeout,
	}
	require.Equal(t, "00000002->timeout", x.String())
}

type mockReqVerifier struct {
	nodeCnt  uint64
	validReq func(req *certification.BlockCertificationRequest) error
}

func (rv mockReqVerifier) GetQuorum() uint64 {
	return (rv.nodeCnt / 2) + 1
}

func (rv mockReqVerifier) GetTotalNodes() uint64 { return rv.nodeCnt }

func (rv mockReqVerifier) ValidRequest(req *certification.BlockCertificationRequest) error {
	if rv.validReq == nil {
		return errors.New("validReq of mockReqVerifier not assigned")
	}
	return rv.validReq(req)
}
