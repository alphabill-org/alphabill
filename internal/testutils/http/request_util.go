package testhttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func DoGet(t *testing.T, url string, response interface{}) *http.Response {
	fmt.Println("sending get request to: " + url)
	httpRes, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	defer func() {
		_ = httpRes.Body.Close()
	}()
	resBytes, _ := ioutil.ReadAll(httpRes.Body)
	fmt.Printf("GET %s response: %s\n", url, string(resBytes))
	err = json.NewDecoder(bytes.NewReader(resBytes)).Decode(response)
	require.NoError(t, err)
	return httpRes
}

func DoPost(t *testing.T, url string, req interface{}, res interface{}) *http.Response {
	reqBodyBytes, err := json.Marshal(req)
	require.NoError(t, err)
	httpRes, err := http.Post(url, "application/json", bytes.NewBuffer(reqBodyBytes))
	require.NoError(t, err)
	defer func() {
		_ = httpRes.Body.Close()
	}()
	resBytes, _ := ioutil.ReadAll(httpRes.Body)
	fmt.Printf("POST %s response: %s\n", url, string(resBytes))
	err = json.NewDecoder(bytes.NewReader(resBytes)).Decode(res)
	require.NoError(t, err)
	return httpRes
}
