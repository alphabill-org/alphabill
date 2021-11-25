package flags

import (
	"strings"
)

type StringSliceFlags []string

func (f *StringSliceFlags) String() string {
	return strings.Join(*f, ", ")
}

func (f *StringSliceFlags) Set(value string) error {
	*f = append(*f, value)
	return nil
}
