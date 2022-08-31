package backend

import (
	"context"
	"errors"
	"net"
	"net/http"
)

type (
	WalletBackendHttpServer struct {
		Handler RequestHandler
		server  *http.Server
	}

	WalletBackendService interface {
		GetBills(pubKey []byte) []*bill
		GetBlockProof(unitId []byte) *blockProof
	}
)

func NewHttpServer(addr string, service WalletBackendService) *WalletBackendHttpServer {
	handler := &RequestHandler{service: service}
	server := &WalletBackendHttpServer{server: &http.Server{Addr: addr, Handler: handler.router()}}
	return server
}

func (s *WalletBackendHttpServer) Start() error {
	log.Info("starting http server on " + s.server.Addr)
	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}
	go func() {
		err := s.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			log.Info("http server closed")
		} else {
			log.Error("http server error %v", err)
		}
	}()
	return nil
}

func (s *WalletBackendHttpServer) Shutdown(ctx context.Context) error {
	log.Info("shutting down http server on " + s.server.Addr)
	return s.server.Shutdown(ctx)
}
