package canonicalizer

import (
	"alphabill-wallet-sdk/internal/errors"
	"reflect"
	"strconv"
	"strings"
)

type (
	fieldOptions struct {
		name        string
		index       int8
		elementSize int8
	}

	hshTagOption string
)

const (
	hshTag            = "hsh"
	hshTagOptionIndex = hshTagOption("idx")  // Hash concatenation index for the given field.
	hshTagOptionSize  = hshTagOption("size") // Element size in bytes. Valid only in case of integers (default size is 1).
)

type byIndex []fieldOptions

func (s byIndex) Len() int           { return len(s) }
func (s byIndex) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s byIndex) Less(i, j int) bool { return s[i].index < s[j].index }

func fieldOptionsOf(o interface{}) ([]fieldOptions, error) {
	var (
		ofType     = getObjType(o)
		structOpts = make([]fieldOptions, 0, ofType.NumField())
		indices    = make(map[int8]bool)
	)
	for i := 0; i < ofType.NumField(); i++ {
		var (
			fieldName = ofType.Field(i).Name
			fieldTag  = ofType.Field(i).Tag
		)
		hsh, exist := fieldTag.Lookup(hshTag)
		if !exist {
			return nil, errors.Wrapf(errors.ErrInvalidHashField, "%s.%s tag='%s' is not defined", ofType.Name(), fieldName, hshTag)
		}

		fieldOpts := fieldOptions{
			name:        fieldName,
			index:       -1,
			elementSize: -1,
		}

		// Parse field tag parameters.
		fieldTagParams := strings.Split(hsh, ",")
		for _, param := range fieldTagParams {
			paramSplit := strings.Split(param, "=")
			switch hshTagOption(paramSplit[0]) {
			case hshTagOptionIndex:
				if len(paramSplit) != 2 {
					return nil, errors.Wrapf(errors.ErrInvalidHashField, "%s.%s tag 'seq' parameter invalid: %v", ofType.Name(), fieldName, paramSplit)
				}
				val, err := strconv.ParseInt(paramSplit[1], 10, 8)
				if err != nil {
					return nil, err
				}
				idx := int8(val)
				if _, used := indices[idx]; used {
					return nil, errors.Wrapf(errors.ErrInvalidHashField, "duplicate index='%d' on field='%s'", idx, fieldOpts.name)
				}
				indices[idx] = true
				fieldOpts.index = idx
			case hshTagOptionSize:
				if len(paramSplit) != 2 {
					return nil, errors.Wrapf(errors.ErrInvalidHashField, "%s.%s tag 'seq' parameter invalid: %v", ofType.Name(), fieldName, paramSplit)
				}
				val, err := strconv.Atoi(paramSplit[1])
				if err != nil {
					return nil, err
				}
				fieldOpts.elementSize = int8(val)
			default:
				return nil, errors.Wrapf(errors.ErrInvalidHashField, "%s.%s unknown tag option: %v", ofType.Name(), fieldName, paramSplit)
			}
		}
		structOpts = append(structOpts, fieldOpts)
	}

	return structOpts, nil
}

// Get's the value type. In case of pointers returns the value type.
func getObjType(o interface{}) reflect.Type {
	t := reflect.TypeOf(o)
	switch t.Kind() {
	case reflect.Ptr:
		return t.Elem()
	default:
		return t
	}
}

func getTypeName(o interface{}) string {
	return getObjType(o).Name()
}

func getObjValue(o interface{}) reflect.Value {
	t := reflect.ValueOf(o)
	switch t.Kind() {
	case reflect.Ptr:
		return t.Elem()
	default:
		return t
	}
}
