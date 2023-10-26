package cmd

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/validator/internal/util"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/account"
	wallet "github.com/alphabill-org/alphabill/validator/pkg/wallet/tokens"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/tokens/backend"
	"github.com/spf13/cobra"
)

const (
	cmdFlagSymbol                     = "symbol"
	cmdFlagName                       = "name"
	cmdFlagIconFile                   = "icon-file"
	cmdFlagDecimals                   = "decimals"
	cmdFlagParentType                 = "parent-type"
	cmdFlagSybTypeClause              = "subtype-clause"
	cmdFlagSybTypeClauseInput         = "subtype-input"
	cmdFlagMintClause                 = "mint-clause"
	cmdFlagBearerClause               = "bearer-clause"
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

	cmdFlagWithAll       = "with-all"
	cmdFlagWithTypeName  = "with-type-name"
	cmdFlagWithTokenURI  = "with-token-uri"
	cmdFlagWithTokenData = "with-token-data"

	predicateTrue  = "true"
	predicatePtpkh = "ptpkh"

	iconFileExtSvgz     = ".svgz"
	iconFileExtSvgzType = "image/svg+xml; encoding=gzip"

	maxBinaryFile64KiB = 64 * 1024
	maxDecimalPlaces   = 8
)

type runTokenListTypesCmd func(cmd *cobra.Command, config *walletConfig, accountNumber *uint64, kind backend.Kind) error
type runTokenListCmd func(cmd *cobra.Command, config *walletConfig, accountNumber *uint64, kind backend.Kind) error

func tokenCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "create and manage fungible and non-fungible tokens",
	}
	cmd.AddCommand(tokenCmdNewType(config))
	cmd.AddCommand(tokenCmdNewToken(config))
	cmd.AddCommand(tokenCmdUpdateNFTData(config))
	cmd.AddCommand(tokenCmdSend(config))
	cmd.AddCommand(tokenCmdDC(config))
	cmd.AddCommand(tokenCmdList(config, execTokenCmdList))
	cmd.AddCommand(tokenCmdListTypes(config, execTokenCmdListTypes))
	cmd.PersistentFlags().StringP(alphabillApiURLCmdName, "r", defaultTokensBackendApiURL, "alphabill tokens backend API uri to connect to")
	cmd.PersistentFlags().StringP(waitForConfCmdName, "w", "true", "waits for transaction confirmation on the blockchain, otherwise just broadcasts the transaction")
	return cmd
}

func tokenCmdNewType(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new-type",
		Short: "create new token type",
	}
	cmd.AddCommand(addCommonAccountFlags(addCommonTypeFlags(tokenCmdNewTypeFungible(config))))
	cmd.AddCommand(addCommonAccountFlags(addCommonTypeFlags(tokenCmdNewTypeNonFungible(config))))
	return cmd
}

func addCommonAccountFlags(cmd *cobra.Command) *cobra.Command {
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "which key to use for sending the transaction")
	return cmd
}

func addDataFlags(cmd *cobra.Command) {
	altMsg := ". Alternatively flag %q can be used to add data."
	cmd.Flags().BytesHex(cmdFlagTokenData, nil, "custom data (hex)"+fmt.Sprintf(altMsg, cmdFlagTokenDataFile))
	cmd.Flags().String(cmdFlagTokenDataFile, "", "data file (max 64Kb) path"+fmt.Sprintf(altMsg, cmdFlagTokenData))
	cmd.MarkFlagsMutuallyExclusive(cmdFlagTokenData, cmdFlagTokenDataFile)
}

func addCommonTypeFlags(cmd *cobra.Command) *cobra.Command {
	cmd.Flags().String(cmdFlagSymbol, "", "symbol (short name) of the token type (mandatory)")
	cmd.Flags().String(cmdFlagName, "", "full name of the token type (optional)")
	cmd.Flags().String(cmdFlagIconFile, "", "icon file name for the token type (optional)")
	if err := cmd.MarkFlagRequired(cmdFlagSymbol); err != nil {
		panic(err)
	}

	cmd.Flags().BytesHex(cmdFlagParentType, nil, "unit identifier of a parent type in hexadecimal format")
	cmd.Flags().StringSlice(cmdFlagSybTypeClauseInput, nil, "input to satisfy the parent type creation clause (mandatory with --parent-type)")
	cmd.MarkFlagsRequiredTogether(cmdFlagParentType, cmdFlagSybTypeClauseInput)
	cmd.Flags().String(cmdFlagSybTypeClause, predicateTrue, "predicate to control sub typing, values <true|false|ptpkh>")
	cmd.Flags().String(cmdFlagMintClause, predicatePtpkh, "predicate to control minting of this type, values <true|false|ptpkh>")
	cmd.Flags().String(cmdFlagInheritBearerClause, predicateTrue, "predicate that will be inherited by subtypes into their bearer clauses, values <true|false|ptpkh>")
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
	cmd.Flags().Uint32(cmdFlagDecimals, 8, "token decimal")
	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
	_ = cmd.Flags().MarkHidden(cmdFlagType)
	return cmd
}

func execTokenCmdNewTypeFungible(cmd *cobra.Command, config *walletConfig) error {
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
	symbol, err := cmd.Flags().GetString(cmdFlagSymbol)
	if err != nil {
		return err
	}
	name, err := cmd.Flags().GetString(cmdFlagName)
	if err != nil {
		return err
	}
	iconFilePath, err := cmd.Flags().GetString(cmdFlagIconFile)
	if err != nil {
		return err
	}
	icon, err := readIconFile(iconFilePath)
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
	am := tw.GetAccountManager()
	parentType, creationInputs, err := readParentTypeInfo(cmd, accountNumber, am)
	if err != nil {
		return err
	}
	subTypeCreationPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagSybTypeClause, accountNumber, am)
	if err != nil {
		return err
	}
	mintTokenPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagMintClause, accountNumber, am)
	if err != nil {
		return err
	}
	invariantPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagInheritBearerClause, accountNumber, am)
	if err != nil {
		return err
	}
	a := wallet.CreateFungibleTokenTypeAttributes{
		Symbol:                   symbol,
		Name:                     name,
		Icon:                     icon,
		DecimalPlaces:            decimals,
		ParentTypeId:             parentType,
		SubTypeCreationPredicate: subTypeCreationPredicate,
		TokenCreationPredicate:   mintTokenPredicate,
		InvariantPredicate:       invariantPredicate,
	}
	result, err := tw.NewFungibleType(cmd.Context(), accountNumber, a, typeId, creationInputs)
	if err != nil {
		return err
	}
	consoleWriter.Println(fmt.Sprintf("Sent request for new fungible token type with id=%s", result.TokenTypeID))
	if result.FeeSum > 0 {
		consoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", amountToString(result.FeeSum, 8)))
	}
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
	cmd.Flags().String(cmdFlagTokenDataUpdateClause, predicateTrue, "data update predicate, values <true|false|ptpkh>")
	return cmd
}

func execTokenCmdNewTypeNonFungible(cmd *cobra.Command, config *walletConfig) error {
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
	symbol, err := cmd.Flags().GetString(cmdFlagSymbol)
	if err != nil {
		return err
	}
	name, err := cmd.Flags().GetString(cmdFlagName)
	if err != nil {
		return err
	}
	iconFilePath, err := cmd.Flags().GetString(cmdFlagIconFile)
	if err != nil {
		return err
	}
	icon, err := readIconFile(iconFilePath)
	if err != nil {
		return err
	}
	am := tw.GetAccountManager()
	parentType, creationInputs, err := readParentTypeInfo(cmd, accountNumber, am)
	if err != nil {
		return err
	}
	subTypeCreationPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagSybTypeClause, accountNumber, am)
	if err != nil {
		return err
	}
	mintTokenPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagMintClause, accountNumber, am)
	if err != nil {
		return err
	}
	dataUpdatePredicate, err := parsePredicateClauseCmd(cmd, cmdFlagTokenDataUpdateClause, accountNumber, am)
	if err != nil {
		return err
	}
	invariantPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagInheritBearerClause, accountNumber, am)
	if err != nil {
		return err
	}
	a := wallet.CreateNonFungibleTokenTypeAttributes{
		Symbol:                   symbol,
		Name:                     name,
		Icon:                     icon,
		ParentTypeId:             parentType,
		SubTypeCreationPredicate: subTypeCreationPredicate,
		TokenCreationPredicate:   mintTokenPredicate,
		InvariantPredicate:       invariantPredicate,
		DataUpdatePredicate:      dataUpdatePredicate,
	}
	result, err := tw.NewNonFungibleType(cmd.Context(), accountNumber, a, typeId, creationInputs)
	if err != nil {
		return err
	}
	consoleWriter.Println(fmt.Sprintf("Sent request for new NFT type with id=%s", result.TokenTypeID))
	if result.FeeSum > 0 {
		consoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", amountToString(result.FeeSum, 8)))
	}
	return nil
}

func tokenCmdNewToken(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new",
		Short: "mint new token",
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
	cmd.Flags().String(cmdFlagBearerClause, predicatePtpkh, "predicate that defines the ownership of this fungible token, values <true|false|ptpkh>")
	cmd.Flags().String(cmdFlagAmount, "", "amount, must be bigger than 0 and is interpreted according to token type precision (decimals)")
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
	am := tw.GetAccountManager()
	defer tw.Shutdown()

	amountStr, err := cmd.Flags().GetString(cmdFlagAmount)
	if err != nil {
		return err
	}
	typeId, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}
	ci, err := readPredicateInput(cmd, cmdFlagMintClauseInput, accountNumber, am)
	if err != nil {
		return err
	}
	tt, err := tw.GetTokenType(cmd.Context(), typeId)
	if err != nil {
		return err
	}
	// convert amount from string to uint64
	amount, err := stringToAmount(amountStr, tt.DecimalPlaces)
	if err != nil {
		return err
	}
	if amount == 0 {
		return fmt.Errorf("invalid parameter \"%s\" for \"--amount\": 0 is not valid amount", amountStr)
	}
	bearerPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagBearerClause, accountNumber, am)
	if err != nil {
		return err
	}
	result, err := tw.NewFungibleToken(cmd.Context(), accountNumber, typeId, amount, bearerPredicate, ci)
	if err != nil {
		return err
	}

	consoleWriter.Println(fmt.Sprintf("Sent request for new fungible token with id=%s", result.TokenID))
	if result.FeeSum > 0 {
		consoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", amountToString(result.FeeSum, 8)))
	}
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
	addDataFlags(cmd)
	cmd.Flags().String(cmdFlagBearerClause, predicatePtpkh, "predicate that defines the ownership of this non-fungible token, values <true|false|ptpkh>")
	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
	err := cmd.MarkFlagRequired(cmdFlagType)
	if err != nil {
		return nil
	}
	cmd.Flags().String(cmdFlagName, "", "name of the token (optional)")
	cmd.Flags().String(cmdFlagTokenURI, "", "URI to associated resource, ie. jpg file on IPFS")
	cmd.Flags().String(cmdFlagTokenDataUpdateClause, predicateTrue, "data update predicate, values <true|false|ptpkh>")
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
	name, err := cmd.Flags().GetString(cmdFlagName)
	if err != nil {
		return err
	}
	uri, err := cmd.Flags().GetString(cmdFlagTokenURI)
	if err != nil {
		return err
	}
	data, err := readNFTData(cmd, false)
	if err != nil {
		return err
	}
	am := tw.GetAccountManager()
	ci, err := readPredicateInput(cmd, cmdFlagMintClauseInput, accountNumber, am)
	if err != nil {
		return err
	}
	bearerPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagBearerClause, accountNumber, am)
	if err != nil {
		return err
	}
	dataUpdatePredicate, err := parsePredicateClauseCmd(cmd, cmdFlagTokenDataUpdateClause, accountNumber, am)
	if err != nil {
		return err
	}
	a := wallet.MintNonFungibleTokenAttributes{
		Bearer:              bearerPredicate,
		Name:                name,
		NftType:             typeId,
		Uri:                 uri,
		Data:                data,
		DataUpdatePredicate: dataUpdatePredicate,
	}
	result, err := tw.NewNFT(cmd.Context(), accountNumber, a, tokenId, ci)
	if err != nil {
		return err
	}

	consoleWriter.Println(fmt.Sprintf("Sent request for new non-fungible token with id=%s", result.TokenID))
	if result.FeeSum > 0 {
		consoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", amountToString(result.FeeSum, 8)))
	}
	return nil
}

func tokenCmdSend(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send",
		Short: "send a token",
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
	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the type's invariant clause")
	cmd.Flags().String(cmdFlagAmount, "", "amount, must be bigger than 0 and is interpreted according to token type precision (decimals)")
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

	amountStr, err := cmd.Flags().GetString(cmdFlagAmount)
	if err != nil {
		return err
	}

	pubKey, err := getPubKeyBytes(cmd, addressCmdName)
	if err != nil {
		return err
	}

	ib, err := readPredicateInput(cmd, cmdFlagInheritBearerClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	// get token type and convert amount string
	tt, err := tw.GetTokenType(cmd.Context(), typeId)
	if err != nil {
		return err
	}
	// convert amount from string to uint64
	targetValue, err := stringToAmount(amountStr, tt.DecimalPlaces)
	if err != nil {
		return err
	}
	if targetValue == 0 {
		return fmt.Errorf("invalid parameter \"%s\" for \"--amount\": 0 is not valid amount", amountStr)
	}
	result, err := tw.SendFungible(cmd.Context(), accountNumber, typeId, targetValue, pubKey, ib)
	if err != nil {
		return err
	}
	if result.FeeSum > 0 {
		consoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", amountToString(result.FeeSum, 8)))
	}
	return err
}

func tokenCmdSendNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "transfer non-fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdSendNonFungible(cmd, config)
		},
	}
	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the type's invariant clause")
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

	ib, err := readPredicateInput(cmd, cmdFlagInheritBearerClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	result, err := tw.TransferNFT(cmd.Context(), accountNumber, tokenId, pubKey, ib)
	if err != nil {
		return err
	}
	if result.FeeSum > 0 {
		consoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", amountToString(result.FeeSum, 8)))
	}
	return err
}

func tokenCmdDC(config *walletConfig) *cobra.Command {
	var accountNumber uint64

	cmd := &cobra.Command{
		Use:   "collect-dust",
		Short: "join fungible tokens into one unit",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdDC(cmd, config, &accountNumber)
		},
	}

	cmd.Flags().Uint64VarP(&accountNumber, keyCmdName, "k", 0, "which key to use for dust collection, 0 for all tokens from all accounts")
	cmd.Flags().StringSlice(cmdFlagType, nil, "type unit identifier (hex)")
	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the type's invariant clause")

	return cmd
}

func execTokenCmdDC(cmd *cobra.Command, config *walletConfig, accountNumber *uint64) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	typeIDStrs, err := cmd.Flags().GetStringSlice(cmdFlagType)
	if err != nil {
		return err
	}
	var types []backend.TokenTypeID
	for _, tokenType := range typeIDStrs {
		typeBytes, err := wallet.DecodeHexOrEmpty(tokenType)
		if err != nil {
			return err
		}
		if len(typeBytes) > 0 {
			types = append(types, typeBytes)
		}
	}
	ib, err := readPredicateInput(cmd, cmdFlagInheritBearerClauseInput, *accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	results, err := tw.CollectDust(cmd.Context(), *accountNumber, types, ib)
	if err != nil {
		return err
	}
	for _, result := range results {
		if result.FeeSum > 0 {
			consoleWriter.Println(fmt.Sprintf("Paid %s fees for dust collection on Account number %d.", amountToString(result.FeeSum, 8), result.AccountNumber))
		}
	}
	return err
}

func tokenCmdUpdateNFTData(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "update the data field on a non-fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdUpdateNFTData(cmd, config)
		},
	}
	cmd.Flags().BytesHex(cmdFlagTokenId, nil, "token identifier (hex)")
	if err := cmd.MarkFlagRequired(cmdFlagTokenId); err != nil {
		panic(err)
	}

	addDataFlags(cmd)
	cmd.Flags().StringSlice(cmdFlagTokenDataUpdateClauseInput, []string{predicateTrue, predicateTrue}, "input to satisfy the data-update clauses")
	return addCommonAccountFlags(cmd)
}

func execTokenCmdUpdateNFTData(cmd *cobra.Command, config *walletConfig) error {
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

	data, err := readNFTData(cmd, true)
	if err != nil {
		return err
	}

	du, err := readPredicateInput(cmd, cmdFlagTokenDataUpdateClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	result, err := tw.UpdateNFTData(cmd.Context(), accountNumber, tokenId, data, du)
	if err != nil {
		return err
	}
	if result.FeeSum > 0 {
		consoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", amountToString(result.FeeSum, 8)))
	}
	return err
}

func tokenCmdList(config *walletConfig, runner runTokenListCmd) *cobra.Command {
	var accountNumber uint64
	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists all available tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, &accountNumber, backend.Any)
		},
	}
	// add persistent password flags
	cmd.PersistentFlags().BoolP(passwordPromptCmdName, "p", false, passwordPromptUsage)
	cmd.PersistentFlags().String(passwordArgCmdName, "", passwordArgUsage)

	cmd.Flags().Bool(cmdFlagWithAll, false, "Show all available fields for each token")
	cmd.Flags().Bool(cmdFlagWithTypeName, false, "Show type name field")
	cmd.Flags().Bool(cmdFlagWithTokenURI, false, "Show non-fungible token URI field")
	cmd.Flags().Bool(cmdFlagWithTokenData, false, "Show non-fungible token data field")

	// add sub commands
	cmd.AddCommand(tokenCmdListFungible(config, runner, &accountNumber))
	cmd.AddCommand(tokenCmdListNonFungible(config, runner, &accountNumber))
	cmd.PersistentFlags().Uint64VarP(&accountNumber, keyCmdName, "k", 0, "which key to use for sending the transaction, 0 for all tokens from all accounts")
	return cmd
}

func tokenCmdListFungible(config *walletConfig, runner runTokenListCmd, accountNumber *uint64) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "lists fungible tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, accountNumber, backend.Fungible)
		},
	}

	cmd.Flags().Bool(cmdFlagWithAll, false, "Show all available fields for each token")
	cmd.Flags().Bool(cmdFlagWithTypeName, false, "Show type name field")

	return cmd
}

func tokenCmdListNonFungible(config *walletConfig, runner runTokenListCmd, accountNumber *uint64) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "lists non-fungible tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, accountNumber, backend.NonFungible)
		},
	}

	cmd.Flags().Bool(cmdFlagWithAll, false, "Show all available fields for each token")
	cmd.Flags().Bool(cmdFlagWithTypeName, false, "Show type name field")
	cmd.Flags().Bool(cmdFlagWithTokenURI, false, "Show token URI field")
	cmd.Flags().Bool(cmdFlagWithTokenData, false, "Show token data field")

	return cmd
}

func execTokenCmdList(cmd *cobra.Command, config *walletConfig, accountNumber *uint64, kind backend.Kind) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	res, err := tw.ListTokens(cmd.Context(), kind, *accountNumber)
	if err != nil {
		return err
	}

	withAll, err := cmd.Flags().GetBool(cmdFlagWithAll)
	if err != nil {
		return err
	}

	withTypeName, withTokenURI, withTokenData := false, false, false
	if !withAll {
		withTypeName, err = cmd.Flags().GetBool(cmdFlagWithTypeName)
		if err != nil {
			return err
		}
		if kind == backend.Any || kind == backend.NonFungible {
			withTokenURI, err = cmd.Flags().GetBool(cmdFlagWithTokenURI)
			if err != nil {
				return err
			}
			withTokenData, err = cmd.Flags().GetBool(cmdFlagWithTokenData)
			if err != nil {
				return err
			}
		}
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

			var typeName, nftURI, nftData string
			if withAll || withTypeName {
				typeName = fmt.Sprintf(", token-type-name='%s'", tok.TypeName)
			}
			if withAll || withTokenURI {
				nftURI = fmt.Sprintf(", URI='%s'", tok.NftURI)
			}
			if withAll || withTokenData {
				nftData = fmt.Sprintf(", data='%X'", tok.NftData)
			}
			kind := fmt.Sprintf(" (%v)", tok.Kind)

			if tok.Kind == backend.Fungible {
				amount := amountToString(tok.Amount, tok.Decimals)
				consoleWriter.Println(fmt.Sprintf("ID='%s', symbol='%s', amount='%v', token-type='%s'",
					tok.ID, tok.Symbol, amount, tok.TypeID) + typeName + kind)
			} else {
				consoleWriter.Println(fmt.Sprintf("ID='%s', symbol='%s', name='%s', token-type='%s'",
					tok.ID, tok.Symbol, tok.NftName, tok.TypeID) + typeName + nftURI + nftData + kind)
			}
		}
	}
	if !atLeastOneFound {
		consoleWriter.Println("No tokens")
	}
	return nil
}

func tokenCmdListTypes(config *walletConfig, runner runTokenListTypesCmd) *cobra.Command {
	var accountNumber uint64
	cmd := &cobra.Command{
		Use:   "list-types",
		Short: "lists token types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, &accountNumber, backend.Any)
		},
	}
	// add password flags as persistent
	cmd.PersistentFlags().BoolP(passwordPromptCmdName, "p", false, passwordPromptUsage)
	cmd.PersistentFlags().String(passwordArgCmdName, "", passwordArgUsage)
	cmd.PersistentFlags().Uint64VarP(&accountNumber, keyCmdName, "k", 0, "show types created from a specific key, 0 for all keys")
	// add optional sub-commands to filter fungible and non-fungible types
	cmd.AddCommand(&cobra.Command{
		Use:   "fungible",
		Short: "lists fungible types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, &accountNumber, backend.Fungible)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "non-fungible",
		Short: "lists non-fungible types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, &accountNumber, backend.NonFungible)
		},
	})
	return cmd
}

func execTokenCmdListTypes(cmd *cobra.Command, config *walletConfig, accountNumber *uint64, kind backend.Kind) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	res, err := tw.ListTokenTypes(cmd.Context(), *accountNumber, kind)
	if err != nil {
		return err
	}
	for _, tok := range res {
		name := ""
		if tok.Name != "" {
			name = fmt.Sprintf(", name=%s", tok.Name)
		}
		kind := fmt.Sprintf(" (%v)", tok.Kind)
		consoleWriter.Println(fmt.Sprintf("ID=%s, symbol=%s", tok.ID, tok.Symbol) + name + kind)
	}
	return nil
}

func initTokensWallet(cmd *cobra.Command, config *walletConfig) (*wallet.Wallet, error) {
	uri, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return nil, err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return nil, err
	}
	confirmTxStr, err := cmd.Flags().GetString(waitForConfCmdName)
	if err != nil {
		return nil, err
	}
	confirmTx, err := strconv.ParseBool(confirmTxStr)
	if err != nil {
		return nil, err
	}
	tw, err := wallet.New(tokens.DefaultSystemIdentifier, uri, am, confirmTx, nil, config.Base.Logger)
	if err != nil {
		return nil, err
	}
	return tw, nil
}

func readParentTypeInfo(cmd *cobra.Command, keyNr uint64, am account.Manager) (backend.TokenTypeID, []*wallet.PredicateInput, error) {
	parentType, err := getHexFlag(cmd, cmdFlagParentType)
	if err != nil {
		return nil, nil, err
	}

	if len(parentType) == 0 {
		return nil, []*wallet.PredicateInput{{Argument: nil}}, nil
	}

	creationInputs, err := readPredicateInput(cmd, cmdFlagSybTypeClauseInput, keyNr, am)
	if err != nil {
		return nil, nil, err
	}

	return parentType, creationInputs, nil
}

func readPredicateInput(cmd *cobra.Command, flag string, keyNr uint64, am account.Manager) ([]*wallet.PredicateInput, error) {
	creationInputStrs, err := cmd.Flags().GetStringSlice(flag)
	if err != nil {
		return nil, err
	}
	if len(creationInputStrs) == 0 {
		return []*wallet.PredicateInput{{Argument: nil}}, nil
	}
	creationInputs, err := wallet.ParsePredicates(creationInputStrs, keyNr, am)
	if err != nil {
		return nil, err
	}
	return creationInputs, nil
}

// parsePredicateClause uses the following format:
// empty string returns "always true"
// true
// false
// ptpkh
// ptpkh:1
// ptpkh:0x<hex> where hex value is the hash of a public key
func parsePredicateClauseCmd(cmd *cobra.Command, flag string, keyNr uint64, am account.Manager) ([]byte, error) {
	clause, err := cmd.Flags().GetString(flag)
	if err != nil {
		return nil, err
	}
	return wallet.ParsePredicateClause(clause, keyNr, am)
}

func readNFTData(cmd *cobra.Command, required bool) ([]byte, error) {
	if required && !cmd.Flags().Changed(cmdFlagTokenData) && !cmd.Flags().Changed(cmdFlagTokenDataFile) {
		return nil, fmt.Errorf("either of ['--%s', '--%s'] flags must be specified", cmdFlagTokenData, cmdFlagTokenDataFile)
	}
	data, err := getHexFlag(cmd, cmdFlagTokenData)
	if err != nil {
		return nil, err
	}
	dataFilePath, err := cmd.Flags().GetString(cmdFlagTokenDataFile)
	if err != nil {
		return nil, err
	}
	if len(dataFilePath) > 0 {
		data, err = readFile(dataFilePath, cmdFlagTokenDataFile, maxBinaryFile64KiB)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

// getHexFlag returns nil in case array is empty (weird behaviour by cobra)
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

func readIconFile(iconFilePath string) (*wallet.Icon, error) {
	if len(iconFilePath) == 0 {
		return nil, nil
	}
	icon := &wallet.Icon{}

	ext := filepath.Ext(iconFilePath)
	if len(ext) == 0 {
		return nil, fmt.Errorf("%s read error: missing file extension", cmdFlagIconFile)
	}

	mime.AddExtensionType(iconFileExtSvgz, iconFileExtSvgzType)
	icon.Type = mime.TypeByExtension(ext)
	if len(icon.Type) == 0 {
		return nil, fmt.Errorf("%s read error: could not determine MIME type from file extension", cmdFlagIconFile)
	}

	data, err := readFile(iconFilePath, cmdFlagIconFile, maxBinaryFile64KiB)
	if err != nil {
		return nil, err
	}
	icon.Data = data
	return icon, nil
}

func readFile(path string, flag string, sizeLimit int64) ([]byte, error) {
	size, err := util.GetFileSize(path)
	if err != nil {
		return nil, fmt.Errorf("%s read error: %w", flag, err)
	}
	if size > sizeLimit {
		return nil, fmt.Errorf("%s read error: file size over %vKiB limit", flag, sizeLimit/1024)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s read error: %w", flag, err)
	}
	return data, nil
}
