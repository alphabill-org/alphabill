package certificates

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test(t *testing.T) {
	bytes, err := base64.StdEncoding.DecodeString("AAAAAAAehIA=")
	require.NoError(t, err)
	data := binary.BigEndian.Uint64(bytes)
	fmt.Printf("%v", data)
}
