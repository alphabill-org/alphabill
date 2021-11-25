package cli

import (
	"encoding/base64"
	"strconv"
	"strings"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

type (
	ConfigVariable struct {
		key string
		val *string

		component string
	}

	ConfigOption     func(*ConfigVariable)
	allConfigOptions struct{}
)

var (
	ConfigOpts = &allConfigOptions{}
)

func ConfigVar(key string, opts ...ConfigOption) *ConfigVariable {
	c := &ConfigVariable{
		key: key,
		val: nil,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (o *allConfigOptions) DefaultValue(defaultValue string) ConfigOption {
	return func(c *ConfigVariable) {
		c.val = &defaultValue
	}
}

// Component specifies the component name explicitly.
func (o *allConfigOptions) Component(name string) ConfigOption {
	return func(c *ConfigVariable) {
		c.component = name
	}
}

func (v *ConfigVariable) IsPresent() bool {
	return v != nil && v.val != nil && *v.val != ""
}

func (v *ConfigVariable) IsNonEmptyValuePresent() bool {
	return v.IsPresent() && strings.TrimSpace(*v.val) != ""
}

func (v *ConfigVariable) StringVal() (string, error) {
	return v.value()
}

func (v *ConfigVariable) Int() (int, error) {
	value, err := v.value()
	if err != nil {
		return 0, err
	}
	i, err := strconv.Atoi(value)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to convert value of key '%s' to int", v.key)
	}
	return i, nil
}

func (v *ConfigVariable) Int64() (int64, error) {
	value, err := v.value()
	if err != nil {
		return 0, err
	}
	i, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to convert value of key '%s' to int64", v.key)
	}
	return i, nil
}

func (v *ConfigVariable) Uint32() (uint32, error) {
	value, err := v.value()
	if err != nil {
		return 0, err
	}
	i, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to convert value of key '%s' to uint32", v.key)
	}
	return uint32(i), nil
}

func (v *ConfigVariable) Uint64() (uint64, error) {
	value, err := v.value()
	if err != nil {
		return 0, err
	}
	i, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to convert value of key '%s' to uint64", v.key)
	}
	return i, nil
}

func (v *ConfigVariable) Bool() (bool, error) {
	value, err := v.value()
	if err != nil {
		return false, err
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return false, errors.Wrapf(err, "failed to convert value of key '%s' to bool6", v.key)
	}
	return b, nil
}

func (v *ConfigVariable) Bytes() ([]byte, error) {
	value, err := v.value()
	if err != nil {
		return nil, err
	}
	bytes, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert value of key '%s' to []byte", v.key)
	}
	return bytes, nil
}

func (v *ConfigVariable) value() (string, error) {
	if v == nil {
		return "", errors.Wrapf(errors.ErrInvalidState, "config variable is nil")
	}
	if v.IsPresent() {
		return *v.val, nil
	}

	return "", errors.Wrapf(errors.ErrInvalidState, "missing value for key '%s'", v.key)
}
