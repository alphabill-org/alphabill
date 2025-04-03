package wvm

import (
	"context"
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/wasm"
	tokenid "github.com/alphabill-org/alphabill-go-base/testutils/tokens"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
	tokenenc "github.com/alphabill-org/alphabill/txsystem/tokens/encoder"
)

// the source code of the predicate is in the Rust SDK repo under examples
//
//go:embed testdata/time_lock.wasm
var timeLockWasm []byte

func Test_time_lock(t *testing.T) {
	checkSpentGas := func(t *testing.T, expected, actual uint64) {
		t.Helper()
		assert.Equal(t, expected, actual, "amount of gas spent, diff %d", max(expected, actual)-min(expected, actual))
	}
	obs := observability.Default(t)

	// tx system unit/attribute encoder
	txsEnc := encoder.TXSystemEncoder{}
	require.NoError(t, tokenenc.RegisterTxAttributeEncoders(txsEnc.RegisterAttrEncoder))
	// checking P2PKH template is delegated to template engine
	tmpPred, err := templates.New(obs)
	require.NoError(t, err)
	templateEng, err := predicates.Dispatcher(tmpPred)
	require.NoError(t, err)
	// wasm engine to use for tests
	wvm, err := New(context.Background(), txsEnc, templateEng.Execute, obs)
	require.NoError(t, err)

	// owner of the time locked unit
	signerOwner, ownerPKH := signerAndPKH(t)

	// configuration of the predicate
	const lockedUntilDate uint64 = 1709683200
	cfgCBOR, err := types.Cbor.Marshal([]any{lockedUntilDate, ownerPKH})
	require.NoError(t, err)
	predConf := wasm.PredicateParams{Entrypoint: "time_lock", Args: cfgCBOR}

	// "current transaction" for the predicate is "transfer NFT"
	txNFTTransfer := types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			PartitionID: tokens.DefaultPartitionID,
			Type:        tokens.TransactionTypeTransferNFT,
			UnitID:      tokenid.NewNonFungibleTokenID(t),
		},
	}
	// the transfer has to be signed as if the bearer predicate is P2PKH
	// note however that we do not set the AuthProof of the transaction - thats because the
	// predicate is meant to be used as Bearer Predicate and when tx system evaluates it the
	// OwnerProof is extracted by the tx system and sent as predicate argument. So in the tests
	// too we send the proof as wvm.Exec argument and do not bother to update the tx order...
	ownerProof := testsig.NewAuthProofSignature(t, &txNFTTransfer, signerOwner)

	t.Run("transfer before unlock date", func(t *testing.T) {
		env := &mockTxContext{
			committedUC: func() *types.UnicityCertificate {
				return &types.UnicityCertificate{
					UnicitySeal: &types.UnicitySeal{Timestamp: lockedUntilDate - 1},
				}
			},
			GasRemaining: 30000,
		}

		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), timeLockWasm, ownerProof, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 285, curGas-env.GasRemaining)
		require.EqualValues(t, 0xff01, res)
	})

	t.Run("not the owner", func(t *testing.T) {
		// someone other than current owner attempts to transfer (after unlock date)
		env := &mockTxContext{
			committedUC: func() *types.UnicityCertificate {
				return &types.UnicityCertificate{
					UnicitySeal: &types.UnicitySeal{Timestamp: lockedUntilDate + 1},
				}
			},
			GasRemaining: 30000,
		}

		signer, err := abcrypto.NewInMemorySecp256K1Signer()
		require.NoError(t, err)

		tx := txNFTTransfer
		// correct P2PKH proof but not by the owner
		ownerProof := testsig.NewAuthProofSignature(t, &txNFTTransfer, signer)

		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), timeLockWasm, ownerProof, predConf, &tx, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 1331, curGas-env.GasRemaining)
		require.EqualValues(t, 0x101, res)

		// invalid proof (nil)
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), timeLockWasm, nil, predConf, &tx, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 331, curGas-env.GasRemaining)
		require.EqualValues(t, 0x701, res)
	})

	t.Run("success", func(t *testing.T) {
		// owner transfers after unlock date
		env := &mockTxContext{
			committedUC: func() *types.UnicityCertificate {
				return &types.UnicityCertificate{
					UnicitySeal: &types.UnicitySeal{Timestamp: lockedUntilDate},
				}
			},
			GasRemaining: 30000,
		}

		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), timeLockWasm, ownerProof, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 1331, curGas-env.GasRemaining)
		require.EqualValues(t, 0, res)
	})

	t.Run("invalid config", func(t *testing.T) {
		env := &mockTxContext{
			committedUC: func() *types.UnicityCertificate {
				return &types.UnicityCertificate{
					UnicitySeal: &types.UnicitySeal{Timestamp: lockedUntilDate},
				}
			},
			GasRemaining: 30000,
		}

		// date is not uint (unix timestamp)
		cfgCBOR, err := types.Cbor.Marshal([]any{"2025-04-01 12:00:00", ownerPKH})
		require.NoError(t, err)
		predConf := wasm.PredicateParams{Entrypoint: "time_lock", Args: cfgCBOR}
		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), timeLockWasm, ownerProof, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 1194, curGas-env.GasRemaining)
		require.EqualValues(t, 0xc01, res)

		// missing owner PKH
		cfgCBOR, err = types.Cbor.Marshal([]any{lockedUntilDate})
		require.NoError(t, err)
		predConf = wasm.PredicateParams{Entrypoint: "time_lock", Args: cfgCBOR}
		start, curGas = time.Now(), env.GasRemaining
		_, err = wvm.Exec(context.Background(), timeLockWasm, ownerProof, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.ErrorContains(t, err, `calling time_lock returned error: wasm error: unreachable`)
		checkSpentGas(t, 351, curGas-env.GasRemaining)
	})
}
