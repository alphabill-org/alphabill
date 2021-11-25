package lookup_test

import (
	"encoding/base64"
	"math/rand"
	"os"
	"strings"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/cli/lookup"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"github.com/stretchr/testify/require"
)

func Test_environmentVariableLookup_LookupVariable(t *testing.T) {
	type args struct {
		componentName string
		key           string
	}
	tests := []struct {
		name        string
		lookup      lookup.EnvironmentVariableLookuper
		args        args
		expectedKey string
	}{
		{
			name:   "default prefix",
			lookup: lookup.AlphaBillEnvironmentVariableLookuper,
			args: args{
				componentName: "bsn",
				key:           "key",
			},
			expectedKey: "AB_BSN_KEY",
		},
		{
			name:   "custom prefix",
			lookup: lookup.EnvironmentVariableLookuper("AB_unit_test"),
			args: args{
				componentName: "some_component",
				key:           "conf_key",
			},
			expectedKey: "AB_UNIT_TEST_SOME_COMPONENT_CONF_KEY",
		},
	}

	// Clear test generated environment variables
	originalEnvironment := os.Environ()
	var createdKeys []string
	defer func() {
		for _, key := range createdKeys {
			var foundOriginal = false
			for _, originalKeyValue := range originalEnvironment {
				if strings.HasPrefix(originalKeyValue, key+"=") {
					_ = os.Setenv(originalKeyValue[0:len(key)], originalKeyValue[len(key)+1:])
					foundOriginal = true
					break
				}
			}
			if !foundOriginal {
				_ = os.Unsetenv(key)
			}
		}
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			randomString := base64.StdEncoding.EncodeToString(test.RandomBytes(rand.Intn(1000) + 1))

			result, ok := tt.lookup.LookupVariable(tt.args.componentName, tt.args.key)
			require.False(t, ok)
			require.Empty(t, result)

			createdKeys = append(createdKeys, tt.expectedKey)
			require.NoError(t, os.Setenv(tt.expectedKey, randomString), "failed to set environment variable for testing")

			result, ok = tt.lookup.LookupVariable(tt.args.componentName, tt.args.key)
			require.True(t, ok)
			require.Equal(t, randomString, result)
		})
	}
}
