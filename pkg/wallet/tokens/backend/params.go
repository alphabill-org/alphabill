package backend

import (
	"fmt"
	"strconv"
)

/*
parseMaxResponseItems parses input "s" as integer.
When empty string or int over "maxValue" is sent in "maxValue" is returned.
In case of invalid int or value smaller than 1 error is returned.
*/
func parseMaxResponseItems(s string, maxValue int) (int, error) {
	if s == "" {
		return maxValue, nil
	}

	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %q as integer: %w", s, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("value must be greater than zero, got %d", v)
	}

	if v > maxValue {
		return maxValue, nil
	}
	return v, nil
}
