package storage

import (
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
)

const (
	roundCreationTime = 100000
)

var zeroHash = make([]byte, gocrypto.SHA256.Size())
var sysID = protocol.SystemIdentifier([]byte{0, 0, 0, 0})
var mockUc = &certificates.UnicityCertificate{
	UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
		SystemIdentifier:      []byte(sysID),
		SiblingHashes:         nil,
		SystemDescriptionHash: nil,
	},
	UnicitySeal: &certificates.UnicitySeal{
		RootRoundInfo: &certificates.RootRoundInfo{
			RoundNumber:     1,
			Timestamp:       roundCreationTime,
			CurrentRootHash: make([]byte, gocrypto.SHA256.Size()),
		},
		CommitInfo: &certificates.CommitInfo{
			RootRoundInfoHash: make([]byte, gocrypto.SHA256.Size()),
			RootHash:          make([]byte, gocrypto.SHA256.Size()),
		},
	},
}
var testGenesis = &genesis.RootGenesis{
	Partitions: []*genesis.GenesisPartitionRecord{
		{
			Nodes:       nil,
			Certificate: mockUc,
			SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
				SystemIdentifier: sysID.Bytes(),
				T2Timeout:        2500,
			},
		},
	},
}
