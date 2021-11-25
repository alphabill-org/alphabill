package viper

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	testfile "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/file"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testEnvPrefix = "TEST_AB"
)

type (
	singleArgTestStruct struct {
		Msg string
	}
	customSnakeCaseArgTestStruct struct {
		Msg string `mapstructure:"special_message"`
	}
	customCamelCaseArgTestStruct struct {
		Msg string `mapstructure:"mySpecialMessage"`
	}
	defaultArgTestStruct struct {
		Msg string `default:"hello"`
	}
	optionalArgTestStruct struct {
		PointerValue *int
	}
	camelCaseTestStruct struct {
		CoolLongMessageInCamelCase singleArgTestStruct
	}
	allCapsCamelCaseTestStruct struct {
		CamelURI string
	}
	customInterfaceTypeStruct struct {
		InterfaceKey SomeInterface
	}
	customNameInterfaceTypeStruct struct {
		InterfaceKey SomeInterface `mapstructure:"privateKey"`
	}

	complexStruct struct {
		Message string
		Length  int
	}
	sliceStruct struct {
		Structs []*complexStruct
	}

	innerStruct struct {
		Embedded struct {
			Msg    string
			Number int
		}
	}

	outerStruct struct {
		Inner innerStruct
	}

	SomeInterface interface {
		myValue() []byte
	}

	someInterfaceImpl struct {
		value []byte
	}
)

func (s *someInterfaceImpl) myValue() []byte {
	return s.value
}

func NewSomeInterface(value []byte) *someInterfaceImpl {
	return &someInterfaceImpl{value}
}

func Test_viperConfigurationUnmarshaler_NilSafety(t *testing.T) {
	require.NotPanics(t, func() {
		var unmarshaler *ConfigurationUnmarshaler = nil
		require.Error(t, unmarshaler.Unmarshal(&struct{}{}))

		unmarshaler = NewConfigurationUnmarshaler(true, true, true, "")
		require.Error(t, unmarshaler.Unmarshal(struct{}{}), "expected an error on non-pointer type")
		require.Error(t, unmarshaler.Unmarshal(nil), "expected an error on nil")
		require.Error(t, unmarshaler.Unmarshal(&struct{}{}, "./non-existent-file-random"),
			"expected an error on file which does not exist")
	})
}

func Test_viperConfigurationUnmarshaler_Unmarshal(t *testing.T) {
	type fields struct {
		setDefaults      bool
		validate         bool
		readEnvVariables bool
	}
	type args struct {
		value            interface{}
		envValues        map[string]string
		confFileContents []string
	}
	tests := []struct {
		name           string
		fields         fields
		deserializers  map[reflect.Type]reflectTypeDeserializer
		args           args
		valueAssertion func(t testing.TB, value interface{})
		wantErr        bool
	}{
		{
			name:   "single yaml arg",
			fields: fields{true, true, true},
			args: args{
				value:            &singleArgTestStruct{},
				envValues:        nil,
				confFileContents: []string{`msg: "hello world"`},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Equal(t, "hello world", value.(*singleArgTestStruct).Msg)
			},
			wantErr: false,
		},
		{
			name:   "single custom snake_cased arg",
			fields: fields{true, true, true},
			args: args{
				value:            &customSnakeCaseArgTestStruct{},
				envValues:        nil,
				confFileContents: []string{`special_message: "hello special"`},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Equal(t, "hello special", value.(*customSnakeCaseArgTestStruct).Msg)
			},
			wantErr: false,
		},
		{
			name:   "single custom camel-cased arg",
			fields: fields{true, true, true},
			args: args{
				value:            &customCamelCaseArgTestStruct{},
				envValues:        nil,
				confFileContents: []string{`mySpecialMessage: "hello special"`},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Equal(t, "hello special", value.(*customCamelCaseArgTestStruct).Msg)
			},
			wantErr: false,
		},
		{
			name:   "simple default value arg",
			fields: fields{true, true, true},
			args: args{
				value:            &defaultArgTestStruct{},
				envValues:        nil,
				confFileContents: nil,
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Equal(t, "hello", value.(*defaultArgTestStruct).Msg)
			},
			wantErr: false,
		},
		{
			name:   "optional value arg",
			fields: fields{true, true, true},
			args: args{
				value:            &optionalArgTestStruct{},
				envValues:        nil,
				confFileContents: nil,
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Nil(t, value.(*optionalArgTestStruct).PointerValue)
			},
			wantErr: false,
		},
		{
			name:   "optional value arg in second yaml file",
			fields: fields{true, true, true},
			args: args{
				value:            &optionalArgTestStruct{},
				envValues:        nil,
				confFileContents: []string{`pointerValue: `, `pointerValue: 10`},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.NotNil(t, value.(*optionalArgTestStruct).PointerValue)
				require.Equal(t, 10, *value.(*optionalArgTestStruct).PointerValue)
			},
			wantErr: false,
		},
		{
			name:   "second file will preserve first file args",
			fields: fields{true, true, true},
			args: args{
				value:            &optionalArgTestStruct{},
				envValues:        nil,
				confFileContents: []string{`pointerValue: 20`, `randomValue: 10`},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.NotNil(t, value.(*optionalArgTestStruct).PointerValue)
				require.Equal(t, 20, *value.(*optionalArgTestStruct).PointerValue)
			},
			wantErr: false,
		},
		{
			name:   "default ignored when default disabled",
			fields: fields{false, true, true},
			args: args{
				value:            &defaultArgTestStruct{},
				envValues:        nil,
				confFileContents: nil,
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Equal(t, "", value.(*defaultArgTestStruct).Msg)
			},
			wantErr: false,
		},
		{
			name:   "default arg overriden by yaml",
			fields: fields{true, true, true},
			args: args{
				value:            &defaultArgTestStruct{},
				envValues:        nil,
				confFileContents: []string{`msg: "hello world"`},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Equal(t, "hello world", value.(*defaultArgTestStruct).Msg)
			},
			wantErr: false,
		},
		{
			name:   "env arg overrides yaml and default value",
			fields: fields{true, true, true},
			args: args{
				value:            &defaultArgTestStruct{},
				envValues:        map[string]string{testEnvPrefix + "_MSG": "hello from env"},
				confFileContents: []string{`msg: "hello world"`},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Equal(t, "hello from env", value.(*defaultArgTestStruct).Msg)
			},
			wantErr: false,
		},
		{
			name:   "env arg is ignored when disabled",
			fields: fields{true, true, false},
			args: args{
				value:            &singleArgTestStruct{},
				envValues:        map[string]string{testEnvPrefix + "_MSG": "hello from env"},
				confFileContents: []string{`msg: "hello world"`},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Equal(t, "hello world", value.(*singleArgTestStruct).Msg)
			},
			wantErr: false,
		},
		{
			name:   "second file arg overwrites first file arg",
			fields: fields{true, true, true},
			args: args{
				value:            &singleArgTestStruct{},
				envValues:        nil,
				confFileContents: []string{`msg: "hello first"`, `msg: "hello second"`},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Equal(t, "hello second", value.(*singleArgTestStruct).Msg)
			},
			wantErr: false,
		},
		{
			name:   "embedded struct unmarshalls and validates successfully",
			fields: fields{true, true, true},
			args: args{
				value:            &outerStruct{},
				envValues:        map[string]string{testEnvPrefix + "_INNER_EMBEDDED_MSG": "env_hello"},
				confFileContents: []string{`inner: {embedded: {msg: "hello", number: 6}}`},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Equal(t, "env_hello", value.(*outerStruct).Inner.Embedded.Msg)
				require.Equal(t, 6, value.(*outerStruct).Inner.Embedded.Number)
			},
			wantErr: false,
		},
		{
			name:   "camel case struct converted to underscore environment variable",
			fields: fields{true, true, true},
			args: args{
				value:            &camelCaseTestStruct{},
				envValues:        map[string]string{testEnvPrefix + "_COOL_LONG_MESSAGE_IN_CAMEL_CASE_MSG": "hello from env"},
				confFileContents: nil,
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Equal(t, "hello from env", value.(*camelCaseTestStruct).CoolLongMessageInCamelCase.Msg)
			},
			wantErr: false,
		},
		{
			name:   "all caps camel case struct converted to underscore environment variable",
			fields: fields{true, true, true},
			args: args{
				value:            &allCapsCamelCaseTestStruct{},
				envValues:        map[string]string{testEnvPrefix + "_CAMEL_URI": "hello-uri"},
				confFileContents: nil,
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Equal(t, "hello-uri", value.(*allCapsCamelCaseTestStruct).CamelURI)
			},
			wantErr: false,
		},
		{
			name:   "all caps camel case struct converted to camel case yaml",
			fields: fields{true, true, true},
			args: args{
				value:            &allCapsCamelCaseTestStruct{},
				envValues:        nil,
				confFileContents: []string{`camelUri: "hello-from-yaml"`},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Equal(t, "hello-from-yaml", value.(*allCapsCamelCaseTestStruct).CamelURI)
			},
			wantErr: false,
		},
		{
			name:   "unmarshalling custom interface type",
			fields: fields{true, true, true},
			args: args{
				value:            &customInterfaceTypeStruct{},
				envValues:        nil,
				confFileContents: []string{`interfaceKey: "YWJj"`}, // 'abc' encoded in base64
			},
			deserializers: map[reflect.Type]reflectTypeDeserializer{
				reflect.TypeOf((*SomeInterface)(nil)).Elem(): func(bytes []byte) (interface{}, error) {
					return NewSomeInterface(bytes), nil
				},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.NotNil(t, value.(*customInterfaceTypeStruct).InterfaceKey)
				require.Equal(t, []byte("abc"), value.(*customInterfaceTypeStruct).InterfaceKey.myValue())
			},
			wantErr: false,
		},
		{
			name:   "unmarshalling custom name interface type as env variable",
			fields: fields{true, true, true},
			args: args{
				value:            &customNameInterfaceTypeStruct{},
				envValues:        map[string]string{testEnvPrefix + "_PRIVATE_KEY": "YWJj"}, // 'abc' encoded in base64
				confFileContents: []string{`privateKey: ""`, ``},
			},
			deserializers: map[reflect.Type]reflectTypeDeserializer{
				reflect.TypeOf((*SomeInterface)(nil)).Elem(): func(bytes []byte) (interface{}, error) {
					return NewSomeInterface(bytes), nil
				},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.NotNil(t, value.(*customNameInterfaceTypeStruct).InterfaceKey)
				require.Equal(t, []byte("abc"), value.(*customNameInterfaceTypeStruct).InterfaceKey.myValue())
			},
			wantErr: false,
		},
		{
			name:   "pointer is nil when arg missing",
			fields: fields{true, true, true},
			args: args{
				value:            &customInterfaceTypeStruct{},
				envValues:        nil,
				confFileContents: []string{``, `interfaceKey:`, `interfaceKey: ""`, ``},
			},
			deserializers: map[reflect.Type]reflectTypeDeserializer{
				reflect.TypeOf((*SomeInterface)(nil)).Elem(): func(bytes []byte) (interface{}, error) {
					return NewSomeInterface(bytes), nil
				},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Nil(t, value.(*customInterfaceTypeStruct).InterfaceKey)
			},
			wantErr: false,
		},
		{
			name:   "unmarshal fails when deserializer returns error",
			fields: fields{true, true, true},
			args: args{
				value:            &customInterfaceTypeStruct{},
				envValues:        nil,
				confFileContents: []string{`interfaceKey: "invalid data"`},
			},
			deserializers: map[reflect.Type]reflectTypeDeserializer{
				reflect.TypeOf((*SomeInterface)(nil)).Elem(): func(bytes []byte) (interface{}, error) {
					return nil, errors.New("expected test error")
				},
			},
			wantErr: true,
		},
		{
			name:   "unmarshal slice",
			fields: fields{true, true, true},
			args: args{
				value:     &sliceStruct{},
				envValues: nil,
				confFileContents: []string{`structs: {}`, `
structs:
  - message: "hello"
    length: 5
  - message: "bye"
    length: 3
`},
			},
			valueAssertion: func(t testing.TB, value interface{}) {
				require.Equal(t, []*complexStruct{
					{"hello", 5},
					{"bye", 3},
				}, value.(*sliceStruct).Structs)
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deserializer := NewBase64StringDeserializer()
			v := NewConfigurationUnmarshaler(tt.fields.setDefaults, tt.fields.validate, tt.fields.readEnvVariables,
				testEnvPrefix, MultipleDecodeHookFunc(deserializer.DecodeHook))

			if tt.deserializers != nil {
				for domainType, constructor := range tt.deserializers {
					deserializer.RegisterDeserializer(domainType, constructor)
				}
			}

			var filePaths []string
			tempDir := os.TempDir()
			for i, content := range tt.args.confFileContents {
				fileName := fmt.Sprintf("%s%cconfig-%d.yaml", tempDir, os.PathSeparator, i)
				testfile.CreateTempFileWithContent(t, fileName, content)
				filePaths = append(filePaths, fileName)
			}

			if tt.args.envValues != nil {
				for key, value := range tt.args.envValues {
					require.NoError(t, os.Setenv(key, value), "failed to set environment variable '%s=%s' for test")
					deferKey := key // variable `key` is overwritten by next loop iteration
					//goland:noinspection GoDeferInLoop
					defer func() {
						deferErr := os.Unsetenv(deferKey)
						if deferErr != nil {
							log.Warning("[TEST] Failed to clear test env variable %s: %s", deferKey, deferErr)
						}
					}()
				}
			}

			err := v.Unmarshal(tt.args.value, filePaths...)
			if tt.wantErr {
				require.Error(t, err, "expected error when unmarshalling configuration")
			} else {
				assert.NoError(t, err, "expected no error when unmarshalling configuration")
				tt.valueAssertion(t, tt.args.value)
			}
		})
	}
}
