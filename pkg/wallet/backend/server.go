package backend

import (
	"context"
	"errors"
	"net"
	"net/http"

	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
)

type (
	WalletBackendHttpServer struct {
		Handler RequestHandler
		server  *http.Server
	}

	WalletBackendService interface {
		GetBills(pubKey []byte) ([]*Bill, error)
		GetBlockProof(unitId []byte) (*BlockProof, error)
	}
)

func NewHttpServer(addr string, service WalletBackendService) *WalletBackendHttpServer {
	handler := &RequestHandler{service: service}
	server := &WalletBackendHttpServer{server: &http.Server{Addr: addr, Handler: handler.router()}}
	return server
}

func (s *WalletBackendHttpServer) Start() error {
	wlog.Info("starting http server on " + s.server.Addr)
	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}
	go func() {
		err := s.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			wlog.Info("http server closed")
		} else {
			wlog.Error("http server error: ", err)
		}
	}()
	return nil
}

func (s *WalletBackendHttpServer) Shutdown(ctx context.Context) error {
	wlog.Info("shutting down http server on " + s.server.Addr)
	return s.server.Shutdown(ctx)
}
