package viper

import (
	"encoding/base64"
	"reflect"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"github.com/mitchellh/mapstructure"
)

type (
	Base64StringDeserializer struct {
		deserializers map[reflect.Type]reflectTypeDeserializer
	}
)

func NewBase64StringDeserializer() *Base64StringDeserializer {
	return &Base64StringDeserializer{
		deserializers: make(map[reflect.Type]reflectTypeDeserializer),
	}
}

func (h *Base64StringDeserializer) RegisterDeserializer(reflectType reflect.Type, deserializer reflectTypeDeserializer) {
	h.deserializers[reflectType] = deserializer
}

func (h *Base64StringDeserializer) DecodeHook(source reflect.Type, target reflect.Type, data interface{}) (
	interface{}, error,
) {
	deserializer, ok := h.deserializers[target]
	if !ok {
		return data, nil
	}

	str, isStr := data.(string)
	if source.Kind() != reflect.String || !isStr {
		return data, nil
	}

	bytes, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode string to bytes using base64 (string='%s')", str)
	}

	if len(bytes) == 0 {
		return nil, nil
	}

	value, err := deserializer(bytes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to construct value for type %s", target)
	}

	return value, nil
}

// MultipleDecodeHookFunc executes decode hooks sequentially until one of them returns a modified value
func MultipleDecodeHookFunc(fs ...mapstructure.DecodeHookFunc) mapstructure.DecodeHookFunc {
	return func(from reflect.Value, to reflect.Value) (interface{}, error) {
		var (
			err          error
			data         interface{}
			originalData = from.Interface()
		)
		for _, decodeHookFunc := range fs {
			data, err = mapstructure.DecodeHookExec(decodeHookFunc, from, to)
			if err != nil {
				return nil, err
			}
			if !reflect.DeepEqual(data, originalData) {
				return data, nil
			}
		}

		return originalData, nil
	}
}
