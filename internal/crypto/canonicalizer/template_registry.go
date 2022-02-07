package canonicalizer

import (
	"sort"
)

type (
	templateRegistry map[string][]fieldOptions
)

var (
	registry = make(templateRegistry)
)

func RegisterTemplate(o interface{}) {
	fieldOpts, err := fieldOptionsOf(o)
	if err != nil {
		panic(err)
	}
	sort.Sort(byIndex(fieldOpts))

	registry[getTypeName(o)] = fieldOpts
}
