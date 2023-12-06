package testhttp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/fxamacker/cbor/v2"
)

func DoGetJson(url string, response interface{}) (*http.Response, error) {
	httpRes, resBytes, err := doGet(url)
	if err != nil {
		return nil, err
	}
	err = json.NewDecoder(bytes.NewReader(resBytes)).Decode(response)
	if err != nil {
		return nil, err
	}
	return httpRes, nil
}

func DoGetCbor(url string, response interface{}) (*http.Response, error) {
	httpRes, resBytes, err := doGet(url)
	if err != nil {
		return nil, err
	}
	err = cbor.NewDecoder(bytes.NewReader(resBytes)).Decode(response)
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

func DoPostCBOR(url string, req interface{}, res interface{}) (*http.Response, error) {
	reqBody, err := cbor.Marshal(req)
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
	resBytes, err := io.ReadAll(httpRes.Body)
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

	if res == nil {
		return httpRes, nil
	}
	if err := json.NewDecoder(httpRes.Body).Decode(res); err != nil {
		return nil, err
	}
	return httpRes, nil
}
