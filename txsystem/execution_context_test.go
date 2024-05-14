package txsystem

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/stretchr/testify/require"
)

func Test_newExecutionContext(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		_, verifier := testsig.CreateSignerAndVerifier(t)
		tb := testtb.NewTrustBase(t, verifier)
		txSys := NewTestGenericTxSystem(t, []Module{}, withCurrentRound(5))
		execCtx := newExecutionContext(txSys, tb)
		require.NotNil(t, execCtx)
		require.EqualValues(t, 5, execCtx.CurrentRound())
		tb, err := execCtx.TrustBase(0)
		require.NoError(t, err)
		require.NotNil(t, tb)
		u, err := execCtx.GetUnit(types.UnitID{2}, false)
		require.Error(t, err)
		require.Nil(t, u)
	})
}
