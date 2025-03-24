package cmd

import (
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/stretchr/testify/require"
)

func TestKeyConfFlags_loadKeyConf(t *testing.T) {
	keyConfFile := filepath.Join(t.TempDir(), "keys.json")
	flags := &keyConfFlags{
		KeyConfFile: keyConfFile,
	}
	_, err := flags.loadKeyConf(&baseFlags{}, false)
	require.ErrorContains(t, err, "failed to load")

	keyConf, err := generateKeys()
	require.NoError(t, err)
	require.NoError(t, util.WriteJsonFile(keyConfFile, keyConf))

	loadedKeyConf, err := flags.loadKeyConf(&baseFlags{}, true)
	require.NoError(t, err)

	require.Equal(t, keyConf, loadedKeyConf)
	require.Equal(t, keyConf.SigKey, loadedKeyConf.SigKey)
	require.Equal(t, keyConf.AuthKey, loadedKeyConf.AuthKey)
}
