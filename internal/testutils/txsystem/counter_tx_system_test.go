package testtxsystem

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-sdk/types"
)

func TestRace(t *testing.T) {
	txSystem := &CounterTxSystem{}
	uc := &types.UnicityCertificate{}
	go func() {
		_ = txSystem.Commit(uc)
	}()
	go func() {
		_ = txSystem.CommittedUC()
	}()
}
