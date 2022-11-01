package cmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	t "github.com/alphabill-org/alphabill/pkg/wallet/tokens"
	"github.com/spf13/cobra"
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
)

func tokenCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "token",
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
	return cmd
}

func tokenCmdNewType(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "new-type",
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
		Use: "fungible",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTypeFungible(cmd, config)
		},
	}
	cmd.Flags().Uint32(cmdFlagDecimals, 8, "token decimal (optional)")
	return cmd
}

func execTokenCmdNewTypeFungible(cmd *cobra.Command, config *walletConfig) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

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
	id, err := tw.NewFungibleType(ctx, a)
	if err != nil {
		return err
	}
	consoleWriter.Println(fmt.Sprintf("Created new fungible token type with id=%X", id))
	return nil
}

func tokenCmdNewTypeNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "non-fungible",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTypeNonFungible(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdNewTypeNonFungible(cmd *cobra.Command, config *walletConfig) error {
	// TODO
	return nil
}

func tokenCmdNewToken(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "new",
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
		Use: "fungible",
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
	//input, err := cmd.Flags().GetString(cmdFlagCreationInput)
	//if err != nil {
	//	return err
	//}
	a := &tokens.MintFungibleTokenAttributes{
		Bearer:                          nil,
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
		Use: "non-fungible",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTokenNonFungible(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdNewTokenNonFungible(cmd *cobra.Command, config *walletConfig) error {
	// TODO
	return nil
}

func tokenCmdTransfer(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "transfer",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
		},
	}
	cmd.AddCommand(tokenCmdTransferFungible(config))
	//cmd.AddCommand(tokenCmdTransferNonFungible(config))
	return cmd
}

func tokenCmdTransferFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "fungible",
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

	pubKeyHex, err := cmd.Flags().GetString(addressCmdName)
	if err != nil {
		return err
	}
	var pubKey []byte
	if pubKeyHex == "true" {
		pubKey = nil // this will assign 'always true' predicate
	} else {
		pk, ok := pubKeyHexToBytes(pubKeyHex)
		if !ok {
			return errors.New(fmt.Sprintf("address in not in valid format: %s", pubKeyHex))
		}
		pubKey = pk
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return tw.Transfer(ctx, accountNumber, tokenId, pubKey)
}

func tokenCmdSend(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "send",
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
		Use: "fungible",
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

	pubKeyHex, err := cmd.Flags().GetString(addressCmdName)
	if err != nil {
		return err
	}
	var pubKey []byte
	if pubKeyHex == "true" {
		pubKey = nil // this will assign 'always true' predicate
	} else {
		pk, ok := pubKeyHexToBytes(pubKeyHex)
		if !ok {
			return errors.New(fmt.Sprintf("address in not in valid format: %s", pubKeyHex))
		}
		pubKey = pk
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return tw.SendFungible(ctx, accountNumber, typeId, targetValue, pubKey)
}

func tokenCmdSendNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "non-fungible",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdSendNonFungible(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdSendNonFungible(cmd *cobra.Command, config *walletConfig) error {
	// TODO
	return nil
}

func tokenCmdDC(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "collect-dust",
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

func tokenCmdList(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "list",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
		},
	}
	cmd.AddCommand(tokenCmdListFungible(config))
	cmd.AddCommand(tokenCmdListNonFungible(config))
	return cmd
}

func tokenCmdSync(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "sync",
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

func tokenCmdListFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "fungible",
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
	res, err := tw.ListTokens(ctx, kind, t.AllAccounts)
	if err != nil {
		return err
	}
	for _, m := range res {
		consoleWriter.Println(m)
	}
	return nil
}

func tokenCmdListNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "non-fungible",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdList(cmd, config, t.Token|t.NonFungible)
		},
	}
	return cmd
}

func tokenCmdListTypes(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "list-types",
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

	tw, err := t.Load(mw)
	if err != nil {
		return nil, err
	}
	return tw, nil
}
