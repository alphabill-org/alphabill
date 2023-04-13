package twb

import (
	"fmt"
	"strconv"

	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func parsePubKey(pubkey string, required bool) (wallet.PubKey, error) {
	if pubkey == "" {
		if required {
			return nil, fmt.Errorf("parameter is required")
		}
		return nil, nil
	}
	return decodePubKeyHex(pubkey)
}

func decodePubKeyHex(pubKey string) (wallet.PubKey, error) {
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

func parseHex[T wallet.UnitID | TokenTypeID | TokenID | wallet.TxHash](value string, required bool) (T, error) {
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

func encodeHex[T wallet.UnitID | TokenTypeID | TokenID | wallet.TxHash](value T) string {
	if len(value) == 0 {
		return ""
	}
	return hexutil.Encode(value)
}

/*
parseMaxResponseItems parses input "s" as integer.
When empty string or int over "maxValue" is sent in "maxValue" is returned.
In case of invalid int or value smaller than 1 error is returned.
*/
func parseMaxResponseItems(s string, maxValue int) (int, error) {
	if s == "" {
		return maxValue, nil
	}

	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %q as integer: %w", s, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("value must be greater than zero, got %d", v)
	}

	if v > maxValue {
		return maxValue, nil
	}
	return v, nil
}
