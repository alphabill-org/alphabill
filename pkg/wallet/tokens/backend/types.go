package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet"
)

type (
	TokenUnitType struct {
		// common
		ID                       TokenTypeID      `json:"id"`
		ParentTypeID             TokenTypeID      `json:"parentTypeId"`
		Symbol                   string           `json:"symbol"`
		Name                     string           `json:"name,omitempty"`
		Icon                     *tokens.Icon     `json:"icon,omitempty"`
		SubTypeCreationPredicate wallet.Predicate `json:"subTypeCreationPredicate,omitempty"`
		TokenCreationPredicate   wallet.Predicate `json:"tokenCreationPredicate,omitempty"`
		InvariantPredicate       wallet.Predicate `json:"invariantPredicate,omitempty"`
		// fungible only
		DecimalPlaces uint32 `json:"decimalPlaces,omitempty"`
		// nft only
		NftDataUpdatePredicate wallet.Predicate `json:"nftDataUpdatePredicate,omitempty"`
		// meta
		Kind   Kind          `json:"kind"`
		TxHash wallet.TxHash `json:"txHash"`
	}

	TokenUnit struct {
		// common
		ID       TokenID          `json:"id"`
		Symbol   string           `json:"symbol"`
		TypeID   TokenTypeID      `json:"typeId"`
		TypeName string           `json:"typeName"`
		Owner    wallet.Predicate `json:"owner"`
		// fungible only
		Amount   uint64 `json:"amount,omitempty,string"`
		Decimals uint32 `json:"decimals,omitempty"`
		Burned   bool   `json:"burned,omitempty"`
		// nft only
		NftName                string           `json:"nftName,omitempty"`
		NftURI                 string           `json:"nftUri,omitempty"`
		NftData                []byte           `json:"nftData,omitempty"`
		NftDataUpdatePredicate wallet.Predicate `json:"nftDataUpdatePredicate,omitempty"`
		// meta
		Kind   Kind          `json:"kind"`
		TxHash wallet.TxHash `json:"txHash"`
	}

	TokenID     wallet.UnitID
	TokenTypeID wallet.UnitID
	Kind        byte

	FeeCreditBill struct {
		Id              []byte `json:"id"`
		Value           uint64 `json:"value,string"`
		TxHash          []byte `json:"txHash"`
		LastAddFCTxHash []byte `json:"lastAddFcTxHash"`
	}
)

const (
	Any Kind = 1 << iota
	Fungible
	NonFungible
)

var (
	NoParent = TokenTypeID{0x00}
)

func (tu *TokenUnit) WriteSSE(w io.Writer) error {
	b, err := json.Marshal(tu)
	if err != nil {
		return fmt.Errorf("failed to convert token unit to json: %w", err)
	}
	_, err = fmt.Fprintf(w, "event: token\ndata: %s\n\n", b)
	return err
}

func (t TokenTypeID) Equal(to TokenTypeID) bool {
	return bytes.Equal(t, to)
}

func (kind Kind) String() string {
	switch kind {
	case Any:
		return "all"
	case Fungible:
		return "fungible"
	case NonFungible:
		return "nft"
	}
	return "unknown"
}

func strToTokenKind(s string) (Kind, error) {
	switch s {
	case "all", "":
		return Any, nil
	case "fungible":
		return Fungible, nil
	case "nft":
		return NonFungible, nil
	}
	return Any, fmt.Errorf("%q is not valid token kind", s)
}

func (f *FeeCreditBill) GetID() []byte {
	if f != nil {
		return f.Id
	}
	return nil
}

func (f *FeeCreditBill) GetValue() uint64 {
	if f != nil {
		return f.Value
	}
	return 0
}

func (f *FeeCreditBill) GetTxHash() []byte {
	if f != nil {
		return f.TxHash
	}
	return nil
}

func (f *FeeCreditBill) GetLastAddFCTxHash() []byte {
	if f != nil {
		return f.LastAddFCTxHash
	}
	return nil
}

func (f *FeeCreditBill) ToGenericBill() *wallet.Bill {
	if f == nil {
		return nil
	}
	return &wallet.Bill{
		Id:              f.Id,
		Value:           f.Value,
		TxHash:          f.TxHash,
		LastAddFCTxHash: f.LastAddFCTxHash,
	}
}
