package testhttp

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func DoGet(t *testing.T, url string, response interface{}) *http.Response {
	httpRes, resBytes, err := doGet(url)
	if err != nil {
		return nil
	}
	err = json.NewDecoder(bytes.NewReader(resBytes)).Decode(response)
	require.NoError(t, err)
	return httpRes
}

func DoGetProto(t *testing.T, url string, response proto.Message) *http.Response {
	httpRes, resBytes, err := doGet(url)
	if err != nil {
		return nil
	}
	err = protojson.Unmarshal(resBytes, response)
	require.NoError(t, err)
	return httpRes
}

func DoPost(t *testing.T, url string, req interface{}, res interface{}) *http.Response {
	reqBody, err := json.Marshal(req)
	require.NoError(t, err)
	return doPost(t, url, reqBody, res)
}

func DoPostProto(t *testing.T, url string, req proto.Message, res interface{}) *http.Response {
	reqBody, err := protojson.Marshal(req)
	require.NoError(t, err)
	return doPost(t, url, reqBody, res)
}

func doGet(url string) (*http.Response, []byte, error) {
	httpRes, err := http.Get(url) // #nosec G107
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		_ = httpRes.Body.Close()
	}()
	resBytes, _ := ioutil.ReadAll(httpRes.Body)
	return httpRes, resBytes, nil
}

func doPost(t *testing.T, url string, reqBody []byte, res interface{}) *http.Response {
	httpRes, err := http.Post(url, "application/json", bytes.NewBuffer(reqBody)) // #nosec G107
	if err != nil {
		return nil
	}
	defer func() {
		_ = httpRes.Body.Close()
	}()
	resBytes, _ := ioutil.ReadAll(httpRes.Body)
	err = json.NewDecoder(bytes.NewReader(resBytes)).Decode(res)
	require.NoError(t, err)
	return httpRes
}
