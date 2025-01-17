package types

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	"github.com/stretchr/testify/require"
)

type stateInfo struct {
}

func (s *stateInfo) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return nil, fmt.Errorf("unit does not exist")
}

func (s *stateInfo) CommittedUC() *types.UnicityCertificate { return nil }

func (s *stateInfo) CurrentRound() uint64 {
	return 1
}

func Test_newExecutionContext(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		_, verifier := testsig.CreateSignerAndVerifier(t)
		tb := testtb.NewTrustBase(t, verifier)
		info := &stateInfo{}
		execCtx := NewExecutionContext(info, NewMockFeeModule(), tb, 10)
		require.NotNil(t, execCtx)
		require.EqualValues(t, 1, execCtx.CurrentRound())
		tb, err := execCtx.TrustBase(0)
		require.NoError(t, err)
		require.NotNil(t, tb)
		u, err := execCtx.GetUnit(types.UnitID{2}, false)
		require.Error(t, err)
		require.Nil(t, u)
	})
}
