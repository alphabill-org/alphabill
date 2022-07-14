package rootchain

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

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
	roundNumber               uint64                               // current round number
	previousRoundRootHash     []byte                               // previous round root hash
	partitionStore            *partitionStore                      // keeps track of partition in the root chain
	latestUnicityCertificates *unicityCertificatesStore            // keeps track of latest unicity certificate for each tx system
	inputRecords              map[string]*certificates.InputRecord // input records ready for certification. key is system identifier
	incomingRequests          map[string]*requestStore             // keeps track of incoming request. key is system identifier
	hashAlgorithm             gocrypto.Hash                        // hash algorithm
	signer                    crypto.Signer                        // private key of the root chain
	verifier                  crypto.Verifier
}

func NewStateFromGenesis(g *genesis.RootGenesis, signer crypto.Signer) (*State, error) {
	_, verifier, err := GetPublicKeyAndVerifier(signer)
	if err != nil {
		return nil, errors.Wrap(err, "invalid root chain private key")
	}
	if err = g.IsValid(verifier); err != nil {
		return nil, errors.Wrap(err, "invalid genesis")
	}

	s, err := NewStateFromPartitionRecords(g.GetPartitionRecords(), signer, gocrypto.Hash(g.HashAlgorithm))
	if err != nil {
		return nil, err
	}
	// load unicity certificates
	for _, p := range g.Partitions {
		identifier := p.GetSystemIdentifierString()
		s.latestUnicityCertificates.put(identifier, p.Certificate)
	}
	// reset incoming requests
	for _, store := range s.incomingRequests {
		// rest requests removes all values from the store.
		store.reset()
	}
	s.roundNumber = g.GetRoundNumber() + 1
	s.previousRoundRootHash = g.GetRoundHash()
	return s, nil
}

// NewStateFromPartitionRecords creates the State from the genesis.PartitionRecord array. The State returned by this
// method is usually used to generate genesis file.
func NewStateFromPartitionRecords(partitions []*genesis.PartitionRecord, signer crypto.Signer, hashAlgorithm gocrypto.Hash) (*State, error) {
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
	requestStores := make(map[string]*requestStore)
	partitionRecords := make(map[string]*genesis.PartitionRecord)
	for _, p := range partitions {
		util.WriteDebugJsonLog(logger, "RootChain genesis is", p)
		if err := p.IsValid(); err != nil {
			return nil, errors.Errorf("invalid partition record: %v", err)
		}
		identifier := p.GetSystemIdentifierString()
		if _, f := requestStores[identifier]; f {
			return nil, errors.Errorf("system identifier %X is not unique", identifier)
		}
		reqStore := newRequestStore()
		for _, v := range p.Validators {
			if _, f := reqStore.requests[v.NodeIdentifier]; f {
				return nil, errors.Errorf("partition %v contains multiple validators with %v id", identifier, v.NodeIdentifier)
			}
			reqStore.add(v.NodeIdentifier, v.BlockCertificationRequest)
			logger.Debug("Node %v added to the partition %X.", v.NodeIdentifier, p.SystemDescriptionRecord.SystemIdentifier)
		}
		requestStores[identifier] = reqStore
		partitionRecords[identifier] = p

		logger.Debug("Partition %X initialized.", p.SystemDescriptionRecord.SystemIdentifier)
	}

	return &State{
		roundNumber:               1,
		previousRoundRootHash:     make([]byte, gocrypto.SHA256.Size()),
		latestUnicityCertificates: newUnicityCertificateStore(),
		inputRecords:              make(map[string]*certificates.InputRecord),
		incomingRequests:          requestStores,
		partitionStore:            newPartitionStore(partitions),
		signer:                    signer,
		verifier:                  verifier,
		hashAlgorithm:             hashAlgorithm,
	}, nil
}

func (s *State) HandleBlockCertificationRequest(req *certification.BlockCertificationRequest) (*certificates.UnicityCertificate, error) {
	if err := s.isInputRecordValid(req); err != nil {
		return nil, err
	}
	systemIdentifier := string(req.SystemIdentifier)
	latestUnicityCertificate := s.latestUnicityCertificates.get(systemIdentifier)
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
		if rr, found := partitionRequests.requests[req.NodeIdentifier]; found {
			if !bytes.Equal(rr.InputRecord.Hash, req.InputRecord.Hash) {
				logger.Debug("Equivocating request with different hash: %v", req)
				return nil, errors.New("equivocating request with different hash")
			} else {
				logger.Debug("Duplicated request: %v", req)
				return nil, errors.New("duplicated request")
			}

		}
	}
	s.incomingRequests[systemIdentifier].add(req.NodeIdentifier, req)
	s.checkConsensus(systemIdentifier)
	return nil, nil
}

func (s *State) checkConsensus(identifier string) bool {
	rs := s.incomingRequests[identifier]
	hash, consensusPossible := rs.isConsensusReceived(s.partitionStore.nodeCount(identifier))
	if hash != nil {
		logger.Debug("Partition reached a consensus. SystemIdentifier: %X, InputHash: %X. ", []byte(identifier), hash.Hash)
		s.inputRecords[identifier] = hash
		return true
	} else if !consensusPossible {
		logger.Debug("Consensus not possible for partition %X.", []byte(identifier))
		luc := s.latestUnicityCertificates.get(identifier)
		if luc != nil {
			s.inputRecords[identifier] = luc.InputRecord
		}
	}
	return false
}

func (s *State) CreateUnicityCertificates() ([]string, error) {
	data := s.toUnicityTreeData(s.inputRecords)
	logger.Info("Creating unicity certificates. RoundNr %v, inputRecords: %v", s.roundNumber, len(data))
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

	var systemIdentifiers []string
	for _, d := range data {
		cert, err := ut.GetCertificate(d.SystemIdentifier)
		if err != nil {
			// this should never happen. if it does then exit with panic because we cannot generate
			// unicity tree certificates.
			panic(err)
		}
		identifier := string(d.SystemIdentifier)
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
		s.latestUnicityCertificates.put(identifier, certificate)
		systemIdentifiers = append(systemIdentifiers, identifier)
		util.WriteDebugJsonLog(logger, fmt.Sprintf("New unicity certificate for partition %X is", d.SystemIdentifier), certificate)
	}
	// send responses
	for key, store := range s.incomingRequests {
		requestStore := s.incomingRequests[key]
		if len(requestStore.requests) > 0 {
			// remove active request from the store.
			store.reset()
		}
	}

	s.inputRecords = make(map[string]*certificates.InputRecord)
	s.previousRoundRootHash = rootHash
	s.roundNumber++
	return systemIdentifiers, nil
}

func (s *State) GetLatestUnicityCertificate(systemID string) *certificates.UnicityCertificate {
	return s.latestUnicityCertificates.get(systemID)
}

// CopyOldInputRecords copies input records from the latest unicity certificates to inputRecords.
func (s *State) CopyOldInputRecords(identifier string) {
	if _, f := s.inputRecords[identifier]; !f {
		s.inputRecords[identifier] = s.latestUnicityCertificates.get(identifier).InputRecord
	}
}

func (s *State) toUnicityTreeData(records map[string]*certificates.InputRecord) []*unicitytree.Data {
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
		RootChainRoundNumber: s.roundNumber,
		PreviousHash:         s.previousRoundRootHash,
		Hash:                 rootHash,
	}
	return u, u.Sign(s.signer)
}

func (s *State) isInputRecordValid(req *certification.BlockCertificationRequest) error {
	if req == nil {
		return errors.New("input record is nil")
	}
	p := s.partitionStore.get(string(req.SystemIdentifier))
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
	return s.roundNumber
}
