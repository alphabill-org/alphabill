package cmd

//
//import (
//	"context"
//	"fmt"
//	"sort"
//	"strconv"
//
//	"github.com/alphabill-org/alphabill/pkg/client"
//	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/legacywallet"
//
//	ttxs "github.com/alphabill-org/alphabill/internal/txsystem/tokens"
//	"github.com/spf13/cobra"
//)
//
//type runTokenLegacyListTypesCmd func(cmd *cobra.Command, config *walletConfig, kind legacywallet.TokenKind) error
//type runTokenLegacyListCmd func(cmd *cobra.Command, config *walletConfig, kind legacywallet.TokenKind, accountNumber *int) error
//
//func legacyTokenCmd(config *walletConfig) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "token-legacy",
//		Short: "create and manage fungible and non-fungible tokens",
//		Run: func(cmd *cobra.Command, args []string) {
//			consoleWriter.Println("Error: must specify a subcommand like new-type, send etc")
//		},
//	}
//	cmd.AddCommand(tokenLegacyCmdNewType(config))
//	cmd.AddCommand(tokenLegacyCmdNewToken(config))
//	cmd.AddCommand(tokenLegacyCmdUpdateNFTData(config))
//	cmd.AddCommand(tokenLegacyCmdSend(config))
//	cmd.AddCommand(tokenLegacyCmdDC(config))
//	cmd.AddCommand(tokenLegacyCmdList(config, execTokenLegacyCmdList))
//	cmd.AddCommand(tokenLegacyCmdListTypes(config, execTokenLegacyCmdListTypes))
//	cmd.AddCommand(tokenLegacyCmdSync(config))
//	cmd.PersistentFlags().StringP(alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill uri to connect to")
//	cmd.PersistentFlags().StringP(cmdFlagSync, "s", "true", "ensures wallet is up to date with the blockchain")
//	return cmd
//}
//
//func tokenLegacyCmdNewType(config *walletConfig) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "new-type",
//		Short: "create new token type",
//		Run: func(cmd *cobra.Command, args []string) {
//			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
//		},
//	}
//	cmd.AddCommand(addCommonTypeFlags(tokenLegacyCmdNewTypeFungible(config)))
//	cmd.AddCommand(addCommonTypeFlags(tokenLegacyCmdNewTypeNonFungible(config)))
//	return cmd
//}
//
//func tokenLegacyCmdNewTypeFungible(config *walletConfig) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "fungible",
//		Short: "create new fungible token type",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return execTokenLegacyCmdNewTypeFungible(cmd, config)
//		},
//	}
//	cmd.Flags().Uint32(cmdFlagDecimals, 8, "token decimal (optional)")
//	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
//	_ = cmd.Flags().MarkHidden(cmdFlagType)
//	return cmd
//}
//
//func execTokenLegacyCmdNewTypeFungible(cmd *cobra.Command, config *walletConfig) error {
//	tw, err := initTokensLegacyWallet(cmd, config)
//	if err != nil {
//		return err
//	}
//	defer tw.Shutdown()
//
//	typeId, err := getHexFlag(cmd, cmdFlagType)
//	if err != nil {
//		return err
//	}
//	symbol, err := cmd.Flags().GetString(cmdFlagSymbol)
//	if err != nil {
//		return err
//	}
//	decimals, err := cmd.Flags().GetUint32(cmdFlagDecimals)
//	if err != nil {
//		return err
//	}
//	if decimals > maxDecimalPlaces {
//		return fmt.Errorf("argument \"%v\" for \"--decimals\" flag is out of range, max value %v", decimals, maxDecimalPlaces)
//	}
//	am := tw.GetAccountManager()
//	parentType, creationInputs, err := readParentTypeInfo(cmd, am)
//	if err != nil {
//		return err
//	}
//	subTypeCreationPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagSybTypeClause, am)
//	if err != nil {
//		return err
//	}
//	mintTokenPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagMintClause, am)
//	if err != nil {
//		return err
//	}
//	invariantPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagInheritBearerClause, am)
//	if err != nil {
//		return err
//	}
//	a := &ttxs.CreateFungibleTokenTypeAttributes{
//		Symbol:                             symbol,
//		DecimalPlaces:                      decimals,
//		ParentTypeId:                       parentType,
//		SubTypeCreationPredicateSignatures: nil, // will be filled by the wallet
//		SubTypeCreationPredicate:           subTypeCreationPredicate,
//		TokenCreationPredicate:             mintTokenPredicate,
//		InvariantPredicate:                 invariantPredicate,
//	}
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	id, err := tw.NewFungibleType(ctx, a, typeId, creationInputs)
//	if err != nil {
//		return err
//	}
//	consoleWriter.Println(fmt.Sprintf("Created new fungible token type with id=%X", id))
//	return nil
//}
//
//func tokenLegacyCmdNewTypeNonFungible(config *walletConfig) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "non-fungible",
//		Short: "create new non-fungible token type",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return execTokenLegacyCmdNewTypeNonFungible(cmd, config)
//		},
//	}
//	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
//	_ = cmd.Flags().MarkHidden(cmdFlagType)
//	cmd.Flags().String(cmdFlagTokenDataUpdateClause, predicateTrue, "data update predicate, values <true|false|ptpkh>, defaults to 'true' (optional)")
//	return cmd
//}
//
//func execTokenLegacyCmdNewTypeNonFungible(cmd *cobra.Command, config *walletConfig) error {
//	tw, err := initTokensLegacyWallet(cmd, config)
//	if err != nil {
//		return err
//	}
//	defer tw.Shutdown()
//
//	typeId, err := getHexFlag(cmd, cmdFlagType)
//	if err != nil {
//		return err
//	}
//	symbol, err := cmd.Flags().GetString(cmdFlagSymbol)
//	if err != nil {
//		return err
//	}
//	am := tw.GetAccountManager()
//	parentType, creationInputs, err := readParentTypeInfo(cmd, am)
//	if err != nil {
//		return err
//	}
//	subTypeCreationPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagSybTypeClause, am)
//	if err != nil {
//		return err
//	}
//	mintTokenPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagMintClause, am)
//	if err != nil {
//		return err
//	}
//	dataUpdatePredicate, err := parsePredicateClauseCmd(cmd, cmdFlagTokenDataUpdateClause, am)
//	if err != nil {
//		return err
//	}
//	invariantPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagInheritBearerClause, am)
//	if err != nil {
//		return err
//	}
//	a := &ttxs.CreateNonFungibleTokenTypeAttributes{
//		Symbol:                             symbol,
//		ParentTypeId:                       parentType,
//		SubTypeCreationPredicateSignatures: nil, // will be filled by the wallet
//		SubTypeCreationPredicate:           subTypeCreationPredicate,
//		TokenCreationPredicate:             mintTokenPredicate,
//		InvariantPredicate:                 invariantPredicate,
//		DataUpdatePredicate:                dataUpdatePredicate,
//	}
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	id, err := tw.NewNonFungibleType(ctx, a, typeId, creationInputs)
//	if err != nil {
//		return err
//	}
//	consoleWriter.Println(fmt.Sprintf("Created new NFT type with id=%X", id))
//	return nil
//}
//
//func tokenLegacyCmdNewToken(config *walletConfig) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "new",
//		Short: "mint new token",
//		Run: func(cmd *cobra.Command, args []string) {
//			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
//		},
//	}
//	cmd.AddCommand(addCommonAccountFlags(tokenLegacyCmdNewTokenFungible(config)))
//	cmd.AddCommand(addCommonAccountFlags(tokenLegacyCmdNewTokenNonFungible(config)))
//	return cmd
//}
//
//func tokenLegacyCmdNewTokenFungible(config *walletConfig) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "fungible",
//		Short: "mint new fungible token",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return execTokenLegacyCmdNewTokenFungible(cmd, config)
//		},
//	}
//	cmd.Flags().String(cmdFlagAmount, "", "amount, must be bigger than 0 and is interpreted according to token type precision (decimals)")
//	err := cmd.MarkFlagRequired(cmdFlagAmount)
//	if err != nil {
//		return nil
//	}
//	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
//	err = cmd.MarkFlagRequired(cmdFlagType)
//	if err != nil {
//		return nil
//	}
//	cmd.Flags().StringSlice(cmdFlagMintClauseInput, []string{predicatePtpkh}, "input to satisfy the type's minting clause")
//	return cmd
//}
//
//func execTokenLegacyCmdNewTokenFungible(cmd *cobra.Command, config *walletConfig) error {
//	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
//	if err != nil {
//		return err
//	}
//	tw, err := initTokensLegacyWallet(cmd, config)
//	if err != nil {
//		return err
//	}
//	defer tw.Shutdown()
//
//	amountStr, err := cmd.Flags().GetString(cmdFlagAmount)
//	if err != nil {
//		return err
//	}
//	typeId, err := getHexFlag(cmd, cmdFlagType)
//	if err != nil {
//		return err
//	}
//	ci, err := readPredicateInput(cmd, cmdFlagMintClauseInput, tw.GetAccountManager())
//	if err != nil {
//		return err
//	}
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	tt, err := tw.GetTokenType(ctx, typeId)
//	if err != nil {
//		return err
//	}
//	// convert amount from string to uint64
//	amount, err := stringToAmount(amountStr, tt.DecimalPlaces)
//	if err != nil {
//		return err
//	}
//	if amount == 0 {
//		return fmt.Errorf("invalid parameter \"%s\" for \"--amount\": 0 is not valid amount", amountStr)
//	}
//
//	a := &ttxs.MintFungibleTokenAttributes{
//		Bearer:                           nil, // will be set in the wallet
//		Type:                             typeId,
//		Value:                            amount,
//		TokenCreationPredicateSignatures: nil, // will be filled by the wallet
//	}
//
//	id, err := tw.NewFungibleToken(ctx, accountNumber, a, ci)
//	if err != nil {
//		return err
//	}
//
//	consoleWriter.Println(fmt.Sprintf("Created new fungible token with id=%X", id))
//	return nil
//}
//
//func tokenLegacyCmdNewTokenNonFungible(config *walletConfig) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "non-fungible",
//		Short: "mint new non-fungible token",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return execTokenLegacyCmdNewTokenNonFungible(cmd, config)
//		},
//	}
//	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
//	err := cmd.MarkFlagRequired(cmdFlagType)
//	if err != nil {
//		return nil
//	}
//	cmd.Flags().String(cmdFlagTokenURI, "", "URI to associated resource, ie. jpg file on IPFS")
//	cmd.Flags().BytesHex(cmdFlagTokenData, nil, "custom data (hex)")
//	cmd.Flags().String(cmdFlagTokenDataFile, "", "data file (max 64Kb) path")
//	cmd.MarkFlagsMutuallyExclusive(cmdFlagTokenData, cmdFlagTokenDataFile)
//	cmd.Flags().String(cmdFlagTokenDataUpdateClause, predicateTrue, "data update predicate, values <true|false|ptpkh>, defaults to 'true' (optional)")
//	cmd.Flags().StringSlice(cmdFlagMintClauseInput, []string{predicatePtpkh}, "input to satisfy the type's minting clause")
//	cmd.Flags().BytesHex(cmdFlagTokenId, nil, "unit identifier of token (hex)")
//	_ = cmd.Flags().MarkHidden(cmdFlagTokenId)
//	return cmd
//}
//
//func execTokenLegacyCmdNewTokenNonFungible(cmd *cobra.Command, config *walletConfig) error {
//	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
//	if err != nil {
//		return err
//	}
//	tw, err := initTokensLegacyWallet(cmd, config)
//	if err != nil {
//		return err
//	}
//	defer tw.Shutdown()
//
//	typeId, err := getHexFlag(cmd, cmdFlagType)
//	if err != nil {
//		return err
//	}
//	tokenId, err := getHexFlag(cmd, cmdFlagTokenId)
//	if err != nil {
//		return err
//	}
//	uri, err := cmd.Flags().GetString(cmdFlagTokenURI)
//	if err != nil {
//		return err
//	}
//	data, err := readNFTData(cmd, false)
//	if err != nil {
//		return err
//	}
//	am := tw.GetAccountManager()
//	ci, err := readPredicateInput(cmd, cmdFlagMintClauseInput, am)
//	if err != nil {
//		return err
//	}
//	dataUpdatePredicate, err := parsePredicateClauseCmd(cmd, cmdFlagTokenDataUpdateClause, am)
//	if err != nil {
//		return err
//	}
//	a := &ttxs.MintNonFungibleTokenAttributes{
//		Bearer:                           nil, // will be set in the wallet
//		NftType:                          typeId,
//		Uri:                              uri,
//		Data:                             data,
//		DataUpdatePredicate:              dataUpdatePredicate,
//		TokenCreationPredicateSignatures: nil, // will be set in the wallet
//	}
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	id, err := tw.NewNFT(ctx, accountNumber, a, tokenId, ci)
//	if err != nil {
//		return err
//	}
//
//	consoleWriter.Println(fmt.Sprintf("Created new non-fungible token with id=%X", id))
//	return nil
//}
//
//func tokenLegacyCmdSend(config *walletConfig) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "send",
//		Short: "send a token",
//		Run: func(cmd *cobra.Command, args []string) {
//			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
//		},
//	}
//	cmd.AddCommand(tokenLegacyCmdSendFungible(config))
//	cmd.AddCommand(tokenLegacyCmdSendNonFungible(config))
//	return cmd
//}
//
//func tokenLegacyCmdSendFungible(config *walletConfig) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "fungible",
//		Short: "send fungible token",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return execTokenLegacyCmdSendFungible(cmd, config)
//		},
//	}
//	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the type's minting clause")
//	cmd.Flags().String(cmdFlagAmount, "", "amount, must be bigger than 0 and is interpreted according to token type precision (decimals)")
//	err := cmd.MarkFlagRequired(cmdFlagAmount)
//	if err != nil {
//		return nil
//	}
//	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
//	err = cmd.MarkFlagRequired(cmdFlagType)
//	if err != nil {
//		return nil
//	}
//	cmd.Flags().StringP(addressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
//	err = cmd.MarkFlagRequired(addressCmdName)
//	if err != nil {
//		return nil
//	}
//	return addCommonAccountFlags(cmd)
//}
//
//func execTokenLegacyCmdSendFungible(cmd *cobra.Command, config *walletConfig) error {
//	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
//	if err != nil {
//		return err
//	}
//	tw, err := initTokensLegacyWallet(cmd, config)
//	if err != nil {
//		return err
//	}
//	defer tw.Shutdown()
//
//	typeId, err := getHexFlag(cmd, cmdFlagType)
//	if err != nil {
//		return err
//	}
//
//	amountStr, err := cmd.Flags().GetString(cmdFlagAmount)
//	if err != nil {
//		return err
//	}
//
//	pubKey, err := getPubKeyBytes(cmd, addressCmdName)
//	if err != nil {
//		return err
//	}
//
//	ib, err := readPredicateInput(cmd, cmdFlagInheritBearerClauseInput, tw.GetAccountManager())
//	if err != nil {
//		return err
//	}
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	// get token type and convert amount string
//	tt, err := tw.GetTokenType(ctx, typeId)
//	if err != nil {
//		return err
//	}
//	// convert amount from string to uint64
//	targetValue, err := stringToAmount(amountStr, tt.DecimalPlaces)
//	if err != nil {
//		return err
//	}
//	if targetValue == 0 {
//		return fmt.Errorf("invalid parameter \"%s\" for \"--amount\": 0 is not valid amount", amountStr)
//	}
//	return tw.SendFungible(ctx, accountNumber, typeId, targetValue, pubKey, ib)
//}
//
//func tokenLegacyCmdSendNonFungible(config *walletConfig) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "non-fungible",
//		Short: "transfer non-fungible token",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return execTokenLegacyCmdSendNonFungible(cmd, config)
//		},
//	}
//	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the type's minting clause")
//	cmd.Flags().BytesHex(cmdFlagTokenId, nil, "unit identifier of token (hex)")
//	err := cmd.MarkFlagRequired(cmdFlagTokenId)
//	if err != nil {
//		return nil
//	}
//	cmd.Flags().StringP(addressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
//	err = cmd.MarkFlagRequired(addressCmdName)
//	if err != nil {
//		return nil
//	}
//	return addCommonAccountFlags(cmd)
//}
//
//func execTokenLegacyCmdSendNonFungible(cmd *cobra.Command, config *walletConfig) error {
//	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
//	if err != nil {
//		return err
//	}
//	tw, err := initTokensLegacyWallet(cmd, config)
//	if err != nil {
//		return err
//	}
//	defer tw.Shutdown()
//
//	tokenId, err := getHexFlag(cmd, cmdFlagTokenId)
//	if err != nil {
//		return err
//	}
//
//	pubKey, err := getPubKeyBytes(cmd, addressCmdName)
//	if err != nil {
//		return err
//	}
//
//	ib, err := readPredicateInput(cmd, cmdFlagInheritBearerClauseInput, tw.GetAccountManager())
//	if err != nil {
//		return err
//	}
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	return tw.TransferNFT(ctx, accountNumber, tokenId, pubKey, ib)
//}
//
//func tokenLegacyCmdDC(config *walletConfig) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "collect-dust",
//		Short: "join fungible tokens into one unit",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return execTokenLegacyCmdDC(cmd, config)
//		},
//	}
//	return cmd
//}
//
//func execTokenLegacyCmdDC(cmd *cobra.Command, config *walletConfig) error {
//	// TODO
//	return nil
//}
//
//func tokenLegacyCmdSync(config *walletConfig) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "sync",
//		Short: "fetch latest blocks from a partition node",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return execTokenLegacyCmdSync(cmd, config)
//		},
//	}
//	return cmd
//}
//
//func execTokenLegacyCmdSync(cmd *cobra.Command, config *walletConfig) error {
//	tw, err := initTokensLegacyWallet(cmd, config)
//	if err != nil {
//		return err
//	}
//	defer tw.Shutdown()
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	return tw.Sync(ctx)
//}
//
//func tokenLegacyCmdUpdateNFTData(config *walletConfig) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "update",
//		Short: "update the data field on a non-fungible token",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return execTokenLegacyCmdUpdateNFTData(cmd, config)
//		},
//	}
//	cmd.Flags().BytesHex(cmdFlagTokenId, nil, "token identifier (hex)")
//	err := cmd.MarkFlagRequired(cmdFlagTokenId)
//	if err != nil {
//		panic(err)
//	}
//	cmd.Flags().BytesHex(cmdFlagTokenData, nil, "custom data (hex)")
//	cmd.Flags().String(cmdFlagTokenDataFile, "", "data file (max 64Kb) path")
//	cmd.MarkFlagsMutuallyExclusive(cmdFlagTokenData, cmdFlagTokenDataFile)
//	cmd.Flags().StringSlice(cmdFlagTokenDataUpdateClauseInput, []string{predicateTrue, predicateTrue}, "input to satisfy the data-update clauses")
//	return addCommonAccountFlags(cmd)
//}
//
//func execTokenLegacyCmdUpdateNFTData(cmd *cobra.Command, config *walletConfig) error {
//	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
//	if err != nil {
//		return err
//	}
//
//	tw, err := initTokensLegacyWallet(cmd, config)
//	if err != nil {
//		return err
//	}
//	defer tw.Shutdown()
//
//	tokenId, err := getHexFlag(cmd, cmdFlagTokenId)
//	if err != nil {
//		return err
//	}
//
//	data, err := readNFTData(cmd, true)
//	if err != nil {
//		return err
//	}
//
//	du, err := readPredicateInput(cmd, cmdFlagTokenDataUpdateClauseInput, tw.GetAccountManager())
//	if err != nil {
//		return err
//	}
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	return tw.UpdateNFTData(ctx, accountNumber, tokenId, data, du)
//}
//
//func tokenLegacyCmdList(config *walletConfig, runner runTokenLegacyListCmd) *cobra.Command {
//	accountNumber := -1
//	cmd := &cobra.Command{
//		Use:   "list",
//		Short: "lists all available tokens",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return runner(cmd, config, legacywallet.Any, &accountNumber)
//		},
//	}
//	// add persistent password flags
//	cmd.PersistentFlags().BoolP(passwordPromptCmdName, "p", false, passwordPromptUsage)
//	cmd.PersistentFlags().String(passwordArgCmdName, "", passwordArgUsage)
//	// add sub commands
//	cmd.AddCommand(tokenLegacyCmdListFungible(config, runner, &accountNumber))
//	cmd.AddCommand(tokenLegacyCmdListNonFungible(config, runner, &accountNumber))
//	cmd.PersistentFlags().IntVarP(&accountNumber, keyCmdName, "k", -1, "which key to use for sending the transaction, 0 for tokens spendable by anyone, -1 for all tokens from all accounts")
//	return cmd
//}
//
//func tokenLegacyCmdListFungible(config *walletConfig, runner runTokenLegacyListCmd, accountNumber *int) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "fungible",
//		Short: "lists fungible tokens",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return runner(cmd, config, legacywallet.FungibleToken, accountNumber)
//		},
//	}
//	return cmd
//}
//
//func tokenLegacyCmdListNonFungible(config *walletConfig, runner runTokenLegacyListCmd, accountNumber *int) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "non-fungible",
//		Short: "lists non-fungible tokens",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return runner(cmd, config, legacywallet.NonFungibleToken, accountNumber)
//		},
//	}
//	return cmd
//}
//
//func execTokenLegacyCmdList(cmd *cobra.Command, config *walletConfig, kind legacywallet.TokenKind, accountNumber *int) error {
//	tw, err := initTokensLegacyWallet(cmd, config)
//	if err != nil {
//		return err
//	}
//	defer tw.Shutdown()
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	res, err := tw.ListTokens(ctx, kind, *accountNumber)
//	if err != nil {
//		return err
//	}
//
//	accounts := make([]uint64, 0, len(res))
//	for accNr := range res {
//		accounts = append(accounts, accNr)
//	}
//	sort.Slice(accounts, func(i, j int) bool {
//		return accounts[i] < accounts[j]
//	})
//
//	atLeastOneFound := false
//	for _, accNr := range accounts {
//		toks := res[accNr]
//		var ownerKey string
//		if accNr == 0 {
//			ownerKey = "Tokens spendable by anyone:"
//		} else {
//			ownerKey = fmt.Sprintf("Tokens owned by account #%v", accNr)
//		}
//		consoleWriter.Println(ownerKey)
//		sort.Slice(toks, func(i, j int) bool {
//			// Fungible, then Non-fungible
//			return toks[i].Kind < toks[j].Kind
//		})
//		for _, tok := range toks {
//			atLeastOneFound = true
//			if tok.IsFungible() {
//				tokUnit, err := tw.GetTokenType(ctx, tok.TypeID)
//				if err != nil {
//					return err
//				}
//				// format amount
//				amount := amountToString(tok.Amount, tokUnit.DecimalPlaces)
//				consoleWriter.Println(fmt.Sprintf("ID='%X', Symbol='%s', amount='%v', token-type='%X' (%v)", tok.ID, tok.Symbol, amount, tok.TypeID, tok.Kind))
//			} else {
//				consoleWriter.Println(fmt.Sprintf("ID='%X', Symbol='%s', token-type='%X', URI='%s' (%v)", tok.ID, tok.Symbol, tok.TypeID, tok.URI, tok.Kind))
//			}
//		}
//	}
//	if !atLeastOneFound {
//		consoleWriter.Println("No tokens")
//	}
//	return nil
//}
//
//func tokenLegacyCmdListTypes(config *walletConfig, runner runTokenLegacyListTypesCmd) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   "list-types",
//		Short: "lists token types",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return runner(cmd, config, legacywallet.Any)
//		},
//	}
//	// add password flags as persistent
//	cmd.PersistentFlags().BoolP(passwordPromptCmdName, "p", false, passwordPromptUsage)
//	cmd.PersistentFlags().String(passwordArgCmdName, "", passwordArgUsage)
//	// add optional sub-commands to filter fungible and non-fungible types
//	cmd.AddCommand(&cobra.Command{
//		Use:   "fungible",
//		Short: "lists fungible types",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return runner(cmd, config, legacywallet.FungibleTokenType)
//		},
//	})
//	cmd.AddCommand(&cobra.Command{
//		Use:   "non-fungible",
//		Short: "lists non-fungible types",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			return runner(cmd, config, legacywallet.NonFungibleTokenType)
//		},
//	})
//	return cmd
//}
//
//func execTokenLegacyCmdListTypes(cmd *cobra.Command, config *walletConfig, kind legacywallet.TokenKind) error {
//	tw, err := initTokensLegacyWallet(cmd, config)
//	if err != nil {
//		return err
//	}
//	defer tw.Shutdown()
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	res, err := tw.ListTokenTypes(ctx, kind)
//	if err != nil {
//		return err
//	}
//	for _, tok := range res {
//		consoleWriter.Println(fmt.Sprintf("ID=%X, symbol=%s (%v)", tok.ID, tok.Symbol, tok.Kind))
//	}
//	return nil
//}
//
//func initTokensLegacyWallet(cmd *cobra.Command, config *walletConfig) (*legacywallet.Wallet, error) {
//	uri, err := cmd.Flags().GetString(alphabillUriCmdName)
//	if err != nil {
//		return nil, err
//	}
//	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
//	if err != nil {
//		return nil, err
//	}
//	syncStr, err := cmd.Flags().GetString(cmdFlagSync)
//	if err != nil {
//		return nil, err
//	}
//	sync, err := strconv.ParseBool(syncStr)
//	if err != nil {
//		return nil, err
//	}
//	tw, err := legacywallet.Load(config.WalletHomeDir, client.AlphabillClientConfig{Uri: uri}, am, sync)
//	if err != nil {
//		return nil, err
//	}
//	return tw, nil
//}
