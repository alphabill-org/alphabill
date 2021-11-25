package cli_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/cli"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/cli/mocks"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	testfile "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/file"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCLI_ConfigurationVariableLookuper(t *testing.T) {
	type lookupResult struct {
		value string
		ok    bool
	}

	type variable struct {
		key          string
		hasDefault   bool
		defaultValue string
		lookupResult lookupResult
		assertFunc   func(t testing.TB, configVariable *cli.ConfigVariable)
	}

	tests := []struct {
		name      string
		variables []variable
	}{
		{
			name: "simple string variable",
			variables: []variable{
				{
					key:        "key",
					hasDefault: false,
					lookupResult: lookupResult{
						value: "value",
						ok:    true,
					},
					assertFunc: func(t testing.TB, configVariable *cli.ConfigVariable) {
						s, err := configVariable.StringVal()
						require.NoError(t, err)
						require.Equal(t, "value", s)
					},
				},
			},
		},
		{
			name: "missing variable",
			variables: []variable{
				{
					key:        "key",
					hasDefault: false,
					lookupResult: lookupResult{
						value: "",
						ok:    false,
					},
					assertFunc: func(t testing.TB, configVariable *cli.ConfigVariable) {
						s, err := configVariable.StringVal()
						require.Error(t, err)
						require.Empty(t, s)
					},
				},
			},
		},
		{
			name: "default variable overriden",
			variables: []variable{
				{
					key:          "key",
					hasDefault:   true,
					defaultValue: "1",
					lookupResult: lookupResult{
						value: "2",
						ok:    true,
					},
					assertFunc: func(t testing.TB, configVariable *cli.ConfigVariable) {
						i, err := configVariable.Int()
						require.NoError(t, err)
						require.Equal(t, 2, i)
					},
				},
			},
		},
		{
			name: "default variable",
			variables: []variable{
				{
					key:          "key",
					hasDefault:   true,
					defaultValue: "default",
					lookupResult: lookupResult{
						value: "",
						ok:    false,
					},
					assertFunc: func(t testing.TB, configVariable *cli.ConfigVariable) {
						s, err := configVariable.StringVal()
						require.NoError(t, err)
						require.Equal(t, "default", s)
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test.MustRunInTime(t, time.Second, func() {
				componentName := "test-component"

				mockStarter := new(mocks.ComponentStarter)
				mockLookup := new(mocks.ConfigurationVariableLookuper)
				testVariables := make(map[string]*cli.ConfigVariable)

				starterSupplier := func(_ context.Context) (cli.ComponentStarter, error) {
					for _, v := range tt.variables {
						v.assertFunc(t, testVariables[v.key])
					}
					return mockStarter, nil
				}
				testCLI, err := cli.New(componentName, nil, starterSupplier)
				require.NoError(t, err)
				require.NoError(t, testCLI.ConfigVariableLookup(mockLookup))

				for _, v := range tt.variables {
					mockLookup.On("LookupVariable", componentName, v.key).Return(v.lookupResult.value,
						v.lookupResult.ok)
					configVar := cli.ConfigVar(v.key)
					if v.hasDefault {
						configVar = cli.ConfigVar(v.key, cli.ConfigOpts.DefaultValue(v.defaultValue))
					}
					testVariables[v.key] = configVar
					require.NoError(t, testCLI.AddConfigurationVariables(configVar))
				}

				mockStarter.On("Start", mock.Anything).Run(func(args mock.Arguments) {
					ctx := args.Get(0).(context.Context)
					wg, err := async.WaitGroup(ctx)
					require.NoError(t, err)
					require.NotNil(t, wg)
				})

				require.NoError(t, testCLI.StartAndWait(contextWithWaitGroup()))
				mock.AssertExpectationsForObjects(t, mockStarter, mockLookup)
			})
		})
	}
}

func TestCLI_AddDuplicateConfigurationVariable(t *testing.T) {
	mockStarter := new(mocks.ComponentStarter)
	mockLookup := new(mocks.ConfigurationVariableLookuper)

	var1, var2 := cli.ConfigVar("key"), cli.ConfigVar("key")

	testCLI, err := cli.New("test-component", nil,
		func(_ context.Context) (cli.ComponentStarter, error) { return mockStarter, nil })
	require.NoError(t, err)
	require.NoError(t, testCLI.ConfigVariableLookup(mockLookup))
	require.Error(t, testCLI.AddConfigurationVariables(var1, var2))
}

func TestCLI_AddDuplicateConfigurationVariable_DifferentComponent(t *testing.T) {
	mockStarter := new(mocks.ComponentStarter)
	mockLookup := new(mocks.ConfigurationVariableLookuper)

	var1 := cli.ConfigVar("key", cli.ConfigOpts.Component("comp_A"))
	var2 := cli.ConfigVar("key", cli.ConfigOpts.Component("comp_B"))

	testCLI, err := cli.New("test-component", nil,
		func(_ context.Context) (cli.ComponentStarter, error) { return mockStarter, nil })
	require.NoError(t, err)
	require.NoError(t, testCLI.ConfigVariableLookup(mockLookup))
	require.NoError(t, testCLI.AddConfigurationVariables(var1, var2))
}

func TestCLI_AddNilConfigurationVariable(t *testing.T) {
	testCLI, err := cli.New("test-component", nil, func(_ context.Context) (cli.ComponentStarter, error) { return nil, nil })
	require.NoError(t, err)
	require.Error(t, testCLI.AddConfigurationVariables(nil))
}

func TestCLI_StartAndWait(t *testing.T) {
	test.MustRunInTime(t, time.Second, func() {
		componentName := "test-component"

		mockStarter := new(mocks.ComponentStarter)
		mockLookup := new(mocks.ConfigurationVariableLookuper)

		testCLI, err := cli.New(componentName, nil, func(_ context.Context) (cli.ComponentStarter, error) {
			return mockStarter, nil
		})
		require.NoError(t, err)
		require.NoError(t, testCLI.ConfigVariableLookup(mockLookup))

		mockStarter.On("Start", mock.Anything).Run(func(args mock.Arguments) {
			ctx := args.Get(0).(context.Context)
			wg, err := async.WaitGroup(ctx)
			require.NoError(t, err)
			require.NotNil(t, wg)
		})

		require.NoError(t, testCLI.StartAndWait(contextWithWaitGroup()))
		mock.AssertExpectationsForObjects(t, mockStarter, mockLookup)
	})
}

func TestCLI_Starter(t *testing.T) {
	test.MustRunInTime(t, time.Second, func() {
		componentName := "test-component"

		testCLI, err := cli.New(componentName, nil, nil)
		require.Error(t, err)
		require.Nil(t, testCLI)

		testCLI, err = cli.New(componentName, nil, func(_ context.Context) (cli.ComponentStarter, error) {
			return nil, errors.New("expected error")
		})
		require.NoError(t, err)
		require.NoError(t, testCLI.ConfigVariableLookup(new(mocks.ConfigurationVariableLookuper)))

		require.Error(t, testCLI.StartAndWait(contextWithWaitGroup()))
	})
}

func TestCLI_LookupNil(t *testing.T) {
	test.MustRunInTime(t, time.Second, func() {
		componentName := "test-component"

		testCLI, err := cli.New(componentName, nil, func(_ context.Context) (cli.ComponentStarter, error) {
			return new(mocks.ComponentStarter), nil
		})
		require.NoError(t, err)
		require.Error(t, testCLI.ConfigVariableLookup(nil))
	})
}

func TestCLI_NilSafety(t *testing.T) {
	test.MustRunInTime(t, time.Second, func() {
		assert.Error(t, (*cli.CLI)(nil).AddConfigurationVariables())
		assert.Error(t, (*cli.CLI)(nil).ConfigVariableLookup(nil))
		assert.Error(t, (*cli.CLI)(nil).StartAndWait(contextWithWaitGroup()))
	})
}

func TestCLI_ConfigStructNotPointer(t *testing.T) {
	_, err := cli.New("test-component", struct{}{}, func(_ context.Context) (cli.ComponentStarter, error) {
		return nil, nil
	})
	require.Error(t, err)
}

func TestCLI_ConfigPointer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	expectedSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	expectedPrivKey, err := expectedSigner.MarshalPrivateKey()
	require.NoError(t, err)

	fileName := fmt.Sprintf("%s%cconfig-%s.yaml", t.TempDir(), os.PathSeparator, t.Name())
	testfile.CreateTempFileWithContent(t, fileName, fmt.Sprintf(`
signer: "%s"
nonDefault: "non-default"`,
		base64.StdEncoding.EncodeToString(expectedPrivKey),
	))

	require.NoError(t, cli.FlagConfigurationFilePaths.Set(fileName), "failed to setup test data")

	config := &struct {
		Signer     *crypto.InMemorySecp256K1Signer
		NonDefault string
		TestString string `default:"default"`
	}{}

	mockStarter := new(mocks.ComponentStarter)

	wg := sync.WaitGroup{}
	wg.Add(1)
	mockStarter.On("Start", mock.Anything).Run(func(args mock.Arguments) {
		wg.Done()
	}).Return(nil)
	c, err := cli.New("test-component", config, func(_ context.Context) (cli.ComponentStarter, error) {
		return mockStarter, nil
	})

	require.NoError(t, err)
	ctx, _ = async.WithWaitGroup(ctx)
	require.NoError(t, c.StartAndWait(ctx))

	test.MustRunInTime(t, 100*time.Millisecond, func() {
		wg.Wait()
		actualPrivKey, err := config.Signer.MarshalPrivateKey()
		require.NoError(t, err)
		require.Equal(t, "non-default", config.NonDefault)
		require.Equal(t, "default", config.TestString)
		require.Equal(t, expectedPrivKey, actualPrivKey)
	})
}

func TestCLI_MissingWaitGroup(t *testing.T) {
	componentName := "test-component"

	mockStarter := new(mocks.ComponentStarter)
	mockStarter.On("Start", mock.Anything).Return(nil)

	testCLI, err := cli.New(componentName, nil, func(_ context.Context) (cli.ComponentStarter, error) {
		return mockStarter, nil
	})
	require.NoError(t, err)

	test.MustRunInTime(t, time.Second, func() {
		assert.NoError(t, testCLI.StartAndWait(contextWithWaitGroup()))
		assert.Error(t, testCLI.StartAndWait(context.Background()))
	})
}

func contextWithWaitGroup() context.Context {
	ctx, _ := async.WithWaitGroup(context.Background())
	return ctx
}
