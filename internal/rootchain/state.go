package rootchain

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/store"
	"github.com/alphabill-org/alphabill/internal/rootchain/unicitytree"
	"github.com/alphabill-org/alphabill/internal/util"
)

type StateStore interface {
	Save(state store.RootState) error
	Get() (store.RootState, error)
}

// State holds the State of the root chain.
type State struct {
	partitionStore   *partitionStore                                  // keeps track of partition in the root chain
	store            StateStore                                       // keeps track of latest unicity certificate for each tx system
	inputRecords     map[p.SystemIdentifier]*certificates.InputRecord // input records ready for certification. key is system identifier
	incomingRequests map[p.SystemIdentifier]*requestStore             // keeps track of incoming request. key is system identifier
	hashAlgorithm    gocrypto.Hash                                    // hash algorithm
	selfId           string                                           // node identifier
	signer           crypto.Signer                                    // private key of the root chain
	verifiers        map[string]crypto.Verifier
}

func NewState(g *genesis.RootGenesis, self string, signer crypto.Signer, stateStore StateStore) (*State, error) {
	_, verifier, err := GetPublicKeyAndVerifier(signer)
	if err != nil {
		return nil, errors.Wrap(err, "invalid root chain private key")
	}
	if g == nil {
		return nil, errors.New("root genesis is nil")
	}
	if err := g.Verify(); err != nil {
		return nil, errors.Wrap(err, "invalid genesis")
	}
	if stateStore == nil {
		return nil, errors.New("root chain store is nil")
	}
	// Init from genesis file is done only once
	rcState, err := stateStore.Get()
	if err != nil {
		return nil, err
	}
	storeInitiated := rcState.LatestRound > 0
	// load/store unicity certificates and register partitions from root genesis file
	partitionStore := partitionStore{}
	var certs = make(map[p.SystemIdentifier]*certificates.UnicityCertificate)
	for _, partition := range g.Partitions {
		identifier := partition.GetSystemIdentifierString()
		partitionStore[identifier] = &genesis.PartitionRecord{
			SystemDescriptionRecord: partition.SystemDescriptionRecord,
			Validators:              partition.Nodes,
		}
		certs[identifier] = partition.Certificate
		// In case the store is already initiated, check if partition identifier is known
		if storeInitiated {
			if _, f := rcState.Certificates[identifier]; !f {
				return nil, errors.Errorf("invalid genesis, new partition %v detected", identifier)
			}
		}
	}
	// If not initiated, save genesis file to store
	if !storeInitiated {
		if err := stateStore.Save(store.RootState{LatestRound: g.GetRoundNumber(), Certificates: certs, LatestRootHash: g.GetRoundHash()}); err != nil {
			return nil, err
		}
	}

	return &State{
		partitionStore:   &partitionStore,
		store:            stateStore,
		inputRecords:     make(map[p.SystemIdentifier]*certificates.InputRecord),
		incomingRequests: make(map[p.SystemIdentifier]*requestStore),
		selfId:           self,
		signer:           signer,
		verifiers:        map[string]crypto.Verifier{self: verifier},
		hashAlgorithm:    gocrypto.Hash(g.Root.Consensus.HashAlgorithm),
	}, nil
}

// NewStateFromPartitionRecords creates the State from the genesis.PartitionRecord array. The State returned by this
// method is usually used to generate genesis file.
func NewStateFromPartitionRecords(partitions []*genesis.PartitionRecord, nodeId string, signer crypto.Signer, hashAlgorithm gocrypto.Hash) (*State, error) {
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

	// Create a temporary state store for genesis generation
	stateStore := store.NewInMemStateStore(hashAlgorithm)
	requestStores := make(map[p.SystemIdentifier]*requestStore)
	partitionStore := partitionStore{}

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
		partitionStore[identifier] = partition
		logger.Debug("Partition %X initialized.", partition.SystemDescriptionRecord.SystemIdentifier)
	}

	return &State{
		partitionStore:   &partitionStore,
		store:            stateStore,
		inputRecords:     make(map[p.SystemIdentifier]*certificates.InputRecord),
		incomingRequests: requestStores,
		selfId:           nodeId,
		signer:           signer,
		verifiers:        map[string]crypto.Verifier{nodeId: verifier},
		hashAlgorithm:    hashAlgorithm,
	}, nil
}

func (s *State) HandleBlockCertificationRequest(req *certification.BlockCertificationRequest) (*certificates.UnicityCertificate, error) {
	if err := s.isInputRecordValid(req); err != nil {
		return nil, err
	}
	systemIdentifier := p.SystemIdentifier(req.SystemIdentifier)
	latestUnicityCertificate, err := s.GetLatestUnicityCertificate(systemIdentifier)
	if err != nil {
		return nil, err
	}
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
	// Partition must be registered in partition store, otherwise error is returned earlier
	// On first incoming request, create request store and add the request
	if !f {
		partitionRequests = newRequestStore(systemIdentifier)
		s.incomingRequests[systemIdentifier] = partitionRequests
	}
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
	return nil, nil
}

func (s *State) checkConsensus(rs *requestStore) bool {
	logger.Debug("Checking consensus for '%X'")
	inputRecord, consensusPossible := rs.isConsensusReceived(s.partitionStore.nodeCount(rs.id))
	if inputRecord != nil {
		logger.Debug("Partition reached a consensus. SystemIdentifier: %X, InputHash: %X. ", []byte(rs.id), inputRecord.Hash)
		s.inputRecords[rs.id] = inputRecord
		return true
	} else if !consensusPossible {
		logger.Debug("Consensus not possible for partition %X.", []byte(rs.id))
		// Get last unicity certificate for the partition
		luc, err := s.GetLatestUnicityCertificate(rs.id)
		if err != nil {
			logger.Error("Cannot re-certify partition: SystemIdentifier: %X, error: %s", []byte(rs.id), err.Error())
			return false
		}
		s.inputRecords[rs.id] = luc.InputRecord
	}
	return false
}

func (s *State) CreateUnicityCertificates() (*store.RootState, error) {
	data := s.toUnicityTreeData(s.inputRecords)
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
	lastState, err := s.store.Get()
	if err != nil {
		logger.Info("Failed to read last state from storage: %s", err.Error())
		return nil, err
	}
	newRound := lastState.LatestRound + 1
	unicitySeal, err := s.createUnicitySeal(newRound, rootHash, lastState.LatestRootHash)
	if err != nil {
		return nil, err
	}
	logger.Info("Creating unicity certificates. RoundNr %v, inputRecords: %v", newRound, len(data))

	var certs = make(map[p.SystemIdentifier]*certificates.UnicityCertificate)
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
		err = certificate.IsValid(s.verifiers, s.hashAlgorithm, d.SystemIdentifier, d.SystemDescriptionRecordHash)
		if err != nil {
			// should never happen.
			panic(err)
		}
		certs[identifier] = certificate
		util.WriteDebugJsonLog(logger, fmt.Sprintf("New unicity certificate for partition %X is", identifier), certificate)

		if requestStore, found := s.incomingRequests[identifier]; found {
			if len(requestStore.requests) > 0 {
				// remove active request from the store.
				requestStore.reset()
			}
		}
	}
	// Save state
	newState := store.RootState{LatestRound: newRound, Certificates: certs, LatestRootHash: rootHash}
	if err := s.store.Save(newState); err != nil {
		return nil, err
	}
	return &newState, nil
}

// GetLatestUnicityCertificate returns last UC for system identifier
func (s *State) GetLatestUnicityCertificate(id p.SystemIdentifier) (*certificates.UnicityCertificate, error) {
	state, err := s.store.Get()
	if err != nil {
		return nil, err
	}
	luc, f := state.Certificates[id]
	if !f {
		return nil, errors.Errorf("no certificate found for system id %X", id)
	}
	return luc, nil
}

// CopyOldInputRecords copies input records from the latest unicity certificate
func (s *State) CopyOldInputRecords(id p.SystemIdentifier) {
	luc, err := s.GetLatestUnicityCertificate(id)
	if err != nil {
		logger.Warning("Unable to re-certify partition %X, error: %v", err.Error())
		return
	}
	s.inputRecords[id] = luc.InputRecord
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

func (s *State) createUnicitySeal(newRound uint64, newRootHash []byte, prevRoot []byte) (*certificates.UnicitySeal, error) {
	u := &certificates.UnicitySeal{
		RootChainRoundNumber: newRound,
		PreviousHash:         prevRoot,
		Hash:                 newRootHash,
	}
	return u, u.Sign(s.selfId, s.signer)
}

func (s *State) isInputRecordValid(req *certification.BlockCertificationRequest) error {
	if req == nil {
		return errors.New("input record is nil")
	}
	partition := s.partitionStore.get(p.SystemIdentifier(req.SystemIdentifier))
	if partition == nil {
		return errors.Errorf("unknown SystemIdentifier %X", req.SystemIdentifier)
	}
	nodeIdentifier := req.NodeIdentifier
	node := partition.GetPartitionNode(nodeIdentifier)
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

// GetCurrentRoundNumber returns last stored round number + 1 as current round number
func (s *State) GetCurrentRoundNumber() uint64 {
	state, err := s.store.Get()
	if err != nil {
		return 0
	}
	return state.LatestRound + 1
}
