package testfile

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// CreateTempFileWithContent Creates a file in the specified filepath with the provided content
// The file will be deleted automatically when test finishes (using testing Cleanup)
func CreateTempFileWithContent(t testing.TB, filePath string, content string) {
	buf := bytes.Buffer{}
	buf.WriteString(content)
	err := ioutil.WriteFile(filePath, buf.Bytes(), 0600)
	require.NoError(t, err, "failed to create test file '%s'", filePath)
	t.Cleanup(func() {
		if err = os.Remove(filePath); err != nil {
			t.Errorf("CreateTempFileWithContent Remove cleanup: %v", err)
		}
	})
}
