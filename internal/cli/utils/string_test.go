package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToSnakeCase(t *testing.T) {
	tests := map[string]string{
		"msg":                "msg",
		"myName":             "my_name",
		"myName_and_surname": "my_name_and_surname",
		"MyName_andSurname":  "my_name_and_surname",
		"AaAa":               "aa_aa",
		"00aA":               "00a_a",
		"HTTPRequest":        "http_request",
		"":                   "",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, want, PascalCaseToSnakeCase(input))
		})
	}
}
