package util

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

func FileExists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false
		}
	}
	return true
}

func ReadJsonFile[T any](path string, res *T) (*T, error) {
	// #nosec G304
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bytes, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func WriteJsonFile[T any](path string, obj *T) error {
	b, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600) // -rw-------
}
