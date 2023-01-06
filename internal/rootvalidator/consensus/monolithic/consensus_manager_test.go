package monolithic

import (
	"crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
	"github.com/stretchr/testify/require"
)

var sysId0 = []byte{0, 0, 0, 0}
var sysId1 = []byte{0, 0, 0, 1}
var sysId2 = []byte{0, 0, 0, 2}

func TestConsensusManager_checkT2Timeout(t *testing.T) {
	partitions, err := partition_store.NewPartitionStore([]*genesis.PartitionRecord{
		{SystemDescriptionRecord: &genesis.SystemDescriptionRecord{SystemIdentifier: sysId0, T2Timeout: 2500}},
		{SystemDescriptionRecord: &genesis.SystemDescriptionRecord{SystemIdentifier: sysId1, T2Timeout: 2500}},
		{SystemDescriptionRecord: &genesis.SystemDescriptionRecord{SystemIdentifier: sysId2, T2Timeout: 2500}},
	})
	require.NoError(t, err)
	manager := &ConsensusManager{
		conf: &consensusConfig{
			hashAlgo:   crypto.SHA256,
			t3Timeout:  900 * time.Millisecond,
			stateStore: store.NewInMemStateStore(crypto.SHA256),
		},
		selfId:     "test",
		partitions: partitions,
		ir: map[p.SystemIdentifier]*certificates.InputRecord{
			p.SystemIdentifier(sysId0): {Hash: []byte{0, 1}, PreviousHash: []byte{0, 0}, BlockHash: []byte{1, 2}, SummaryValue: []byte{2, 3}},
			p.SystemIdentifier(sysId1): {Hash: []byte{0, 1}, PreviousHash: []byte{0, 0}, BlockHash: []byte{1, 2}, SummaryValue: []byte{2, 3}},
			p.SystemIdentifier(sysId2): {Hash: []byte{0, 1}, PreviousHash: []byte{0, 0}, BlockHash: []byte{1, 2}, SummaryValue: []byte{2, 3}},
		},
		changes: map[p.SystemIdentifier]*certificates.InputRecord{},
	}
	// store mock state
	lastState := store.RootState{LatestRound: 4, LatestRootHash: []byte{0, 1}, Certificates: map[p.SystemIdentifier]*certificates.UnicityCertificate{
		p.SystemIdentifier(sysId0): {
			InputRecord:            &certificates.InputRecord{Hash: []byte{1, 1}, PreviousHash: []byte{1, 1}, BlockHash: []byte{2, 3}, SummaryValue: []byte{3, 4}},
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{},
			UnicitySeal:            &certificates.UnicitySeal{RootRoundInfo: &certificates.RootRoundInfo{RoundNumber: 3}}}, // no timeout (5 - 3) * 900 = 1800 ms
		p.SystemIdentifier(sysId1): {
			InputRecord:            &certificates.InputRecord{Hash: []byte{1, 2}, PreviousHash: []byte{1, 1}, BlockHash: []byte{2, 3}, SummaryValue: []byte{3, 4}},
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{},
			UnicitySeal:            &certificates.UnicitySeal{RootRoundInfo: &certificates.RootRoundInfo{RoundNumber: 2}}}, // timeout (5 - 2) * 900 = 2700 ms
		p.SystemIdentifier(sysId2): {
			InputRecord:            &certificates.InputRecord{Hash: []byte{1, 3}, PreviousHash: []byte{1, 1}, BlockHash: []byte{2, 3}, SummaryValue: []byte{3, 4}},
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{},
			UnicitySeal:            &certificates.UnicitySeal{RootRoundInfo: &certificates.RootRoundInfo{RoundNumber: 4}}}, // no timeout
	}}
	require.NoError(t, manager.checkT2Timeout(5, &lastState))
	// if round is 900ms then timeout of 2500 is reached in 3 * 900ms rounds, which is 2700ms
	require.Equal(t, 1, len(manager.changes))
	require.Contains(t, manager.changes, p.SystemIdentifier(sysId1))
}
