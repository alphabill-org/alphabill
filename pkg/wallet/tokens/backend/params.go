package twb

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

func parsePubKey(pubkey string, required bool) (PubKey, error) {
	if pubkey == "" {
		if required {
			return nil, fmt.Errorf("parameter is required")
		}
		return nil, nil
	}
	return decodePubKeyHex(pubkey)
}

func decodePubKeyHex(pubKey string) (PubKey, error) {
	if n := len(pubKey); n != 68 {
		s := " starting "
		switch {
		case n == 0:
			s = ""
		case n <= 6:
			s += pubKey
		default:
			s += pubKey[:6]
		}
		return nil, fmt.Errorf("must be 68 characters long (including 0x prefix), got %d characters%s", len(pubKey), s)
	}
	bytes, err := hexutil.Decode(pubKey)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func parseTokenID(value string, required bool) (TokenID, error) {
	if value == "" {
		if required {
			return nil, fmt.Errorf("parameter is required")
		}
		return nil, nil
	}

	bytes, err := hexutil.Decode(value)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func encodeTokenID(value TokenID) string {
	if len(value) == 0 {
		return ""
	}
	return hexutil.Encode(value)
}

func parseTokenTypeID(value string, required bool) (TokenTypeID, error) {
	if value == "" {
		if required {
			return nil, fmt.Errorf("parameter is required")
		}
		return nil, nil
	}

	bytes, err := hexutil.Decode(value)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func encodeTokenTypeID(value TokenTypeID) string {
	if len(value) == 0 {
		return ""
	}
	return hexutil.Encode(value)
}
