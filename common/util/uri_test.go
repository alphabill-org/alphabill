package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsValidURI(t *testing.T) {
	require.True(t, IsValidURI("https://alphabill.org"))
	require.True(t, IsValidURI("ldap://[2001:db8::7]/c=GB?objectClass?one"))
	require.True(t, IsValidURI("ftp://ftp.is.co.za/rfc/rfc1808.txt"))
	require.True(t, IsValidURI("mailto:John.Doe@example.com"))
	require.True(t, IsValidURI("telnet://192.0.2.16:80/"))
	require.True(t, IsValidURI("urn:oasis:names:specification:docbook:dtd:xml:4.1.2"))
	require.False(t, IsValidURI("invalid"))
}
