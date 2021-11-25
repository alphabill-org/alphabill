package cli

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigKey_Int(t *testing.T) {
	zero, empty, thousand, hex := "0", "", "1010", "ff"

	type fields struct {
		name  string
		value *string
	}
	tests := []struct {
		name    string
		fields  fields
		want    int
		wantErr bool
	}{
		{
			name: "empty",
			fields: fields{
				name:  "id",
				value: &empty,
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "zero",
			fields: fields{
				name:  "id",
				value: &zero,
			},
			want:    0,
			wantErr: false,
		},
		{
			name: "thousand",
			fields: fields{
				name:  "id",
				value: &thousand,
			},
			want:    1010,
			wantErr: false,
		},
		{
			name: "hex",
			fields: fields{
				name:  "id",
				value: &hex,
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "nil",
			fields: fields{
				name:  "id",
				value: nil,
			},
			want:    0,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := ConfigVariable{
				key: tt.fields.name,
				val: tt.fields.value,
			}
			got, err := v.Int()
			if (err != nil) != tt.wantErr {
				t.Errorf("Int() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConfigKey_Int64(t *testing.T) {
	zero, empty, thousand, hex := "0", "", "1010", "ff"
	min, max := strconv.Itoa(math.MinInt64), strconv.Itoa(math.MaxInt64)

	type fields struct {
		name  string
		value *string
	}
	tests := []struct {
		name    string
		fields  fields
		want    int64
		wantErr bool
	}{
		{
			name: "empty",
			fields: fields{
				name:  "id",
				value: &empty,
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "zero",
			fields: fields{
				name:  "id",
				value: &zero,
			},
			want:    0,
			wantErr: false,
		},
		{
			name: "thousand",
			fields: fields{
				name:  "id",
				value: &thousand,
			},
			want:    1010,
			wantErr: false,
		},
		{
			name: "hex",
			fields: fields{
				name:  "id",
				value: &hex,
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "nil",
			fields: fields{
				name:  "id",
				value: nil,
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "min",
			fields: fields{
				name:  "id",
				value: &min,
			},
			want:    math.MinInt64,
			wantErr: false,
		},
		{
			name: "max",
			fields: fields{
				name:  "id",
				value: &max,
			},
			want:    math.MaxInt64,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := ConfigVariable{
				key: tt.fields.name,
				val: tt.fields.value,
			}
			got, err := v.Int64()
			if (err != nil) != tt.wantErr {
				t.Errorf("Int64() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}

}

func TestConfigKey_IsPresent(t *testing.T) {
	tests := []struct {
		name      string
		configKey *ConfigVariable
		want      bool
	}{
		{
			name:      "missing value",
			configKey: ConfigVar("id"),
			want:      false,
		},
		{
			name:      "empty value",
			configKey: ConfigVar("id", ConfigOpts.DefaultValue("")),
			want:      false,
		},
		{
			name:      "present",
			configKey: ConfigVar("id", ConfigOpts.DefaultValue("default")),
			want:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.configKey.IsPresent(); got != tt.want {
				t.Errorf("IsPresent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigKey_nilSafety(t *testing.T) {
	testFunc := func(configKey *ConfigVariable) {
		var (
			v   interface{}
			err error
		)

		v, err = configKey.Int64()
		require.Error(t, err)
		require.Zero(t, v)

		v, err = configKey.Uint32()
		require.Error(t, err)
		require.Zero(t, v)

		v, err = configKey.Uint64()
		require.Error(t, err)
		require.Zero(t, v)

		v, err = configKey.Int()
		require.Error(t, err)
		require.Zero(t, v)

		v, err = configKey.StringVal()
		require.Error(t, err)
		require.Zero(t, v)

		v, err = configKey.Bool()
		require.Error(t, err)
		require.Zero(t, v)

		v, err = configKey.Bytes()
		require.Error(t, err)
		require.Zero(t, v)

		require.False(t, configKey.IsPresent())
	}

	testFunc(nil)
	testFunc(&ConfigVariable{
		key: "key",
		val: nil,
	})
}

func TestKey(t *testing.T) {
	key := ConfigVar("id")
	expected := &ConfigVariable{
		key: "id",
		val: nil,
	}
	assert.Equal(t, expected, key)
}

func TestKeyWithDefault(t *testing.T) {
	randomString := "somethingsomething"

	key := ConfigVar("host", ConfigOpts.DefaultValue(randomString))
	expected := &ConfigVariable{
		key: "host",
		val: &randomString,
	}
	assert.Equal(t, expected, key)
}
