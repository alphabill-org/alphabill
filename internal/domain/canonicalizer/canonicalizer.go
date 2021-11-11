package canonicalizer

import (
	"encoding/binary"
	"reflect"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
)

var log = logger.CreateForPackage()

type (
	// Canonicalizer Interface for serializing data structures into deterministic hash-format.
	Canonicalizer interface {
		Canonicalize() ([]byte, error)
	}

	// Option represent serialization optional parameters.
	Option func(*serializeOptions) error

	serializeOptions struct {
		template []fieldOptions
		extras   []fieldExtras
	}

	fieldExtras struct {
		exclude bool
	}
)

// Canonicalize returns hash-format representation of the data structure. Use
// the opts parameter for applying additional serialization options.
//
// Nested types must implement Canonicalizer interface.
//
// The implementation makes use of struct tags with the key 'hsh' as serialization
// template. Available options:
//  - 'idx'  [mandatory]: binary serialization sequence for given field.
//  - 'size' [optional] : integer value size expressed in bytes. Supported values 1, 4, 8.
//                        If not set 1 is used as default.
// eg.
//  type Object struct {
//	    Number uint64 `hsh:"idx=1,size=8"`
//	    Slice  []byte `hsh:"idx=2"`
//  }
func Canonicalize(obj interface{}, opts ...Option) ([]byte, error) {
	if obj == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, errstr.NilArgument)
	}

	// Extract field serialization templates
	typeName := getTypeName(obj)
	fieldOpts, registered := registry[typeName]
	if !registered {
		return nil, errors.Wrapf(errors.ErrInvalidHashField, "template is not registered for type: %s", typeName)
	}
	options := serializeOptions{
		template: fieldOpts,
		extras:   make([]fieldExtras, len(fieldOpts)),
	}
	// Update field options with user input.
	for _, o := range opts {
		if err := o(&options); err != nil {
			return nil, err
		}
	}

	return extractDataBytes(obj, options)
}

// OptionExcludeField provides the ability for excluding fields from serialization.
func OptionExcludeField(name string) Option {
	return func(o *serializeOptions) error {
		if o == nil {
			return errors.Wrap(errors.ErrInvalidArgument, "missing options")
		}
		if name == "" {
			return errors.Wrap(errors.ErrInvalidArgument, "field name not specified")
		}

		for i := 0; i < len(o.template); i++ {
			if o.template[i].name == name {
				o.extras[i].exclude = true
				return nil
			}
		}
		return errors.Wrapf(errors.ErrInvalidHashField, "exclude field='%s' not found", name)
	}
}

func extractDataBytes(obj interface{}, opts serializeOptions) ([]byte, error) {
	var (
		data     []byte
		objValue = getObjValue(obj)
	)
	for i := 0; i < len(opts.template); i++ {
		if opts.extras[i].exclude {
			log.Debug("'%s' excluding field='%s' from serialization.", objValue.Type().Name(), opts.template[i].name)
			continue
		}

		fieldDataBytes, err := extractFieldDataBytes(
			objValue.FieldByName(opts.template[i].name), opts.template[i])
		if err != nil {
			return nil, err
		}
		data = append(data, fieldDataBytes...)
	}
	return data, nil
}

// extractFieldDataBytes returns data bytes for a given field. The functions is used recursively.
func extractFieldDataBytes(fieldValue reflect.Value, opts fieldOptions) ([]byte, error) {
	var (
		bin []byte
		err error

		fieldKind = fieldValue.Kind()
	)

	switch fieldKind {
	case reflect.Ptr:
		val, ok := fieldValue.Interface().(Canonicalizer)
		if !ok {
			return nil, errors.Wrapf(errors.ErrInvalidHashField, "field='%s' value does not implement Canonicalizer (type=%T)",
				opts.name, fieldValue.Interface())
		}
		bin, err = val.Canonicalize()
		if err != nil {
			return nil, err
		}
	case reflect.Struct:
		// First convert to value pointer
		vp := reflect.New(fieldValue.Type())
		vp.Elem().Set(fieldValue)
		val, ok := vp.Interface().(Canonicalizer)
		if !ok {
			return nil, errors.Wrapf(errors.ErrInvalidHashField, "field='%s' value does not implement Canonicalizer (type=%T)",
				opts.name, fieldValue.Interface())
		}
		bin, err = val.Canonicalize()
		if err != nil {
			return nil, err
		}
	case reflect.Slice:
		switch fieldValue.Type().Elem().Kind() {
		case reflect.Uint8: // Optimization for byte slices.
			bin = fieldValue.Interface().([]byte)
		default:
			for i := 0; i < fieldValue.Len(); i++ {
				tmp, err := extractFieldDataBytes(fieldValue.Index(i), opts)
				if err != nil {
					return nil, err
				}
				bin = append(bin, tmp...)
			}
		}
	case reflect.Uint8, reflect.Uint32, reflect.Uint64:
		switch opts.elementSize {
		case -1, 1:
			bin = []byte{byte(fieldValue.Uint())}
		case 4:
			bin = make([]byte, opts.elementSize)
			binary.BigEndian.PutUint32(bin, uint32(fieldValue.Uint()))
		case 8:
			bin = Uint64ToBytes(fieldValue.Uint())
		default:
			return nil, errors.Wrapf(errors.ErrInvalidHashField,
				"field='%s' unsupported uint size: %d", opts.name, opts.elementSize)
		}
	default:
		return nil, errors.Wrapf(errors.ErrInvalidHashField, " field='%s' of type='%s' is unhandled kind: %s",
			opts.name, fieldValue.Type().Name(), fieldKind)
	}
	return bin, nil
}
