package viper

import (
	"reflect"
	"strings"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/cli/utils"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"

	"github.com/creasty/defaults"
	"github.com/davecgh/go-spew/spew"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	validate "gopkg.in/dealancer/validate.v2"
)

type (
	reflectTypeDeserializer func(bytes []byte) (interface{}, error)

	ConfigurationUnmarshaler struct {
		fileViper        *viper.Viper
		envViper         *viper.Viper
		setDefaults      bool   // Whether default values in struct should be used
		validate         bool   // Whether to validate the struct after unmarshal
		readEnvVariables bool   // Whether to read values from environment values
		envPrefix        string // Prefix for all env variables

		decodeHooks []mapstructure.DecodeHookFunc // mapstructure hooks for customizing unmarshalling
	}
)

var log = logger.CreateForPackage()

func NewConfigurationUnmarshaler(
	setDefaults bool, validate bool, readEnvVariables bool, envPrefix string,
	hooks ...mapstructure.DecodeHookFunc,
) *ConfigurationUnmarshaler {
	return &ConfigurationUnmarshaler{
		fileViper:        viper.New(),
		envViper:         viper.New(),
		setDefaults:      setDefaults,
		validate:         validate,
		readEnvVariables: readEnvVariables,
		envPrefix:        envPrefix,

		decodeHooks: hooks,
	}
}

func (v *ConfigurationUnmarshaler) Unmarshal(value interface{}, filePaths ...string) error {
	if v == nil {
		return errors.Wrapf(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	if reflect.ValueOf(value).Kind() != reflect.Ptr {
		return errors.Wrapf(errors.ErrInvalidArgument, "argument is not a pointer: %v", value)
	}

	if v.readEnvVariables {
		v.envViper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		err := v.registerEnvironmentVariables(value)
		if err != nil {
			return errors.Wrapf(err, "failed to register environment variables for configuration")
		}
	}

	var err error
	if v.setDefaults {
		err = defaults.Set(value)
		if err != nil {
			return errors.Wrapf(err, "failed to set defaults for configuration")
		}
	}

	err = v.unmarshal(value, filePaths)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal configuration")
	}

	if v.validate {
		err = validate.Validate(value)
		if err != nil {
			log.Trace("Dumping failed validation configuration: %s", spew.Sdump(value))
			return errors.Wrapf(err, "validation failed for unmarshalled configuration: %#v", value)
		}
	}

	return nil
}

func (v *ConfigurationUnmarshaler) unmarshal(value interface{}, filePaths []string) error {
	var decoderOptions []viper.DecoderConfigOption
	decoderOptions = append(decoderOptions,
		viper.DecodeHook(MultipleDecodeHookFunc(v.decodeHooks...)))

	if len(filePaths) > 0 {
		log.Debug("Reading configuration using files %v", filePaths)
	}
	for _, file := range filePaths {
		v.fileViper.SetConfigFile(file)
		err := v.fileViper.ReadInConfig()
		if err != nil {
			return errors.Wrapf(err, "failed to read config for file '%s'", file)
		}
		err = v.fileViper.Unmarshal(value, decoderOptions...)
		if err != nil {
			return errors.Wrapf(err, "failed to unmarshal file '%s' using viper'", file)
		}
	}

	// Read from env when no config files are provided, but env variables are enabled
	if v.readEnvVariables {
		log.Debug("Reading configuration from Environment Variables")
		err := v.envViper.Unmarshal(value, decoderOptions...)
		if err != nil {
			return errors.Wrapf(err, "failed to unmarshal environment variables using viper")
		}
	}

	return nil
}

func (v *ConfigurationUnmarshaler) registerEnvironmentVariables(value interface{}) error {
	m := map[string]interface{}{}
	err := mapstructure.Decode(value, &m)
	if err != nil {
		return errors.Wrapf(err, "failed to decode target struct for binding environment variables")
	}
	return v.registerEnvVariablesForMap(m)
}

func (v *ConfigurationUnmarshaler) registerEnvVariablesForMap(m map[string]interface{}, prefixes ...string) error {
	for key, value := range m {
		if subMap, ok := value.(map[string]interface{}); ok {
			err := v.registerEnvVariablesForMap(subMap, append(prefixes, key)...)
			if err != nil {
				return err
			}
			continue
		}

		fullKey := strings.Join(append(prefixes, key), ".")
		envVar := strings.Replace(utils.PascalCaseToSnakeCase(fullKey), "._", ".", -1)
		if len(v.envPrefix) > 0 {
			envVar = v.envPrefix + "_" + envVar
		}
		envVar = strings.ToUpper(envVar)

		err := v.envViper.BindEnv(fullKey, envVar)
		if err != nil {
			return errors.Wrapf(err, "error binding env variable '%s'", fullKey)
		}
	}
	return nil
}
