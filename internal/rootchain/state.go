package rootchain

import (
	"bytes"
	gocrypto "crypto"
	"fmt"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/rootchain/store"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/unicitytree"
	"github.com/alphabill-org/alphabill/internal/util"
)

// State holds the State of the root chain.
type State struct {
	partitionStore   *partitionStore                      // keeps track of partition in the root chain
	store            store.RootChainStore                 // keeps track of latest unicity certificate for each tx system
	incomingRequests map[p.SystemIdentifier]*requestStore // keeps track of incoming request. key is system identifier
	hashAlgorithm    gocrypto.Hash                        // hash algorithm
	signer           crypto.Signer                        // private key of the root chain
	verifier         crypto.Verifier
}

var zeroHash = make([]byte, gocrypto.SHA256.Size())

func NewState(g *genesis.RootGenesis, signer crypto.Signer, store store.RootChainStore) (*State, error) {
	_, verifier, err := GetPublicKeyAndVerifier(signer)
	if err != nil {
		return nil, errors.Wrap(err, "invalid root chain private key")
	}
	if err = g.IsValid(verifier); err != nil {
		return nil, errors.Wrap(err, "invalid genesis")
	}

	s, err := NewStateFromPartitionRecords(g.GetPartitionRecords(), signer, gocrypto.Hash(g.HashAlgorithm), store)
	if err != nil {
		return nil, err
	}
	// load unicity certificates
	var certs = make([]*certificates.UnicityCertificate, 0, len(g.Partitions))
	for _, partition := range g.Partitions {
		identifier := partition.GetSystemIdentifierString()
		if s.store.GetUC(identifier) == nil {
			certs = append(certs, partition.Certificate)
		}
	}
	// reset incoming requests
	for _, requests := range s.incomingRequests {
		requests.reset()
	}

	s.PrepareNextRound(g.GetRoundHash(), certs, s.GetRoundNumber()+1)
	return s, nil
}

// NewStateFromPartitionRecords creates the State from the genesis.PartitionRecord array. The State returned by this
// method is usually used to generate genesis file.
func NewStateFromPartitionRecords(partitions []*genesis.PartitionRecord, signer crypto.Signer, hashAlgorithm gocrypto.Hash, chainStore store.RootChainStore) (*State, error) {
	if len(partitions) == 0 {
		return nil, errors.New("partitions not found")
	}
	if signer == nil {
		return nil, ErrSignerIsNil
	}
	verifier, err := signer.Verifier()
	if err != nil {
		return nil, err
	}
	requestStores := make(map[p.SystemIdentifier]*requestStore)
	for _, partition := range partitions {
		util.WriteDebugJsonLog(logger, "RootChain genesis is", partition)
		if err := partition.IsValid(); err != nil {
			return nil, errors.Errorf("invalid partition record: %v", err)
		}
		identifier := p.SystemIdentifier(partition.GetSystemIdentifier())
		if _, f := requestStores[identifier]; f {
			return nil, errors.Errorf("system identifier %X is not unique", identifier)
		}
		reqStore := newRequestStore(identifier)
		for _, v := range partition.Validators {
			if _, f := reqStore.requests[v.NodeIdentifier]; f {
				return nil, errors.Errorf("partition %v contains multiple validators with %v id", identifier, v.NodeIdentifier)
			}
			reqStore.add(v.NodeIdentifier, v.BlockCertificationRequest)
			logger.Debug("Node %v added to the partition %X.", v.NodeIdentifier, partition.SystemDescriptionRecord.SystemIdentifier)
		}
		requestStores[identifier] = reqStore

		logger.Debug("Partition %X initialized.", partition.SystemDescriptionRecord.SystemIdentifier)
	}

	return &State{
		store:            chainStore,
		incomingRequests: requestStores,
		partitionStore:   newPartitionStore(partitions),
		signer:           signer,
		verifier:         verifier,
		hashAlgorithm:    hashAlgorithm,
	}, nil
}

func (s *State) HandleBlockCertificationRequest(req *certification.BlockCertificationRequest) (*certificates.UnicityCertificate, error) {
	if err := s.isInputRecordValid(req); err != nil {
		return nil, err
	}
	systemIdentifier := p.SystemIdentifier(req.SystemIdentifier)
	latestUnicityCertificate := s.store.GetUC(systemIdentifier)
	seal := latestUnicityCertificate.UnicitySeal
	if req.RootRoundNumber < seal.RootChainRoundNumber {
		// Older UC, return current.
		return latestUnicityCertificate, errors.Errorf("old request: root round number %v, partition node round number %v", seal.RootChainRoundNumber, req.RootRoundNumber)
	} else if req.RootRoundNumber > seal.RootChainRoundNumber {
		// should not happen, partition has newer UC
		return latestUnicityCertificate, errors.Errorf("partition has never unicity certificate: root round number %v, partition node round number %v", seal.RootChainRoundNumber, req.RootRoundNumber)
	} else if !bytes.Equal(req.InputRecord.PreviousHash, latestUnicityCertificate.InputRecord.Hash) {
		// Extending of unknown State.
		return latestUnicityCertificate, errors.Errorf("request extends unknown state: expected hash: %v, got: %v", seal.Hash, req.InputRecord.PreviousHash)
	}
	partitionRequests, f := s.incomingRequests[systemIdentifier]
	if f {
		logger.Debug("Partition '%X' has already sent %v certification requests", req.SystemIdentifier, len(partitionRequests.requests))
		if rr, found := partitionRequests.requests[req.NodeIdentifier]; found {
			if !bytes.Equal(rr.InputRecord.Hash, req.InputRecord.Hash) {
				logger.Debug("Equivocating request with different hash: %v", req)
				return nil, errors.New("equivocating request with different hash")
			} else {
				logger.Debug("Duplicated request: %v", req)
				return nil, errors.New("duplicated request")
			}
		}
		partitionRequests.add(req.NodeIdentifier, req)
		s.checkConsensus(partitionRequests)
	}
	return nil, nil
}

func (s *State) checkConsensus(rs *requestStore) bool {
	if uc := s.store.GetUC(rs.id); uc != nil {
		logger.Debug("Checking consensus for '%X', latest completed root round: %v", []byte(rs.id), uc.UnicitySeal.RootChainRoundNumber)
	}
	inputRecord, consensusPossible := rs.isConsensusReceived(s.partitionStore.nodeCount(rs.id))
	if inputRecord != nil {
		logger.Debug("Partition reached a consensus. SystemIdentifier: %X, InputHash: %X. ", []byte(rs.id), inputRecord.Hash)
		s.store.AddIR(rs.id, inputRecord)
		return true
	} else if !consensusPossible {
		logger.Debug("Consensus not possible for partition %X.", []byte(rs.id))
		luc := s.store.GetUC(rs.id)
		if luc != nil {
			s.store.AddIR(rs.id, luc.InputRecord)
		}
	}
	return false
}

func (s *State) CreateUnicityCertificates() ([]p.SystemIdentifier, error) {
	data := s.toUnicityTreeData(s.store.GetAllIRs())
	logger.Debug("Input records are:")
	for _, ir := range data {
		util.WriteDebugJsonLog(logger, fmt.Sprintf("IR for partition %X is:", ir.SystemIdentifier), ir)
	}
	ut, err := unicitytree.New(s.hashAlgorithm.New(), data)
	if err != nil {
		return nil, err
	}
	rootHash := ut.GetRootHash()
	logger.Info("New root hash is %X", rootHash)
	unicitySeal, err := s.createUnicitySeal(rootHash)
	if err != nil {
		return nil, err
	}
	logger.Info("Creating unicity certificates. RoundNr %v, inputRecords: %v", unicitySeal.RootChainRoundNumber, len(data))

	var systemIdentifiers []p.SystemIdentifier
	var certs = make([]*certificates.UnicityCertificate, 0, len(data))
	for _, d := range data {
		cert, err := ut.GetCertificate(d.SystemIdentifier)
		if err != nil {
			// this should never happen. if it does then exit with panic because we cannot generate
			// unicity tree certificates.
			panic(err)
		}
		identifier := p.SystemIdentifier(d.SystemIdentifier)
		sdrHash := s.partitionStore.get(identifier).SystemDescriptionRecord.Hash(s.hashAlgorithm)

		certificate := &certificates.UnicityCertificate{
			InputRecord: d.InputRecord,
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
				SystemIdentifier:      cert.SystemIdentifier,
				SiblingHashes:         cert.SiblingHashes,
				SystemDescriptionHash: sdrHash,
			},
			UnicitySeal: unicitySeal,
		}

		// check the certificate
		err = certificate.IsValid(s.verifier, s.hashAlgorithm, d.SystemIdentifier, d.SystemDescriptionRecordHash)
		if err != nil {
			// should never happen.
			panic(err)
		}
		certs = append(certs, certificate)
		systemIdentifiers = append(systemIdentifiers, identifier)
		util.WriteDebugJsonLog(logger, fmt.Sprintf("New unicity certificate for partition %X is", d.SystemIdentifier), certificate)

		if requestStore, found := s.incomingRequests[identifier]; found {
			if len(requestStore.requests) > 0 {
				// remove active request from the store.
				requestStore.reset()
			}
		}
	}

	s.PrepareNextRound(rootHash, certs, unicitySeal.RootChainRoundNumber+1)
	return systemIdentifiers, nil
}

func (s *State) GetLatestUnicityCertificate(id p.SystemIdentifier) *certificates.UnicityCertificate {
	return s.store.GetUC(id)
}

// CopyOldInputRecords copies input records from the latest unicity certificate
func (s *State) CopyOldInputRecords(id p.SystemIdentifier) {
	if ir := s.store.GetIR(id); ir == nil {
		s.store.AddIR(id, s.store.GetUC(id).InputRecord)
	}
}

func (s *State) toUnicityTreeData(records map[p.SystemIdentifier]*certificates.InputRecord) []*unicitytree.Data {
	data := make([]*unicitytree.Data, len(records))
	i := 0
	for key, r := range records {
		systemDescriptionRecord := s.partitionStore.get(key).SystemDescriptionRecord
		data[i] = &unicitytree.Data{
			SystemIdentifier:            systemDescriptionRecord.SystemIdentifier,
			InputRecord:                 r,
			SystemDescriptionRecordHash: systemDescriptionRecord.Hash(s.hashAlgorithm),
		}
		i++
	}
	return data
}

func (s *State) createUnicitySeal(rootHash []byte) (*certificates.UnicitySeal, error) {
	u := &certificates.UnicitySeal{
		RootChainRoundNumber: s.GetRoundNumber(),
		PreviousHash:         s.store.GetPreviousRoundRootHash(),
		Hash:                 rootHash,
	}
	return u, u.Sign(s.signer)
}

func (s *State) isInputRecordValid(req *certification.BlockCertificationRequest) error {
	if req == nil {
		return errors.New("input record is nil")
	}
	p := s.partitionStore.get(p.SystemIdentifier(req.SystemIdentifier))
	if p == nil {
		return errors.Errorf("unknown SystemIdentifier %X", req.SystemIdentifier)
	}
	nodeIdentifier := req.NodeIdentifier
	node := p.GetPartitionNode(nodeIdentifier)
	if node == nil {
		return errors.Errorf("unknown node identifier %v", nodeIdentifier)
	}
	verifier, err := crypto.NewVerifierSecp256k1(node.SigningPublicKey)
	if err != nil {
		return errors.Wrapf(err, "node %v has invalid signing public key %X", nodeIdentifier, node.SigningPublicKey)
	}
	if err := req.IsValid(verifier); err != nil {
		return errors.Wrapf(err, "invalid InputRequest request")
	}
	return nil
}

func (s *State) GetRoundNumber() uint64 {
	return s.store.GetRoundNumber()
}

func (s *State) PrepareNextRound(prevStateHash []byte, ucs []*certificates.UnicityCertificate, newRoundNumber uint64) {
	s.store.SaveState(prevStateHash, ucs, newRoundNumber)
}
