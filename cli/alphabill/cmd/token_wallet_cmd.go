package cmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	t "github.com/alphabill-org/alphabill/pkg/wallet/tokens"
	"github.com/spf13/cobra"
	"sort"
	"strconv"
)

const (
	cmdFlagSymbol              = "symbol"
	cmdFlagDecimals            = "decimals"
	cmdFlagParentType          = "parent-type"
	cmdFlagCreationInput       = "creation-input"
	cmdFlagSybtypeClause       = "subtype-clause"
	cmdFlagMintClause          = "mint-clause"
	cmdFlagInheritBearerClause = "inherit-bearer-clause"
	cmdFlagAmount              = "amount"
	cmdFlagType                = "type"
	cmdFlagTokenId             = "token-identifier"
	cmdFlagTokenURI            = "token-uri"
	cmdFlagTokenData           = "data"
	cmdFlagTokenDataUpdate     = "data-update"
	cmdFlagSync                = "sync"
)

func tokenCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "create and manage fungible and non-fungible tokens",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand like new-type, send etc")
		},
	}
	cmd.AddCommand(tokenCmdNewType(config))
	cmd.AddCommand(tokenCmdNewToken(config))
	cmd.AddCommand(tokenCmdTransfer(config))
	cmd.AddCommand(tokenCmdSend(config))
	cmd.AddCommand(tokenCmdDC(config))
	cmd.AddCommand(tokenCmdList(config))
	cmd.AddCommand(tokenCmdListTypes(config))
	cmd.AddCommand(tokenCmdSync(config))
	cmd.PersistentFlags().StringP(alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill uri to connect to")
	cmd.PersistentFlags().StringP(cmdFlagSync, "s", "true", "ensures wallet is up to date with the blockchain")
	return cmd
}

func tokenCmdNewType(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new-type",
		Short: "create new token type",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
		},
	}
	cmd.AddCommand(addCommonTypeFlags(tokenCmdNewTypeFungible(config)))
	cmd.AddCommand(addCommonTypeFlags(tokenCmdNewTypeNonFungible(config)))
	return cmd
}

func addCommonAccountFlags(cmd *cobra.Command) *cobra.Command {
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "which key to use for sending the transaction")
	addPasswordFlags(cmd)
	return cmd
}

func addCommonTypeFlags(cmd *cobra.Command) *cobra.Command {
	cmd.Flags().String(cmdFlagSymbol, "", "token symbol (mandatory)")
	_ = cmd.MarkFlagRequired(cmdFlagSymbol)
	cmd.Flags().String(cmdFlagParentType, "0x0", "unit identifier of a parent type-node in hexadecimal format, must start with 0x (optional)")
	cmd.Flags().String(cmdFlagCreationInput, "empty", "input to satisfy the parent types minting clause (mandatory with --parent-type)")
	cmd.Flags().String(cmdFlagSybtypeClause, "true", "predicate to control sub typing, values <true|false|ptpkh>, defaults to 'true' (optional)")
	cmd.Flags().String(cmdFlagMintClause, "ptpkh", "predicate to control minting of this type, values <true|false|ptpkh>, defaults to 'ptpkh' (optional)")
	cmd.Flags().String(cmdFlagInheritBearerClause, "true", "predicate that will be inherited by subtypes into their bearer clauses, values <true|false|ptpkh>, defaults to 'true' (optional)")
	return cmd
}

func tokenCmdNewTypeFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "create new fungible token type",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTypeFungible(cmd, config)
		},
	}
	cmd.Flags().Uint32(cmdFlagDecimals, 8, "token decimal (optional)")
	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
	_ = cmd.Flags().MarkHidden(cmdFlagType)
	return cmd
}

func execTokenCmdNewTypeFungible(cmd *cobra.Command, config *walletConfig) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	typeId, err := cmd.Flags().GetBytesHex(cmdFlagType)
	if err != nil {
		return err
	}
	symbol, err := cmd.Flags().GetString(cmdFlagSymbol)
	if err != nil {
		return err
	}
	decimals, err := cmd.Flags().GetUint32(cmdFlagDecimals)
	if err != nil {
		return err
	}
	a := &tokens.CreateFungibleTokenTypeAttributes{
		Symbol:                            symbol,
		DecimalPlaces:                     decimals,
		ParentTypeId:                      nil,
		SubTypeCreationPredicateSignature: nil,
		SubTypeCreationPredicate:          script.PredicateAlwaysFalse(),
		TokenCreationPredicate:            script.PredicateAlwaysTrue(),
		InvariantPredicate:                script.PredicateAlwaysTrue(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, err := tw.NewFungibleType(ctx, a, typeId)
	if err != nil {
		return err
	}
	consoleWriter.Println(fmt.Sprintf("Created new fungible token type with id=%X", id))
	return nil
}

func tokenCmdNewTypeNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "create new non-fungible token type",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTypeNonFungible(cmd, config)
		},
	}
	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
	_ = cmd.Flags().MarkHidden(cmdFlagType)
	return cmd
}

func execTokenCmdNewTypeNonFungible(cmd *cobra.Command, config *walletConfig) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	typeId, err := cmd.Flags().GetBytesHex(cmdFlagType)
	if err != nil {
		return err
	}
	symbol, err := cmd.Flags().GetString(cmdFlagSymbol)
	if err != nil {
		return err
	}
	a := &tokens.CreateNonFungibleTokenTypeAttributes{
		Symbol:                            symbol,
		ParentTypeId:                      nil,
		SubTypeCreationPredicateSignature: nil,
		SubTypeCreationPredicate:          script.PredicateAlwaysFalse(),
		TokenCreationPredicate:            script.PredicateAlwaysTrue(),
		InvariantPredicate:                script.PredicateAlwaysTrue(),
		DataUpdatePredicate:               script.PredicateAlwaysTrue(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, err := tw.NewNonFungibleType(ctx, a, typeId)
	if err != nil {
		return err
	}
	consoleWriter.Println(fmt.Sprintf("Created new NFT type with id=%X", id))
	return nil
}

func tokenCmdNewToken(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new",
		Short: "mint new token",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
		},
	}
	cmd.AddCommand(addCommonAccountFlags(tokenCmdNewTokenFungible(config)))
	cmd.AddCommand(addCommonAccountFlags(tokenCmdNewTokenNonFungible(config)))
	return cmd
}

func tokenCmdNewTokenFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "mint new fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTokenFungible(cmd, config)
		},
	}
	cmd.Flags().Uint64(cmdFlagAmount, 0, "amount")
	_ = cmd.MarkFlagRequired(cmdFlagAmount)
	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
	_ = cmd.MarkFlagRequired(cmdFlagType)
	cmd.Flags().StringArray(cmdFlagCreationInput, []string{"true"}, "input to satisfy the type's minting clause")
	return cmd
}

func execTokenCmdNewTokenFungible(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	amount, err := cmd.Flags().GetUint64(cmdFlagAmount)
	if err != nil {
		return err
	}
	typeId, err := cmd.Flags().GetBytesHex(cmdFlagType)
	if err != nil {
		return err
	}
	_, err = cmd.Flags().GetStringArray(cmdFlagCreationInput)
	if err != nil {
		return err
	}
	a := &tokens.MintFungibleTokenAttributes{
		Bearer:                          nil, // will be set in the wallet
		Type:                            typeId,
		Value:                           amount,
		TokenCreationPredicateSignature: script.PredicateArgumentEmpty(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, err := tw.NewFungibleToken(ctx, accountNumber, a)
	if err != nil {
		return err
	}

	consoleWriter.Println(fmt.Sprintf("Created new fungible token with id=%X", id))
	return nil
}

func tokenCmdNewTokenNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "mint new non-fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTokenNonFungible(cmd, config)
		},
	}
	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
	_ = cmd.MarkFlagRequired(cmdFlagType)
	cmd.Flags().String(cmdFlagTokenURI, "", "URI to associated resource, ie. jpg file on IPFS")
	cmd.Flags().BytesHex(cmdFlagTokenData, nil, "custom data (hex)")
	cmd.Flags().BytesHex(cmdFlagTokenDataUpdate, nil, "data update predicate (hex)")
	cmd.Flags().StringArray(cmdFlagCreationInput, []string{"true"}, "input to satisfy the type's minting clause")
	cmd.Flags().BytesHex(cmdFlagTokenId, nil, "unit identifier of token (hex)")
	_ = cmd.Flags().MarkHidden(cmdFlagTokenId)
	return cmd
}

func execTokenCmdNewTokenNonFungible(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	typeId, err := cmd.Flags().GetBytesHex(cmdFlagType)
	if err != nil {
		return err
	}
	tokenId, err := cmd.Flags().GetBytesHex(cmdFlagTokenId)
	if err != nil {
		return err
	}
	uri, err := cmd.Flags().GetString(cmdFlagTokenURI)
	if err != nil {
		return err
	}
	data, err := cmd.Flags().GetBytesHex(cmdFlagTokenData)
	if err != nil {
		return err
	}
	_, err = cmd.Flags().GetStringArray(cmdFlagCreationInput)
	if err != nil {
		return err
	}
	a := &tokens.MintNonFungibleTokenAttributes{
		Bearer:                          nil, // will be set in the wallet
		NftType:                         typeId,
		Uri:                             uri,
		Data:                            data,
		DataUpdatePredicate:             script.PredicateAlwaysTrue(),
		TokenCreationPredicateSignature: script.PredicateArgumentEmpty(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, err := tw.NewNFT(ctx, accountNumber, a, tokenId)
	if err != nil {
		return err
	}

	consoleWriter.Println(fmt.Sprintf("Created new fungible token with id=%X", id))
	return nil
}

func tokenCmdTransfer(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transfer",
		Short: "transfer a token by its id",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
		},
	}
	cmd.AddCommand(tokenCmdTransferFungible(config))
	return cmd
}

func tokenCmdTransferFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "transfer fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdTransferFungible(cmd, config)
		},
	}
	cmd.Flags().BytesHex(cmdFlagTokenId, nil, "unit identifier of token (hex)")
	_ = cmd.MarkFlagRequired(cmdFlagTokenId)
	cmd.Flags().StringP(addressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	_ = cmd.MarkFlagRequired(addressCmdName)
	return addCommonAccountFlags(cmd)
}

func execTokenCmdTransferFungible(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	tokenId, err := cmd.Flags().GetBytesHex(cmdFlagTokenId)
	if err != nil {
		return err
	}

	pubKey, err := getPubKeyBytes(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return tw.Transfer(ctx, accountNumber, tokenId, pubKey)
}

func tokenCmdSend(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send",
		Short: "send a token",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
		},
	}
	cmd.AddCommand(tokenCmdSendFungible(config))
	cmd.AddCommand(tokenCmdSendNonFungible(config))
	return cmd
}

func tokenCmdSendFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "send fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdSendFungible(cmd, config)
		},
	}
	cmd.Flags().Uint64(cmdFlagAmount, 0, "amount")
	_ = cmd.MarkFlagRequired(cmdFlagAmount)
	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
	_ = cmd.MarkFlagRequired(cmdFlagType)
	cmd.Flags().StringP(addressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	_ = cmd.MarkFlagRequired(addressCmdName)
	return addCommonAccountFlags(cmd)
}

// getPubKeyBytes returns 'nil' for flag value 'true', must be interpreted as 'always true' predicate
func getPubKeyBytes(cmd *cobra.Command) ([]byte, error) {
	pubKeyHex, err := cmd.Flags().GetString(addressCmdName)
	if err != nil {
		return nil, err
	}
	var pubKey []byte
	if pubKeyHex == "true" {
		pubKey = nil // this will assign 'always true' predicate
	} else {
		pk, ok := pubKeyHexToBytes(pubKeyHex)
		if !ok {
			return nil, errors.New(fmt.Sprintf("address in not in valid format: %s", pubKeyHex))
		}
		pubKey = pk
	}
	return pubKey, nil
}

func execTokenCmdSendFungible(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	typeId, err := cmd.Flags().GetBytesHex(cmdFlagType)
	if err != nil {
		return err
	}

	targetValue, err := cmd.Flags().GetUint64(cmdFlagAmount)
	if err != nil {
		return err
	}

	pubKey, err := getPubKeyBytes(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return tw.SendFungible(ctx, accountNumber, typeId, targetValue, pubKey)
}

func tokenCmdSendNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "transfer non-fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdSendNonFungible(cmd, config)
		},
	}
	cmd.Flags().BytesHex(cmdFlagTokenId, nil, "unit identifier of token (hex)")
	_ = cmd.MarkFlagRequired(cmdFlagTokenId)
	cmd.Flags().StringP(addressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	_ = cmd.MarkFlagRequired(addressCmdName)
	return addCommonAccountFlags(cmd)
}

func execTokenCmdSendNonFungible(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	tokenId, err := cmd.Flags().GetBytesHex(cmdFlagTokenId)
	if err != nil {
		return err
	}

	pubKey, err := getPubKeyBytes(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return tw.TransferNFT(ctx, accountNumber, tokenId, pubKey)
}

func tokenCmdDC(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collect-dust",
		Short: "join fungible tokens into one unit",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdDC(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdDC(cmd *cobra.Command, config *walletConfig) error {
	// TODO
	return nil
}

func tokenCmdSync(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "fetch latest blocks from a partition node",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdSync(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdSync(cmd *cobra.Command, config *walletConfig) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return tw.Sync(ctx)
}

func tokenCmdList(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists all available tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdList(cmd, config, t.Any)
		},
	}
	cmd.AddCommand(tokenCmdListFungible(config))
	cmd.AddCommand(tokenCmdListNonFungible(config))
	addPasswordFlags(cmd)
	cmd.PersistentFlags().IntP(keyCmdName, "k", 1, "which key to use for sending the transaction, 0 for tokens spendable by anyone, -1 for all tokens from all accounts")
	return cmd
}

func tokenCmdListFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "lists fungible tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdList(cmd, config, t.Token|t.Fungible)
		},
	}
	return cmd
}

func execTokenCmdList(cmd *cobra.Command, config *walletConfig, kind t.TokenKind) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	accountNumber, err := cmd.Flags().GetInt(keyCmdName)
	if err != nil {
		return err
	}
	res, err := tw.ListTokens(ctx, kind, accountNumber)
	if err != nil {
		return err
	}
	for accNr, toks := range res {
		var ownerKey string
		if accNr == 0 {
			ownerKey = "Tokens spendable by anyone:"
		} else {
			ownerKey = fmt.Sprintf("Tokens owned by account #%v", accNr)
		}
		consoleWriter.Println(ownerKey)
		sort.Slice(toks, func(i, j int) bool {
			// Fungible, then Non-fungible
			return toks[i].Kind < toks[j].Kind
		})
		for _, tok := range toks {
			if tok.IsFungible() {
				consoleWriter.Println(fmt.Sprintf("Id='%X', Symbol='%s', amount='%v', token-type='%X' (fungible)", tok.Id, tok.Symbol, tok.Amount, tok.TypeId))
			} else {
				consoleWriter.Println(fmt.Sprintf("Id='%X', Symbol='%s', token-type='%X', URI='%s' (non-fungible)", tok.Id, tok.Symbol, tok.TypeId, tok.Uri))
			}
		}
	}
	return nil
}

func tokenCmdListNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "lists non-fungible tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdList(cmd, config, t.Token|t.NonFungible)
		},
	}
	return cmd
}

func tokenCmdListTypes(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-types",
		Short: "lists token types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdListTypes(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdListTypes(cmd *cobra.Command, config *walletConfig) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, err := tw.ListTokenTypes(ctx)
	if err != nil {
		return err
	}
	for _, m := range res {
		consoleWriter.Println(m)
	}
	return nil
}

func initTokensWallet(cmd *cobra.Command, config *walletConfig) (*t.TokensWallet, error) {
	uri, err := cmd.Flags().GetString(alphabillUriCmdName)
	if err != nil {
		return nil, err
	}
	mw, err := loadExistingWallet(cmd, config.WalletHomeDir, uri)
	if err != nil {
		return nil, err
	}
	syncStr, err := cmd.Flags().GetString(cmdFlagSync)
	if err != nil {
		return nil, err
	}
	sync, err := strconv.ParseBool(syncStr)
	if err != nil {
		return nil, err
	}
	tw, err := t.Load(mw, sync)
	if err != nil {
		return nil, err
	}
	return tw, nil
}
