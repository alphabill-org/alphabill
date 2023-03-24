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
	require.NoError(t, err)
	err = json.NewDecoder(bytes.NewReader(resBytes)).Decode(response)
	require.NoError(t, err)
	return httpRes
}

func DoGetProto(url string, response proto.Message) (*http.Response, error) {
	httpRes, resBytes, err := doGet(url)
	if err != nil {
		return nil, err
	}
	err = protojson.Unmarshal(resBytes, response)
	if err != nil {
		return nil, err
	}
	return httpRes, nil
}

func DoPost(url string, req interface{}, res interface{}) (*http.Response, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return doPost(url, reqBody, res)
}

func DoPostProto(url string, req proto.Message, res interface{}) (*http.Response, error) {
	reqBody, err := protojson.Marshal(req)
	if err != nil {
		return nil, err
	}
	return doPost(url, reqBody, res)
}

func doGet(url string) (*http.Response, []byte, error) {
	httpRes, err := http.Get(url) // #nosec G107
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		_ = httpRes.Body.Close()
	}()
	resBytes, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return nil, nil, err
	}
	return httpRes, resBytes, nil
}

func doPost(url string, reqBody []byte, res interface{}) (*http.Response, error) {
	httpRes, err := http.Post(url, "application/json", bytes.NewBuffer(reqBody)) // #nosec G107
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = httpRes.Body.Close()
	}()
	resBytes, _ := ioutil.ReadAll(httpRes.Body)
	err = json.NewDecoder(bytes.NewReader(resBytes)).Decode(res)
	if err != nil {
		return nil, err
	}
	return httpRes, nil
}
