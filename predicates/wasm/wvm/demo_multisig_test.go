package wvm

import (
	"context"
	"crypto/sha256"
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
//go:embed testdata/multi_sig.wasm
var multisigWasm []byte

func Test_multisig(t *testing.T) {
	checkSpentGas := func(t *testing.T, expected, actual uint64) {
		t.Helper()
		assert.Equal(t, expected, actual, "amount of gas spent, diff %d", max(expected, actual)-min(expected, actual))
	}

	// tx system unit/attribute encoder
	txsEnc := encoder.TXSystemEncoder{}
	require.NoError(t, tokenenc.RegisterTxAttributeEncoders(tokens.DefaultPartitionID, txsEnc.RegisterAttrEncoder))

	engine := func(t *testing.T) (*WasmVM, *mockTxContext) {
		t.Helper()

		obs := observability.Default(t)
		// checking P2PKH template is delegated to template engine
		tmpPred, err := templates.New(obs)
		require.NoError(t, err)
		templateEng, err := predicates.Dispatcher(tmpPred)
		require.NoError(t, err)
		// wasm engine to use for tests
		wvm, err := New(context.Background(), txsEnc, templateEng.Execute, obs)
		require.NoError(t, err)

		env := &mockTxContext{
			GasRemaining: 30000,
		}

		return wvm, env
	}

	// "current transaction" for the predicate is "transfer NFT"
	txNFTTransfer := types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			PartitionID: tokens.DefaultPartitionID,
			Type:        tokens.TransactionTypeTransferNFT,
			UnitID:      tokenid.NewNonFungibleTokenID(t),
		},
	}
	// note that we do not set the AuthProof of the transaction - thats because the predicate
	// is meant to be used as Bearer Predicate and when tx system evaluates it the OwnerProof
	// is extracted by the tx system and sent as predicate argument. So in the tests too we send
	// the proof as wvm.Exec argument and do not bother to update the tx order...

	signerA, pkhA := signerAndPKH(t)
	signerB, pkhB := signerAndPKH(t)
	signerC, pkhC := signerAndPKH(t)
	// the transfer has to be signed as if the bearer predicate is P2PKH
	ownerProofA := testsig.NewAuthProofSignature(t, &txNFTTransfer, signerA) // cbor of templates.P2pkh256Signature{}
	ownerProofB := testsig.NewAuthProofSignature(t, &txNFTTransfer, signerB)
	ownerProofC := testsig.NewAuthProofSignature(t, &txNFTTransfer, signerC)

	t.Run("not enough signatures", func(t *testing.T) {
		wvm, env := engine(t)

		// require all 3 signatures
		cfgCBOR, err := types.Cbor.Marshal([]any{3, pkhA, pkhB, pkhC})
		require.NoError(t, err)
		predConf := wasm.PredicateParams{Entrypoint: "multi_sig", Args: cfgCBOR}

		// second signature is missing
		ownerProofs, err := types.Cbor.Marshal([][]byte{ownerProofA, nil, ownerProofC})
		require.NoError(t, err)
		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 1518, curGas-env.GasRemaining)
		assert.EqualValues(t, 0x01, res)

		// last signature is missing
		ownerProofs, err = types.Cbor.Marshal([][]byte{ownerProofA, ownerProofB, nil})
		require.NoError(t, err)
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 2606, curGas-env.GasRemaining)
		assert.EqualValues(t, 0x01, res)

		// last signature is invalid - basically the same as previous but will take more gas
		// as the P2PKH template will run (it didn't for the missing signature).
		// repeat proof A instead of C so last proof should not verify
		ownerProofs, err = types.Cbor.Marshal([][]byte{ownerProofA, ownerProofB, ownerProofA})
		require.NoError(t, err)
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 3593, curGas-env.GasRemaining)
		assert.EqualValues(t, 0x101, res, "false because P2PKH evaluated to false")

		// invalid proof - not even valid data structure, causes P2PKH error
		ownerProofs, err = types.Cbor.Marshal([][]byte{make([]byte, len(ownerProofA)), ownerProofB, ownerProofC})
		require.NoError(t, err)
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 1417, curGas-env.GasRemaining)
		assert.EqualValues(t, 0x0201, res, "false because P2PKH returned error")
	})

	t.Run("require 2 of 3 signatures", func(t *testing.T) {
		wvm, env := engine(t)

		// require 2 out of 3
		cfgCBOR, err := types.Cbor.Marshal([]any{2, pkhA, pkhB, pkhC})
		require.NoError(t, err)
		predConf := wasm.PredicateParams{Entrypoint: "multi_sig", Args: cfgCBOR}

		// missing signature for B
		ownerProofs, err := types.Cbor.Marshal([][]byte{ownerProofA, nil, ownerProofC})
		require.NoError(t, err)
		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 2654, curGas-env.GasRemaining)
		assert.EqualValues(t, 0, res)

		// invalid signature for B, repeating A proof
		ownerProofs, err = types.Cbor.Marshal([][]byte{ownerProofA, ownerProofA, ownerProofC})
		require.NoError(t, err)
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 2505, curGas-env.GasRemaining)
		require.EqualValues(t, 0x101, res)
	})

	t.Run("invalid conf, threshold", func(t *testing.T) {
		// ie user made mistake when creating the predicate and provided invalid threshold configuration
		wvm, env := engine(t)

		ownerProofs, err := types.Cbor.Marshal([][]byte{ownerProofA, ownerProofB})
		require.NoError(t, err)

		// require one more than the number of pkh-s provided
		cfgCBOR, err := types.Cbor.Marshal([]any{3, pkhA, pkhB})
		require.NoError(t, err)
		predConf := wasm.PredicateParams{Entrypoint: "multi_sig", Args: cfgCBOR}
		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 2553, curGas-env.GasRemaining)
		assert.EqualValues(t, 0xff01, res, "predicate evaluates to false because the threshold > valid_signatures")

		// require zero signatures - if there is no invalid/missing signatures it evaluates to true
		// IOW returns true when either everyone signs or no one signs!
		cfgCBOR, err = types.Cbor.Marshal([]any{0, pkhA, pkhB})
		require.NoError(t, err)
		predConf = wasm.PredicateParams{Entrypoint: "multi_sig", Args: cfgCBOR}
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 2553, curGas-env.GasRemaining)
		assert.EqualValues(t, 0, res)

		// require 257 signatures - will overflow byte and one signature is required, will
		// evaluate to true! Note that 256 would be zero as byte so will act as previous test
		cfgCBOR, err = types.Cbor.Marshal([]any{257, pkhA, pkhB})
		require.NoError(t, err)
		predConf = wasm.PredicateParams{Entrypoint: "multi_sig", Args: cfgCBOR}
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 2553, curGas-env.GasRemaining)
		assert.EqualValues(t, 0, res)

		// not a number
		cfgCBOR, err = types.Cbor.Marshal([]any{"foo", pkhA, pkhB})
		require.NoError(t, err)
		predConf = wasm.PredicateParams{Entrypoint: "multi_sig", Args: cfgCBOR}
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 877, curGas-env.GasRemaining)
		require.EqualValues(t, 0x0C, res)
	})

	t.Run("invalid conf, pkh", func(t *testing.T) {
		wvm, env := engine(t)

		ownerProofs, err := types.Cbor.Marshal([][]byte{ownerProofA, ownerProofB})
		require.NoError(t, err)

		// no PKHs at all in the config
		cfgCBOR, err := types.Cbor.Marshal([]any{1})
		require.NoError(t, err)
		predConf := wasm.PredicateParams{Entrypoint: "multi_sig", Args: cfgCBOR}

		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 320, curGas-env.GasRemaining)
		require.EqualValues(t, 0x1C, res, "number of proofs does not equal to number of PKH-s")

		// nil PKH
		cfgCBOR, err = types.Cbor.Marshal([]any{1, nil, nil})
		require.NoError(t, err)
		predConf = wasm.PredicateParams{Entrypoint: "multi_sig", Args: cfgCBOR}
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 417, curGas-env.GasRemaining)
		require.EqualValues(t, 0x601, res, "false because of invalid PKH handle")
	})

	t.Run("proof count not equal to pkh count", func(t *testing.T) {
		// the count of proofs must equal to the pkh-s in the configuration, nil
		// is allowed to mark missing signature
		wvm, env := engine(t)

		cfgCBOR, err := types.Cbor.Marshal([]any{2, pkhA, pkhB, pkhC})
		require.NoError(t, err)
		predConf := wasm.PredicateParams{Entrypoint: "multi_sig", Args: cfgCBOR}

		// provide only 2 proofs while conf has 3 pkh-s
		ownerProofs, err := types.Cbor.Marshal([][]byte{ownerProofA, ownerProofC})
		require.NoError(t, err)

		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 320, curGas-env.GasRemaining)
		assert.EqualValues(t, 0x1C, res)

		// provide 4 proofs while conf has only 3 pkh-s
		ownerProofs, err = types.Cbor.Marshal([][]byte{ownerProofA, ownerProofB, ownerProofC, nil})
		require.NoError(t, err)

		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 320, curGas-env.GasRemaining)
		require.EqualValues(t, 0x1C, res)
	})

	t.Run("success 3 of 3", func(t *testing.T) {
		wvm, env := engine(t)

		ownerProofs, err := types.Cbor.Marshal([][]byte{ownerProofA, ownerProofB, ownerProofC})
		require.NoError(t, err)

		cfgCBOR, err := types.Cbor.Marshal([]any{3, pkhA, pkhB, pkhC})
		require.NoError(t, err)
		predConf := wasm.PredicateParams{Entrypoint: "multi_sig", Args: cfgCBOR}

		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), multisigWasm, ownerProofs, predConf, &txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 3641, curGas-env.GasRemaining)
		require.EqualValues(t, 0, res)
	})
}

func signerAndPKH(t *testing.T) (*abcrypto.InMemorySecp256K1Signer, []byte) {
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	verifier, err := signer.Verifier()
	require.NoError(t, err)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	h := sha256.Sum256(pubKey)
	return signer, h[:]
}
