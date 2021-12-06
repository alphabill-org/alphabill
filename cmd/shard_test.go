package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
)

type envVar [2]string

func TestShardConfig_Table(t *testing.T) {
	tests := []struct {
		args           string   // arguments as a space separated string
		envVars        []envVar // Environment variables that will be set before creating command
		expectedConfig *shardConfiguration
	}{
		// Root configuration permutations
		{
			args:           "shard",
			expectedConfig: defaultShardConfiguration(),
		}, {
			args: "shard --home=/custom-home",
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.Root = &rootConfiguration{
					HomeDir: "/custom-home",
					CfgFile: "/custom-home" + string(os.PathSeparator) + defaultConfigFile,
				}
				return sc
			}(),
		}, {
			args: "shard --home=/custom-home --config=custom-config.props",
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.Root = &rootConfiguration{
					HomeDir: "/custom-home",
					CfgFile: "/custom-home/custom-config.props",
				}
				return sc
			}(),
		}, {
			args: "shard --config=custom-config.props",
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.Root = &rootConfiguration{
					HomeDir: defaultHomeDir,
					CfgFile: defaultHomeDir + "/custom-config.props",
				}
				return sc
			}(),
		},
		// Shard configuration from flags
		{
			args: "shard --initial-bill-value=555",
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.InitialBillValue = 555
				return sc
			}(),
		}, {
			args: "shard --server-address=srv:1234 --server-max-recv-msg-size=66 --server-max-connection-age-ms=77 --server-max-connection-age-grace-ms=88",
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.Server = &serverConfiguration{
					Address:                 "srv:1234",
					MaxRecvMsgSize:          66,
					MaxConnectionAgeMs:      77,
					MaxConnectionAgeGraceMs: 88,
				}
				return sc
			}(),
		},
		// Shard configuration from ENV
		{
			args: "shard",
			envVars: []envVar{
				{"AB_INITIAL_BILL_VALUE", "555"},
				{"AB_SERVER_ADDRESS", "srv:1234"},
			},
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.InitialBillValue = 555
				sc.Server.Address = "srv:1234"
				return sc
			}(),
		}, {
			args: "shard --initial-bill-value=666 --server-address=srv:666",
			envVars: []envVar{
				{"AB_INITIAL_BILL_VALUE", "555"},
				{"AB_SERVER_ADDRESS", "srv:1234"},
			},
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.InitialBillValue = 666
				sc.Server.Address = "srv:666"
				return sc
			}(),
		},
	}
	for _, tt := range tests {
		t.Run("shard_conf|"+tt.args+"|"+envVarsStr(tt.envVars), func(t *testing.T) {
			var actualConfig *shardConfiguration
			shardRunFunc := func(sc *shardConfiguration) error {
				actualConfig = sc
				return nil
			}

			// Set environment variables only for single test.
			for _, en := range tt.envVars {
				err := os.Setenv(en[0], en[1])
				require.NoError(t, err)
				defer os.Unsetenv(en[0])
			}

			abApp := New()
			abApp.rootCmd.SetArgs(strings.Split(tt.args, " "))
			abApp.Execute(Opts.ShardRunFunc(shardRunFunc))

			assert.Equal(t, tt.expectedConfig, actualConfig)
		})
	}
}

func TestShardConfig_ConfigFile(t *testing.T) {
	configFileContents := `
initial-bill-value=666
server-address=srv:1234
server-max-recv-msg-size=9999
`

	// Write temporary file and clean up afterwards
	f, err := os.CreateTemp("", "example-conf.*.props")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write([]byte(configFileContents)); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	expectedConfig := defaultShardConfiguration()
	expectedConfig.Root.CfgFile = f.Name()
	expectedConfig.InitialBillValue = 666
	expectedConfig.Server.Address = "srv:1234"
	expectedConfig.Server.MaxRecvMsgSize = 9999

	// Set up shard runner mock
	var actualConfig *shardConfiguration
	shardRunFunc := func(sc *shardConfiguration) error {
		actualConfig = sc
		return nil
	}

	abApp := New()
	args := "shard --config=" + f.Name()
	abApp.rootCmd.SetArgs(strings.Split(args, " "))
	abApp.Execute(Opts.ShardRunFunc(shardRunFunc))

	assert.Equal(t, expectedConfig, actualConfig)
}

func defaultShardConfiguration() *shardConfiguration {
	return &shardConfiguration{
		Root: &rootConfiguration{
			HomeDir: defaultHomeDir,
			CfgFile: defaultHomeDir + string(os.PathSeparator) + defaultConfigFile,
		},
		Server: &serverConfiguration{
			Address:        defaultServerAddr,
			MaxRecvMsgSize: defaultMaxRecvMsgSize,
		},
		InitialBillValue: defaultInitialBillValue,
	}
}

// envVarsStr creates sting for test names from envVars
func envVarsStr(envVars []envVar) (out string) {
	if len(envVars) == 0 {
		return
	}
	out += "ENV:"
	for i, ev := range envVars {
		if i > 0 {
			out += "&"
		}
		out += ev[0] + "=" + ev[1]
	}
	return
}
