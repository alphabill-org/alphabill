package wvm

import (
	"context"
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/hash"
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
	require.NoError(t, tokenenc.RegisterAuthProof(txsEnc.RegisterAuthProof))
	// checking P2PKH template is delegated to template engine
	tmpPred, err := templates.New(obs)
	require.NoError(t, err)
	templateEng, err := predicates.Dispatcher(tmpPred)
	require.NoError(t, err)
	// wasm engine to use for tests
	wvm, err := New(context.Background(), txsEnc, templateEng.Execute, obs)
	require.NoError(t, err)

	// owner of the time locked unit
	signerOwner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	verifierOwner, err := signerOwner.Verifier()
	require.NoError(t, err)
	pubKeyOwner, err := verifierOwner.MarshalPublicKey()
	require.NoError(t, err)

	// configuration of the predicate
	const lockedUntilDate uint64 = 1709683200
	cfgCBOR, err := types.Cbor.Marshal([]any{lockedUntilDate, hash.Sum256(pubKeyOwner)})
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
	ownerProof := testsig.NewAuthProofSignature(t, &txNFTTransfer, signerOwner)
	require.NoError(t, txNFTTransfer.SetAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
		OwnerProof: ownerProof,
	}))

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
		checkSpentGas(t, 2315, curGas-env.GasRemaining)
		require.EqualValues(t, 0x101, res)
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
		require.NoError(t, tx.SetAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			OwnerProof: ownerProof,
		}))

		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), timeLockWasm, ownerProof, predConf, &tx, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 5954, curGas-env.GasRemaining)
		require.EqualValues(t, 0x801, res)

		// invalid proof
		require.NoError(t, tx.SetAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			OwnerProof: nil,
		}))
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), timeLockWasm, ownerProof, predConf, &tx, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 5954, curGas-env.GasRemaining)
		require.EqualValues(t, 0x901, res)
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
		checkSpentGas(t, 5954, curGas-env.GasRemaining)
		require.EqualValues(t, 0, res)
	})
}
