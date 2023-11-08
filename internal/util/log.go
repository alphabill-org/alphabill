package util

import (
	"encoding/json"
	"fmt"
)

// EncodeToJsonHelper encodes to argument to json and returns it as string,
// on encode error return error message as string
func EncodeToJsonHelper(arg interface{}) string {
	j, err := json.MarshalIndent(arg, "", "\t")
	if err != nil {
		return fmt.Sprintf("json encode error: %v", err)
	}
	return string(j)
}
