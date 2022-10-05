package util

import "net/url"

func IsValidURI(u string) bool {
	url, err := url.ParseRequestURI(u)
	if err != nil {
		return false
	}
	return url != nil
}
