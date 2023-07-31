package types

import (
	"bytes"
	"fmt"
)

type SystemID []byte
type UnitID []byte

func (uid UnitID) Compare(key UnitID) int {
	return bytes.Compare(uid, key)
}

func (uid UnitID) String() string {
	return fmt.Sprintf("%X", []byte(uid))
}

func (uid UnitID) Eq(id UnitID) bool {
	return bytes.Equal(uid, id)
}

func (uid UnitID) IsZero(l int) bool {
	return bytes.Equal(uid, make([]byte, l))
}
