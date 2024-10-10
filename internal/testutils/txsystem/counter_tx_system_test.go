package testtxsystem

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
)

func TestRace(t *testing.T) {
	txSystem := &CounterTxSystem{}
	uc := &types.UnicityCertificate{Version: 1}
	go func() {
		_ = txSystem.Commit(uc)
	}()
	go func() {
		_ = txSystem.CommittedUC()
	}()
}
