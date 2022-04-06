package rootchain

import (
	"bytes"
	gocrypto "crypto"
	"encoding/base64"
	"fmt"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain/unicitytree"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"
)

// state holds the state of the root chain.
type state struct {
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

func newStateFromGenesis(g *genesis.RootGenesis, signer crypto.Signer) (*state, error) {
	_, verifier, err := GetPublicKeyAndVerifier(signer)
	if err != nil {
		return nil, errors.Wrap(err, "invalid root chain private key")
	}
	if err = g.IsValid(verifier); err != nil {
		return nil, errors.Wrap(err, "invalid genesis")
	}

	s, err := newStateFromPartitionRecords(g.GetPartitionRecords(), signer, gocrypto.Hash(g.HashAlgorithm))
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

// newStateFromPartitionRecords creates the state from the genesis.PartitionRecord array. The state returned by this
// method is usually used to generate genesis file.
func newStateFromPartitionRecords(partitions []*genesis.PartitionRecord, signer crypto.Signer, hashAlgorithm gocrypto.Hash) (*state, error) {
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
		WriteDebugJsonLog(logger, "Loading partition record", p)
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
			reqStore.add(v.NodeIdentifier, &p1.RequestEvent{Req: v.P1Request})
			logger.Debug("Node %v added to the partition %X.", v.NodeIdentifier, p.SystemDescriptionRecord.SystemIdentifier)
		}
		requestStores[identifier] = reqStore
		partitionRecords[identifier] = p

		logger.Debug("Partition %X initialized.", p.SystemDescriptionRecord.SystemIdentifier)
	}

	return &state{
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

func (s *state) handleInputRequestEvent(e *p1.RequestEvent) {
	if !s.isInputRecordValid(e.Req) && e.ResponseCh != nil {
		e.ResponseCh <- &p1.P1Response{Status: p1.P1Response_INVALID}
		return
	}
	r := e.Req
	identifier := string(r.SystemIdentifier)
	latestUnicityCertificate := s.latestUnicityCertificates.get(identifier)
	seal := latestUnicityCertificate.UnicitySeal
	if r.RootRoundNumber < seal.RootChainRoundNumber {
		// Older UC, return current.
		logger.Debug("Old request. Root chain round number %X, partition round number: %X", seal.RootChainRoundNumber, r.RootRoundNumber)
		// "ok" status means that response contains the UC
		e.ResponseCh <- &p1.P1Response{Status: p1.P1Response_OK, Message: latestUnicityCertificate}
		return
	} else if r.RootRoundNumber > seal.RootChainRoundNumber {
		// should not happen, partition has newer UC
		logger.Warning("Partition has never unicity certificate. Root chain round number %X, partition round number: %X", seal.RootChainRoundNumber, r.RootRoundNumber)
		e.ResponseCh <- &p1.P1Response{Status: p1.P1Response_INVALID}
		return
	} else if !bytes.Equal(r.InputRecord.PreviousHash, latestUnicityCertificate.InputRecord.Hash) {
		// Extending of unknown state. "ok" status means that response contains the UC
		logger.Debug("P1 Request extends unknown state. Expected previous hash: %X, got: %X", seal.Hash, r.InputRecord.PreviousHash)
		// "ok" status means that response contains the UC
		e.ResponseCh <- &p1.P1Response{Status: p1.P1Response_OK, Message: latestUnicityCertificate}
		return
	}
	partitionRequests, f := s.incomingRequests[identifier]
	if f {
		if rr, found := partitionRequests.requests[r.NodeIdentifier]; found {
			if !bytes.Equal(rr.Req.InputRecord.Hash, r.InputRecord.Hash) {
				logger.Debug("Equivocating request with different hash: %v", r)
				e.ResponseCh <- &p1.P1Response{Status: p1.P1Response_INVALID}
				return
			} else {
				logger.Debug("Duplicated request: %v", r)
				e.ResponseCh <- &p1.P1Response{Status: p1.P1Response_DUPLICATE}
				return
			}

		}
	}
	s.incomingRequests[identifier].add(e.Req.NodeIdentifier, e)
	s.checkConsensus(identifier)
}

func (s *state) checkConsensus(identifier string) bool {
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

func (s *state) createUnicityCertificates() ([]string, error) {
	data := s.toUnicityTreeData(s.inputRecords)
	logger.Info("Creating unicity certificates. RoundNr %v, inputRecords: %v", s.roundNumber, len(data))
	ut, err := unicitytree.New(s.hashAlgorithm.New(), data)
	if err != nil {
		return nil, err
	}
	rootHash := ut.GetRootHash()
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
		WriteDebugJsonLog(logger, fmt.Sprintf("New uncity certificate for partition %X", d.SystemIdentifier), certificate)
	}
	// send responses
	for key, store := range s.incomingRequests {
		requestStore := s.incomingRequests[key]
		if len(requestStore.requests) > 0 {
			logger.Info("Sending responses to partition %X. Active requests count is %v.", []byte(key), len(requestStore.requests))
			for _, req := range requestStore.requests {
				logger.Debug("Returning unicity certificate for node %v from partition %X", req.Req.NodeIdentifier, []byte(key))
				if req.ResponseCh != nil {
					req.ResponseCh <- &p1.P1Response{
						Status:  p1.P1Response_OK,
						Message: s.latestUnicityCertificates.get(key),
					}
				}
			}
			// remove active request from the store.
			store.reset()
		}
	}

	s.inputRecords = make(map[string]*certificates.InputRecord)
	logger.Debug("New Root Chain root hash is %X", rootHash)
	s.previousRoundRootHash = rootHash
	s.roundNumber++
	return systemIdentifiers, nil
}

// copyOldInputRecords copies input records from the latest unicity certificates to inputRecords.
func (s *state) copyOldInputRecords(identifier string) {
	if _, f := s.inputRecords[identifier]; !f {
		s.inputRecords[identifier] = s.latestUnicityCertificates.get(identifier).InputRecord
	}
}

func (s *state) toUnicityTreeData(records map[string]*certificates.InputRecord) []*unicitytree.Data {
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

func (s *state) createUnicitySeal(rootHash []byte) (*certificates.UnicitySeal, error) {
	u := &certificates.UnicitySeal{
		RootChainRoundNumber: s.roundNumber,
		PreviousHash:         s.previousRoundRootHash,
		Hash:                 rootHash,
	}
	return u, u.Sign(s.signer)
}

func (s *state) isInputRecordValid(req *p1.P1Request) bool {
	if req == nil {
		logger.Warning("InputRecord is nil")
		return false
	}
	p := s.partitionStore.get(string(req.SystemIdentifier))
	if p == nil {
		logger.Warning("Unknown SystemIdentifier %X", req.SystemIdentifier)
		return false
	}
	nodeIdentifier := req.NodeIdentifier
	node := p.GetPartitionNode(nodeIdentifier)
	if node == nil {
		logger.Warning("Unknown node identifier %v", nodeIdentifier)
		return false
	}
	verifier, err := crypto.NewVerifierSecp256k1(node.PublicKey)
	if err != nil {
		logger.Warning("Node %v has invalid public key %X: %v", nodeIdentifier, node.PublicKey, err)
		return false
	}
	if err := req.IsValid(verifier); err != nil {
		logger.Warning("Invalid InputRequest request %v. Error: %v. PublicKey: %v", req, err, base64.StdEncoding.EncodeToString(node.PublicKey))
		return false
	}
	return true
}
