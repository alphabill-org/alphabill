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

type CertificationRequestStore interface {
	Add(request *certification.BlockCertificationRequest) error
	IsConsensusReceived(id p.SystemIdentifier, nrOfNodes int) (*certificates.InputRecord, bool)
	GetRequests(id p.SystemIdentifier) []*certification.BlockCertificationRequest
	Reset()
	Clear(id p.SystemIdentifier)
}

type StateStore interface {
	Save(state store.RootState) error
	Get() (store.RootState, error)
}

// State holds the State of the root chain.
type State struct {
	partitionStore   *PartitionStore                                  // keeps track of partition in the root chain
	store            StateStore                                       // keeps track of latest unicity certificate for each tx system
	inputRecords     map[p.SystemIdentifier]*certificates.InputRecord // input records ready for certification. key is system identifier
	incomingRequests CertificationRequestStore                        // keeps track of incoming request. key is system identifier
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
	state, err := stateStore.Get()
	if err != nil {
		return nil, err
	}
	storeInitiated := state.LatestRound > 0
	// load/store unicity certificates and register partitions from root genesis file
	partitionStore, err := NewPartitionStoreFromGenesis(g.Partitions)
	var certs = make(map[p.SystemIdentifier]*certificates.UnicityCertificate)
	for _, partition := range g.Partitions {
		identifier := partition.GetSystemIdentifierString()
		certs[identifier] = partition.Certificate
		// In case the store is already initiated, check if partition identifier is known
		if storeInitiated {
			if _, f := state.Certificates[identifier]; !f {
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
		partitionStore:   partitionStore,
		store:            stateStore,
		inputRecords:     make(map[p.SystemIdentifier]*certificates.InputRecord),
		incomingRequests: NewCertificationRequestStore(),
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
	// Make sure that all provided partitions have unique system identifiers
	if err := genesis.CheckPartitionSystemIdentifiersUnique(partitions); err != nil {
		return nil, err
	}
	requestStore := NewCertificationRequestStore()

	// Create a temporary state store for genesis generation
	stateStore := store.NewInMemStateStore(hashAlgorithm)
	partitionStore := NewEmptyPartitionStore()

	for _, partition := range partitions {
		util.WriteDebugJsonLog(logger, "RootChain genesis is", partition)
		// Check that partition is valid: required fields sent and no duplicate nodes
		if err := partition.IsValid(); err != nil {
			return nil, errors.Errorf("invalid partition record: %v", err)
		}
		identifier := p.SystemIdentifier(partition.SystemDescriptionRecord.SystemIdentifier)
		// Collect certification requests
		for _, v := range partition.Validators {
			if err := requestStore.Add(v.BlockCertificationRequest); err != nil {
				return nil, err
			}
			logger.Debug("Node %v added to the partition %X.", v.NodeIdentifier, identifier)
		}
		err := partitionStore.AddPartition(partition)
		if err != nil {
			return nil, errors.Wrap(err, "add partition failed")
		}
		logger.Debug("Partition %X initialized.", identifier)
	}

	return &State{
		partitionStore:   partitionStore,
		store:            stateStore,
		inputRecords:     make(map[p.SystemIdentifier]*certificates.InputRecord),
		incomingRequests: requestStore,
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
	if err := s.incomingRequests.Add(req); err != nil {
		return nil, err
	}
	s.checkConsensus(systemIdentifier)
	return nil, nil
}

func (s *State) checkConsensus(id p.SystemIdentifier) bool {
	inputRecord, consensusPossible := s.incomingRequests.IsConsensusReceived(id, s.partitionStore.NodeCount(id))
	if inputRecord != nil {
		logger.Debug("Partition reached a consensus. SystemIdentifier: %X, InputHash: %X. ", []byte(id), inputRecord.Hash)
		s.inputRecords[id] = inputRecord
		return true
	} else if !consensusPossible {
		logger.Debug("Consensus not possible for partition %X.", []byte(id))
		// Get last unicity certificate for the partition
		luc, err := s.GetLatestUnicityCertificate(id)
		if err != nil {
			logger.Error("Cannot re-certify partition: SystemIdentifier: %X, error: %v", []byte(id), err.Error())
			return false
		}
		s.inputRecords[id] = luc.InputRecord
	}
	return false
}

// CreateUnicityCertificates certifies input records and returns state containing changes (new root, round, UCs changed)
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
		logger.Info("Failed to read last state from storage: %v", err.Error())
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
		sysDes, err := s.partitionStore.GetSystemDescription(identifier)
		if err != nil {
			// impossible
			return nil, err
		}
		sdrHash := sysDes.Hash(s.hashAlgorithm)

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
		util.WriteDebugJsonLog(logger, fmt.Sprintf("New unicity certificate for partition %X is", d.SystemIdentifier), certificate)
		// clear requests for partition after new Unicity Certificate has been made
		s.incomingRequests.Clear(identifier)
	}
	// Save state
	newState := store.RootState{LatestRound: newRound, Certificates: certs, LatestRootHash: rootHash}
	if err := s.store.Save(newState); err != nil {
		return nil, err
	}
	s.inputRecords = make(map[p.SystemIdentifier]*certificates.InputRecord)
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
		systemDescriptionRecord, err := s.partitionStore.GetSystemDescription(key)
		if err != nil {
			// impossible, refactor needed
			continue
		}
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
	tb, err := s.partitionStore.GetTrustBase(p.SystemIdentifier(req.SystemIdentifier))
	if err != nil {
		return errors.Errorf("unknown SystemIdentifier %X", req.SystemIdentifier)
	}
	nodeIdentifier := req.NodeIdentifier
	verifier, f := tb[nodeIdentifier]
	if !f {
		return errors.Errorf("unknown node identifier %v", nodeIdentifier)
	}
	if err := req.IsValid(verifier); err != nil {
		return errors.Wrapf(err, "invalid InputRequest request")
	}
	return nil
}
