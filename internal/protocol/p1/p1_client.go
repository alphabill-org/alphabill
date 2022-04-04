package p1

import (
	"context"
	gocrypto "crypto"

	ma "github.com/multiformats/go-multiaddr"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
)

var (
	ErrClientPeerIsNil            = errors.New("peer client is nil")
	ErrSignerIsNil                = errors.New("client signer is nil")
	ErrRootChainVerifierIsNil     = errors.New("root chain verifier is nil")
	ErrorClientConfigurationIsNil = errors.New("client configuration is nil")
	ErrInvalidSystemIdentifier    = errors.New("invalid system identifier")
)

type (
	Client struct {
		self                        *network.Peer
		serverID                    peer.ID
		signer                      crypto.Signer
		rootVerifier                crypto.Verifier
		nodeID                      string
		systemIdentifier            []byte
		systemDescriptionRecordHash []byte
		roundNumber                 uint64
		previousHash                []byte
	}

	ClientConfiguration struct {
		Signer                      crypto.Signer
		NodeIdentifier              string
		SystemIdentifier            []byte
		SystemDescriptionRecordHash []byte
		HashAlgorithm               gocrypto.Hash
		RootChainRoundNumber        uint64
		PreviousHash                []byte
	}

	ServerConfiguration struct {
		RootChainID     peer.ID
		ServerAddresses []ma.Multiaddr
		RootVerifier    crypto.Verifier
	}
)

func (cc ClientConfiguration) IsValid() error {
	if cc.Signer == nil {
		return ErrSignerIsNil
	}
	if len(cc.SystemIdentifier) == 0 {
		return ErrInvalidSystemIdentifier
	}
	return nil
}

func NewClient(self *network.Peer, clientConf ClientConfiguration, serverConf ServerConfiguration) (*Client, error) {
	if self == nil {
		return nil, ErrClientPeerIsNil
	}

	self.Network().Peerstore().SetAddrs(serverConf.RootChainID, serverConf.ServerAddresses, peerstore.PermanentAddrTTL)
	return &Client{
		self:                        self,
		signer:                      clientConf.Signer,
		nodeID:                      clientConf.NodeIdentifier,
		systemIdentifier:            clientConf.SystemIdentifier,
		systemDescriptionRecordHash: clientConf.SystemDescriptionRecordHash,
		roundNumber:                 clientConf.RootChainRoundNumber,
		previousHash:                clientConf.PreviousHash,
		serverID:                    serverConf.RootChainID,
		rootVerifier:                serverConf.RootVerifier,
	}, nil
}

func (c *Client) SendSync(ctx context.Context, hash, blockHash, summaryValue []byte) (*P1Response, error) {
	req := &P1Request{
		SystemIdentifier: c.systemIdentifier,
		NodeIdentifier:   c.nodeID,
		RootRoundNumber:  c.roundNumber,
		InputRecord: &certificates.InputRecord{
			PreviousHash: c.previousHash,
			Hash:         hash,
			BlockHash:    blockHash,
			SummaryValue: summaryValue,
		},
	}
	err := req.Sign(c.signer)
	if err != nil {
		return nil, err
	}

	s, err := c.self.CreateStream(ctx, c.serverID, ProtocolP1)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	w := protocol.NewProtoBufWriter(s)
	err = w.Write(req)
	if err != nil {
		return nil, err
	}
	reader := protocol.NewProtoBufReader(s)
	response := &P1Response{}
	err = reader.Read(response)
	if err != nil {
		return nil, err
	}
	if response.Status == P1Response_OK {
		message := response.Message
		if err := message.IsValid(c.rootVerifier, gocrypto.SHA256, c.systemIdentifier, c.systemDescriptionRecordHash); err != nil {
			return nil, err
		}
		c.roundNumber = message.UnicitySeal.RootChainRoundNumber
		c.previousHash = message.InputRecord.Hash
	}

	return response, nil
}
