// Code below is taken from https://github.com/distribution/distribution/blob/a4d9db5a884b70be0c96dd6a7a9dbef4f2798c51/registry/client/errors.go
// Copyright 2022 github.com/distribution/distribution authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package docker

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func TestHandleErrorResponse401ValidBody(t *testing.T) {
	json := "{\"errors\":[{\"code\":\"UNAUTHORIZED\",\"message\":\"action requires authentication\"}]}"
	response := &http.Response{
		Status:     "401 Unauthorized",
		StatusCode: 401,
		Body:       nopCloser{bytes.NewBufferString(json)},
	}
	err := handleErrorResponse(response)

	expectedMsg := "unauthorized: action requires authentication"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}

func TestHandleErrorResponse401WithInvalidBody(t *testing.T) {
	json := "{invalid json}"
	response := &http.Response{
		Status:     "401 Unauthorized",
		StatusCode: 401,
		Body:       nopCloser{bytes.NewBufferString(json)},
	}
	err := handleErrorResponse(response)

	expectedMsg := "unauthorized: authentication required"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}

func TestHandleErrorResponseExpectedStatusCode400ValidBody(t *testing.T) {
	json := "{\"errors\":[{\"code\":\"DIGEST_INVALID\",\"message\":\"provided digest does not match\"}]}"
	response := &http.Response{
		Status:     "400 Bad Request",
		StatusCode: 400,
		Body:       nopCloser{bytes.NewBufferString(json)},
	}
	err := handleErrorResponse(response)

	expectedMsg := "digest invalid: provided digest does not match"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}

func TestHandleErrorResponseExpectedStatusCode404EmptyErrorSlice(t *testing.T) {
	json := `{"randomkey": "randomvalue"}`
	response := &http.Response{
		Status:     "404 Not Found",
		StatusCode: 404,
		Body:       nopCloser{bytes.NewBufferString(json)},
	}
	err := handleErrorResponse(response)

	expectedMsg := `error parsing HTTP 404 response body: no error details found in HTTP response body: "{\"randomkey\": \"randomvalue\"}"`
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}

func TestHandleErrorResponseExpectedStatusCode404InvalidBody(t *testing.T) {
	json := "{invalid json}"
	response := &http.Response{
		Status:     "404 Not Found",
		StatusCode: 404,
		Body:       nopCloser{bytes.NewBufferString(json)},
	}
	err := handleErrorResponse(response)

	expectedMsg := "error parsing HTTP 404 response body: invalid character 'i' looking for beginning of object key string: \"{invalid json}\""
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}

func TestHandleErrorResponseUnexpectedStatusCode501(t *testing.T) {
	response := &http.Response{
		Status:     "501 Not Implemented",
		StatusCode: 501,
		Body:       nopCloser{bytes.NewBufferString("{\"Error Encountered\" : \"Function not implemented.\"}")},
	}
	err := handleErrorResponse(response)

	expectedMsg := "received unexpected HTTP status: 501 Not Implemented"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}
