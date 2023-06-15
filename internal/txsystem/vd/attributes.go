package vd

const (
	PayloadTypeRegisterData = "registerData"
)

type (
	RegisterDataAttributes struct {
		_ struct{} `cbor:",toarray"`
	}
)
