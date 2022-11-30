package cmd

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"sort"
	"strconv"
	"strings"

	"github.com/alphabill-org/alphabill/internal/util"

	aberrors "github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	t "github.com/alphabill-org/alphabill/pkg/wallet/tokens"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
)

const (
	cmdFlagSymbol                     = "symbol"
	cmdFlagDecimals                   = "decimals"
	cmdFlagParentType                 = "parent-type"
	cmdFlagSybTypeClause              = "subtype-clause"
	cmdFlagSybTypeClauseInput         = "subtype-input"
	cmdFlagMintClause                 = "mint-clause"
	cmdFlagMintClauseInput            = "mint-input"
	cmdFlagInheritBearerClause        = "inherit-bearer-clause"
	cmdFlagInheritBearerClauseInput   = "inherit-bearer-input"
	cmdFlagTokenDataUpdateClause      = "data-update-clause"
	cmdFlagTokenDataUpdateClauseInput = "data-update-input"
	cmdFlagAmount                     = "amount"
	cmdFlagType                       = "type"
	cmdFlagTokenId                    = "token-identifier"
	cmdFlagTokenURI                   = "token-uri"
	cmdFlagTokenData                  = "data"
	cmdFlagTokenDataFile              = "data-file"
	cmdFlagSync                       = "sync"

	predicateEmpty    = "empty"
	predicateTrue     = "true"
	predicateFalse    = "false"
	predicatePtpkh    = "ptpkh"
	hexPrefix         = "0x"
	maxBinaryFile64Kb = 64 * 1024
	maxDecimalPlaces  = 8
)

var NoParent = []byte{0x00}

type runTokenListTypesCmd func(cmd *cobra.Command, config *walletConfig, kind t.TokenKind) error
type runTokenListCmd func(cmd *cobra.Command, config *walletConfig, kind t.TokenKind, accountNumber *int) error

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
	cmd.AddCommand(tokenCmdList(config, execTokenCmdList))
	cmd.AddCommand(tokenCmdListTypes(config, execTokenCmdListTypes))
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
	err := cmd.MarkFlagRequired(cmdFlagSymbol)
	if err != nil {
		return nil
	}
	cmd.Flags().BytesHex(cmdFlagParentType, NoParent, "unit identifier of a parent type in hexadecimal format, must start with 0x (optional)")
	cmd.Flags().StringSlice(cmdFlagSybTypeClauseInput, nil, "input to satisfy the parent type creation clause (mandatory with --parent-type)")
	cmd.Flags().String(cmdFlagSybTypeClause, predicateTrue, "predicate to control sub typing, values <true|false|ptpkh>, defaults to 'true' (optional)")
	cmd.Flags().String(cmdFlagMintClause, predicatePtpkh, "predicate to control minting of this type, values <true|false|ptpkh>, defaults to 'ptpkh' (optional)")
	cmd.Flags().String(cmdFlagInheritBearerClause, predicateTrue, "predicate that will be inherited by subtypes into their bearer clauses, values <true|false|ptpkh>, defaults to 'true' (optional)")
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

	typeId, err := getHexFlag(cmd, cmdFlagType)
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
	if decimals > maxDecimalPlaces {
		return fmt.Errorf("argument \"%v\" for \"--decimals\" flag is out of range, max value %v", decimals, maxDecimalPlaces)
	}
	parentType, creationInputs, err := readParentTypeInfo(cmd, tw.GetAccountManager())
	if err != nil {
		return err
	}
	subTypeCreationPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagSybTypeClause, tw.GetAccountManager())
	if err != nil {
		return err
	}
	mintTokenPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagMintClause, tw.GetAccountManager())
	if err != nil {
		return err
	}
	a := &tokens.CreateFungibleTokenTypeAttributes{
		Symbol:                             symbol,
		DecimalPlaces:                      decimals,
		ParentTypeId:                       parentType,
		SubTypeCreationPredicateSignatures: nil, // will be filled by the wallet
		SubTypeCreationPredicate:           subTypeCreationPredicate,
		TokenCreationPredicate:             mintTokenPredicate,
		InvariantPredicate:                 script.PredicateAlwaysTrue(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, err := tw.NewFungibleType(ctx, a, typeId, creationInputs)
	if err != nil {
		return err
	}
	consoleWriter.Println(fmt.Sprintf("Created new fungible token type with id=%X", id))
	return nil
}

func readParentTypeInfo(cmd *cobra.Command, am wallet.AccountManager) (tokens.Predicate, []*t.PredicateInput, error) {
	parentType, err := getHexFlag(cmd, cmdFlagParentType)
	if err != nil {
		return nil, nil, err
	}

	if len(parentType) == 0 || bytes.Equal(parentType, NoParent) {
		parentType = NoParent
		return NoParent, []*t.PredicateInput{{Argument: script.PredicateArgumentEmpty()}}, nil
	}

	creationInputs, err := readPredicateInput(cmd, cmdFlagSybTypeClauseInput, am)
	if err != nil {
		return nil, nil, err
	}

	return parentType, creationInputs, nil
}

func readPredicateInput(cmd *cobra.Command, flag string, am wallet.AccountManager) ([]*t.PredicateInput, error) {
	creationInputStrs, err := cmd.Flags().GetStringSlice(flag)
	if err != nil {
		return nil, err
	}
	if len(creationInputStrs) == 0 {
		return []*t.PredicateInput{{Argument: script.PredicateArgumentEmpty()}}, nil
	}
	creationInputs, err := parsePredicateArguments(creationInputStrs, am)
	if err != nil {
		return nil, err
	}
	return creationInputs, nil
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

	typeId, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}
	symbol, err := cmd.Flags().GetString(cmdFlagSymbol)
	if err != nil {
		return err
	}
	parentType, creationInputs, err := readParentTypeInfo(cmd, nil)
	if err != nil {
		return err
	}
	subTypeCreationPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagSybTypeClause, tw.GetAccountManager())
	if err != nil {
		return err
	}
	mintTokenPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagMintClause, tw.GetAccountManager())
	if err != nil {
		return err
	}
	a := &tokens.CreateNonFungibleTokenTypeAttributes{
		Symbol:                             symbol,
		ParentTypeId:                       parentType,
		SubTypeCreationPredicateSignatures: nil, // will be filled by the wallet
		SubTypeCreationPredicate:           subTypeCreationPredicate,
		TokenCreationPredicate:             mintTokenPredicate,
		InvariantPredicate:                 script.PredicateAlwaysTrue(),
		DataUpdatePredicate:                script.PredicateAlwaysTrue(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, err := tw.NewNonFungibleType(ctx, a, typeId, creationInputs)
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
	err := cmd.MarkFlagRequired(cmdFlagAmount)
	if err != nil {
		return nil
	}
	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
	err = cmd.MarkFlagRequired(cmdFlagType)
	if err != nil {
		return nil
	}
	cmd.Flags().StringSlice(cmdFlagMintClauseInput, []string{predicatePtpkh}, "input to satisfy the type's minting clause")
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
	typeId, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}
	ci, err := readPredicateInput(cmd, cmdFlagMintClauseInput, tw.GetAccountManager())
	if err != nil {
		return err
	}
	a := &tokens.MintFungibleTokenAttributes{
		Bearer:                           nil, // will be set in the wallet
		Type:                             typeId,
		Value:                            amount,
		TokenCreationPredicateSignatures: nil, // will be filled by the wallet
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, err := tw.NewFungibleToken(ctx, accountNumber, a, ci)
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
	err := cmd.MarkFlagRequired(cmdFlagType)
	if err != nil {
		return nil
	}
	cmd.Flags().String(cmdFlagTokenURI, "", "URI to associated resource, ie. jpg file on IPFS")
	cmd.Flags().BytesHex(cmdFlagTokenData, nil, "custom data (hex)")
	cmd.Flags().String(cmdFlagTokenDataFile, "", "data file (max 64Kb) path")
	// cmd.MarkFlagsMutuallyExclusive(cmdFlagTokenDataFile, cmdFlagTokenDataFile) TODO use once 1.5.0 is released
	cmd.Flags().BytesHex(cmdFlagTokenDataUpdateClause, nil, "data update predicate (hex)")
	cmd.Flags().StringSlice(cmdFlagMintClauseInput, []string{predicatePtpkh}, "input to satisfy the type's minting clause")
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

	typeId, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}
	tokenId, err := getHexFlag(cmd, cmdFlagTokenId)
	if err != nil {
		return err
	}
	uri, err := cmd.Flags().GetString(cmdFlagTokenURI)
	if err != nil {
		return err
	}
	data, err := getHexFlag(cmd, cmdFlagTokenData)
	if err != nil {
		return err
	}
	dataFilePath, err := cmd.Flags().GetString(cmdFlagTokenDataFile)
	if err != nil {
		return err
	}
	// TODO remove once 1.5.0 is released and use MarkFlagsMutuallyExclusive instead
	// cannot specify both inputs, either data or data-file
	if data != nil && len(dataFilePath) > 0 {
		return fmt.Errorf("flags \"--%v\" and \"--%v\" are mutually exclusive", cmdFlagTokenData, cmdFlagTokenDataFile)
	}
	if len(dataFilePath) > 0 {
		data, err = readDataFile(dataFilePath)
		if err != nil {
			return err
		}
	}
	ci, err := readPredicateInput(cmd, cmdFlagMintClauseInput, tw.GetAccountManager())
	if err != nil {
		return err
	}
	a := &tokens.MintNonFungibleTokenAttributes{
		Bearer:                           nil, // will be set in the wallet
		NftType:                          typeId,
		Uri:                              uri,
		Data:                             data,
		DataUpdatePredicate:              script.PredicateAlwaysTrue(),
		TokenCreationPredicateSignatures: nil, // will be set in the wallet
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, err := tw.NewNFT(ctx, accountNumber, a, tokenId, ci)
	if err != nil {
		return err
	}

	consoleWriter.Println(fmt.Sprintf("Created new non-fungible token with id=%X", id))
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
	err := cmd.MarkFlagRequired(cmdFlagTokenId)
	if err != nil {
		return nil
	}
	cmd.Flags().StringP(addressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	err = cmd.MarkFlagRequired(addressCmdName)
	if err != nil {
		return nil
	}
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

	tokenId, err := getHexFlag(cmd, cmdFlagTokenId)
	if err != nil {
		return err
	}

	pubKey, err := getPubKeyBytes(cmd, addressCmdName)
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
	err := cmd.MarkFlagRequired(cmdFlagAmount)
	if err != nil {
		return nil
	}
	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
	err = cmd.MarkFlagRequired(cmdFlagType)
	if err != nil {
		return nil
	}
	cmd.Flags().StringP(addressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	err = cmd.MarkFlagRequired(addressCmdName)
	if err != nil {
		return nil
	}
	return addCommonAccountFlags(cmd)
}

// getPubKeyBytes returns 'nil' for flag value 'true', must be interpreted as 'always true' predicate
func getPubKeyBytes(cmd *cobra.Command, flag string) ([]byte, error) {
	pubKeyHex, err := cmd.Flags().GetString(flag)
	if err != nil {
		return nil, err
	}
	var pubKey []byte
	if pubKeyHex == predicateTrue {
		pubKey = nil // this will assign 'always true' predicate
	} else {
		pk, ok := pubKeyHexToBytes(pubKeyHex)
		if !ok {
			return nil, fmt.Errorf("address in not in valid format: %s", pubKeyHex)
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

	typeId, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}

	targetValue, err := cmd.Flags().GetUint64(cmdFlagAmount)
	if err != nil {
		return err
	}

	pubKey, err := getPubKeyBytes(cmd, addressCmdName)
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
	err := cmd.MarkFlagRequired(cmdFlagTokenId)
	if err != nil {
		return nil
	}
	cmd.Flags().StringP(addressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	err = cmd.MarkFlagRequired(addressCmdName)
	if err != nil {
		return nil
	}
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

	tokenId, err := getHexFlag(cmd, cmdFlagTokenId)
	if err != nil {
		return err
	}

	pubKey, err := getPubKeyBytes(cmd, addressCmdName)
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

func tokenCmdList(config *walletConfig, runner runTokenListCmd) *cobra.Command {
	accountNumber := -1
	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists all available tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, t.Any, &accountNumber)
		},
	}
	// ass persistent password flags
	cmd.PersistentFlags().BoolP(passwordPromptCmdName, "p", false, passwordPromptUsage)
	cmd.PersistentFlags().String(passwordArgCmdName, "", passwordArgUsage)
	// add sub commands
	cmd.AddCommand(tokenCmdListFungible(config, runner, &accountNumber))
	cmd.AddCommand(tokenCmdListNonFungible(config, runner, &accountNumber))
	cmd.PersistentFlags().IntVarP(&accountNumber, keyCmdName, "k", -1, "which key to use for sending the transaction, 0 for tokens spendable by anyone, -1 for all tokens from all accounts")
	return cmd
}

func tokenCmdListFungible(config *walletConfig, runner runTokenListCmd, accountNumber *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "lists fungible tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, t.FungibleToken, accountNumber)
		},
	}
	return cmd
}

func tokenCmdListNonFungible(config *walletConfig, runner runTokenListCmd, accountNumber *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "lists non-fungible tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, t.NonFungibleToken, accountNumber)
		},
	}
	return cmd
}

// amountToString converts amount to string with specified decimals
// NB! it is assumed that the decimal places value is sane and verified before
// calling this method.
func amountToString(amount uint64, decimals uint32) string {
	amountStr := strconv.FormatUint(amount, 10)
	if decimals == 0 {
		return amountStr
	}
	// length of amount string is less than decimal places, insert comma in value
	if decimals < uint32(len(amountStr)) {
		return amountStr[:uint32(len(amountStr))-decimals] + "." + amountStr[uint32(len(amountStr))-decimals:]
	}
	// resulting amount is less than 0
	resultStr := "0."
	resultStr += strings.Repeat("0", int(decimals)-len(amountStr))
	return resultStr + amountStr
}

func execTokenCmdList(cmd *cobra.Command, config *walletConfig, kind t.TokenKind, accountNumber *int) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, err := tw.ListTokens(ctx, kind, *accountNumber)
	if err != nil {
		return err
	}

	accounts := make([]uint64, 0, len(res))
	for accNr := range res {
		accounts = append(accounts, accNr)
	}
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i] < accounts[j]
	})

	atLeastOneFound := false
	for _, accNr := range accounts {
		toks := res[accNr]
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
			atLeastOneFound = true
			if tok.IsFungible() {
				tokUnit, err := tw.GetTokenType(ctx, tok.TypeID)
				if err != nil {
					return err
				}
				// format amount
				amount := amountToString(tok.Amount, tokUnit.DecimalPlaces)
				consoleWriter.Println(fmt.Sprintf("ID='%X', Symbol='%s', amount='%v', token-type='%X' (%v)", tok.ID, tok.Symbol, amount, tok.TypeID, tok.Kind))
			} else {
				consoleWriter.Println(fmt.Sprintf("ID='%X', Symbol='%s', token-type='%X', URI='%s' (%v)", tok.ID, tok.Symbol, tok.TypeID, tok.URI, tok.Kind))
			}
		}
	}
	if !atLeastOneFound {
		consoleWriter.Println("No tokens")
	}
	return nil
}

func tokenCmdListTypes(config *walletConfig, runner runTokenListTypesCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-types",
		Short: "lists token types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, t.Any)
		},
	}
	// add password flags as persistent
	cmd.PersistentFlags().BoolP(passwordPromptCmdName, "p", false, passwordPromptUsage)
	cmd.PersistentFlags().String(passwordArgCmdName, "", passwordArgUsage)
	// add optional sub-commands to filter fungible and non-fungible types
	cmd.AddCommand(&cobra.Command{
		Use:   "fungible",
		Short: "lists fungible types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, t.FungibleTokenType)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "non-fungible",
		Short: "lists non-fungible types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, t.NonFungibleTokenType)
		},
	})
	return cmd
}

func execTokenCmdListTypes(cmd *cobra.Command, config *walletConfig, kind t.TokenKind) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, err := tw.ListTokenTypes(ctx, kind)
	if err != nil {
		return err
	}
	for _, tok := range res {
		consoleWriter.Println(fmt.Sprintf("ID=%X, symbol=%s (%v)", tok.ID, tok.Symbol, tok.Kind))
	}
	return nil
}

func initTokensWallet(cmd *cobra.Command, config *walletConfig) (*t.Wallet, error) {
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

// parsePredicateClause uses the following format:
// empty string returns "always true"
// true
// false
// ptpkh
// ptpkh:1
// ptpkh:0x<hex> where hex value is the hash of a public key
func parsePredicateClauseCmd(cmd *cobra.Command, flag string, am wallet.AccountManager) ([]byte, error) {
	clause, err := cmd.Flags().GetString(flag)
	if err != nil {
		return nil, err
	}
	return parsePredicateClause(clause, am)
}

func parsePredicateClause(clause string, am wallet.AccountManager) ([]byte, error) {
	if len(clause) == 0 || clause == predicateTrue {
		return script.PredicateAlwaysTrue(), nil
	}
	if clause == predicateFalse {
		return script.PredicateAlwaysFalse(), nil
	}

	keyNr := 1
	var err error
	if strings.HasPrefix(clause, predicatePtpkh) {
		if split := strings.Split(clause, ":"); len(split) == 2 {
			keyStr := split[1]
			if strings.HasPrefix(strings.ToLower(keyStr), hexPrefix) {
				if len(keyStr) < 3 {
					return nil, fmt.Errorf("invalid predicate clause: '%s'", clause)
				}
				keyHash, err := hexutil.Decode(keyStr)
				if err != nil {
					return nil, err
				}
				return script.PredicatePayToPublicKeyHashDefault(keyHash), nil
			} else {
				keyNr, err = strconv.Atoi(keyStr)
				if err != nil {
					return nil, aberrors.Wrapf(err, "invalid predicate clause: '%s'", clause)
				}
			}
		}
		if keyNr < 1 {
			return nil, fmt.Errorf("invalid key number: %v in '%s'", keyNr, clause)
		}
		accountKey, err := am.GetAccountKey(uint64(keyNr - 1))
		if err != nil {
			return nil, err
		}
		return script.PredicatePayToPublicKeyHashDefault(accountKey.PubKeyHash.Sha256), nil

	}
	if strings.HasPrefix(clause, hexPrefix) {
		return decodeHexOrEmpty(clause)
	}
	return nil, fmt.Errorf("invalid predicate clause: '%s'", clause)
}

func parsePredicateArguments(arguments []string, am wallet.AccountManager) ([]*t.PredicateInput, error) {
	creationInputs := make([]*t.PredicateInput, 0, len(arguments))
	for _, argument := range arguments {
		input, err := parsePredicateArgument(argument, am)
		if err != nil {
			return nil, err
		}
		creationInputs = append(creationInputs, input)
	}
	return creationInputs, nil
}

// parsePredicateArguments uses the following format:
// empty|true|false|empty produce an empty predicate argument
// ptpkh (key 1) or ptpkh:n (n > 0) produce an argument with the signed transaction by the given key
func parsePredicateArgument(argument string, am wallet.AccountManager) (*t.PredicateInput, error) {
	if len(argument) == 0 || argument == predicateEmpty || argument == predicateTrue || argument == predicateFalse {
		return &t.PredicateInput{Argument: script.PredicateArgumentEmpty()}, nil
	}
	keyNr := 1
	var err error
	if strings.HasPrefix(argument, predicatePtpkh) {
		if split := strings.Split(argument, ":"); len(split) == 2 {
			keyStr := split[1]
			if strings.HasPrefix(strings.ToLower(keyStr), hexPrefix) {
				return nil, fmt.Errorf("invalid creation input: '%s'", argument)
			} else {
				keyNr, err = strconv.Atoi(keyStr)
				if err != nil {
					return nil, aberrors.Wrapf(err, "invalid creation input: '%s'", argument)
				}
			}
		}
		if keyNr < 1 {
			return nil, fmt.Errorf("invalid key number: %v in '%s'", keyNr, argument)
		}
		_, err := am.GetAccountKey(uint64(keyNr - 1))
		if err != nil {
			return nil, err
		}
		return &t.PredicateInput{AccountNumber: uint64(keyNr)}, nil

	}
	if strings.HasPrefix(argument, hexPrefix) {
		decoded, err := decodeHexOrEmpty(argument)
		if err != nil {
			return nil, err
		}
		if len(decoded) == 0 {
			decoded = script.PredicateArgumentEmpty()
		}
		return &t.PredicateInput{Argument: decoded}, nil
	}
	return nil, fmt.Errorf("invalid creation input: '%s'", argument)
}

//getHexFlag returns nil in case array is empty (weird behaviour by cobra)
func getHexFlag(cmd *cobra.Command, flag string) ([]byte, error) {
	res, err := cmd.Flags().GetBytesHex(flag)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, err
	}
	return res, err
}

func decodeHexOrEmpty(input string) ([]byte, error) {
	if len(input) == 0 || input == predicateEmpty {
		return []byte{}, nil
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(input), hexPrefix))
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

func readDataFile(path string) ([]byte, error) {
	size, err := util.GetFileSize(path)
	if err != nil {
		return nil, fmt.Errorf("data-file read error: %w", err)
	}
	// verify file max 64KB
	if size > maxBinaryFile64Kb {
		return nil, fmt.Errorf("data-file read error: file size over 64Kb limit")
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("data-file read error: %w", err)
	}
	return data, nil
}
