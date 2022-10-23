package cmd

import (
	"context"
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
	cmd.AddCommand(tokenCmdSend(config))
	cmd.AddCommand(tokenCmdDC(config))
	cmd.AddCommand(tokenCmdList(config))
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
		SubTypeCreationPredicate:          nil,
		TokenCreationPredicate:            nil,
		InvariantPredicate:                nil,
	}
	err = tw.NewFungibleType(a)
	if err != nil {
		return err
	}
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
	cmd.AddCommand(tokenCmdNewTokenFungible(config))
	cmd.AddCommand(tokenCmdNewTokenNonFungible(config))
	return cmd
}

func tokenCmdNewTokenFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "fungible",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTokenFungible(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdNewTokenFungible(cmd *cobra.Command, config *walletConfig) error {
	// TODO
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

func tokenCmdSend(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "new",
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
	return cmd
}

func execTokenCmdSendFungible(cmd *cobra.Command, config *walletConfig) error {
	// TODO
	return nil
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
	cmd.Flags().StringP(alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill uri to connect to")
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
	return tw.ListTokens(ctx, kind)
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
