package keyvaluedb

import (
	"errors"
	"reflect"
)

var (
	errInvalidKey = errors.New("invalid key")
	errValueIsNil = errors.New("value is nil")
)

func CheckKey(key []byte) error {
	if len(key) == 0 {
		return errInvalidKey
	}
	return nil
}

func CheckValue(val any) error {
	if reflect.ValueOf(val).Kind() == reflect.Ptr && reflect.ValueOf(val).IsNil() {
		return errValueIsNil
	}
	return nil
}

func CheckKeyAndValue(key []byte, val any) error {
	if err := CheckKey(key); err != nil {
		return err
	}
	if err := CheckValue(val); err != nil {
		return err
	}
	return nil
}
