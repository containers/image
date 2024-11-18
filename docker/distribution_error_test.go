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

	"github.com/docker/distribution/registry/api/errcode"
)

func TestHandleErrorResponse401InvalidTokenChallenge(t *testing.T) {
	json := []byte(`{"errors":[{"code":"UNKNOWN","message":"some unknown error"}]}`)
	challenge := `Bearer realm="example.io",
error="invalid_token",
error_description="The access token expired"`
	response := &http.Response{
		Status:     "401 Unauthorized",
		StatusCode: 401,
		Body:       io.NopCloser(bytes.NewReader(json)),
		Header: http.Header{
			http.CanonicalHeaderKey("WWW-Authenticate"): []string{challenge},
		},
	}
	err := handleErrorResponse(response)
	if err == nil {
		t.Fatal("Expected handleErrorResponse to return error, got nil.")
	}

	expectedMsg := "unauthorized: The access token expired"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}

func TestHandleErrorResponse401InsufficientScopeChallenge(t *testing.T) {
	json := []byte(`{"errors":[{"code":"UNKNOWN","message":"some unknown error"}]}`)
	challenge := `Bearer realm="example.io",
error="insufficient_scope",
error_description="Insufficient permission"`
	response := &http.Response{
		Status:     "401 Unauthorized",
		StatusCode: 401,
		Body:       io.NopCloser(bytes.NewReader(json)),
		Header: http.Header{
			http.CanonicalHeaderKey("WWW-Authenticate"): []string{challenge},
		},
	}
	err := handleErrorResponse(response)
	if err == nil {
		t.Fatal("Expected handleErrorResponse to return error, got nil.")
	}

	expectedMsg := "denied: Insufficient permission"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}

func TestHandleErrorResponse401InvalidTokenWithoutDescription(t *testing.T) {
	json := []byte(`{"errors":[{"code":"UNKNOWN","message":"some unknown error"}]}`)
	challenge := `Bearer realm="example.io",
error="invalid_token"`
	response := &http.Response{
		Status:     "401 Unauthorized",
		StatusCode: 401,
		Body:       io.NopCloser(bytes.NewReader(json)),
		Header: http.Header{
			http.CanonicalHeaderKey("WWW-Authenticate"): []string{challenge},
		},
	}
	err := handleErrorResponse(response)
	if err == nil {
		t.Fatal("Expected handleErrorResponse to return error, got nil.")
	}

	expectedMsg := errcode.ErrorCodeUnauthorized.Message()
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}

func TestHandleErrorResponse401UnexpectedChallenge(t *testing.T) {
	json := []byte(`{"errors":[{"code":"UNKNOWN","message":"some unknown error"}]}`)
	challenge := `Bearer realm="example.io",
error="invalid_request",
error_description="The request is missing a required parameter"`
	response := &http.Response{
		Status:     "401 Unauthorized",
		StatusCode: 401,
		Body:       io.NopCloser(bytes.NewReader(json)),
		Header: http.Header{
			http.CanonicalHeaderKey("WWW-Authenticate"): []string{challenge},
		},
	}
	err := handleErrorResponse(response)
	if err == nil {
		t.Fatal("Expected handleErrorResponse to return error, got nil.")
	}

	expectedMsg := "unknown: some unknown error"
	if err.Error() != expectedMsg {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}

func TestHandleErrorResponse403InsufficientScopeChallenge(t *testing.T) {
	json := []byte(`{"errors":[{"code":"UNKNOWN","message":"some unknown error"}]}`)
	challenge := `Bearer realm="example.io",
error="insufficient_scope",
error_description="Insufficient permission"`
	response := &http.Response{
		Status:     "403 Forbidden",
		StatusCode: 403,
		Body:       io.NopCloser(bytes.NewReader(json)),
		Header: http.Header{
			http.CanonicalHeaderKey("WWW-Authenticate"): []string{challenge},
		},
	}
	err := handleErrorResponse(response)
	if err == nil {
		t.Fatal("Expected handleErrorResponse to return error, got nil.")
	}

	expectedMsg := "unknown: some unknown error"
	if err.Error() != expectedMsg {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}

func TestHandleErrorResponse401ValidBody(t *testing.T) {
	json := []byte("{\"errors\":[{\"code\":\"UNAUTHORIZED\",\"message\":\"action requires authentication\"}]}")
	response := &http.Response{
		Status:     "401 Unauthorized",
		StatusCode: 401,
		Body:       io.NopCloser(bytes.NewReader(json)),
	}
	err := handleErrorResponse(response)

	expectedMsg := "unauthorized: action requires authentication"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}

func TestHandleErrorResponse401WithInvalidBody(t *testing.T) {
	json := []byte("{invalid json}")
	response := &http.Response{
		Status:     "401 Unauthorized",
		StatusCode: 401,
		Body:       io.NopCloser(bytes.NewReader(json)),
	}
	err := handleErrorResponse(response)

	expectedMsg := "unauthorized: authentication required"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}

func TestHandleErrorResponseExpectedStatusCode400ValidBody(t *testing.T) {
	json := []byte("{\"errors\":[{\"code\":\"DIGEST_INVALID\",\"message\":\"provided digest does not match\"}]}")
	response := &http.Response{
		Status:     "400 Bad Request",
		StatusCode: 400,
		Body:       io.NopCloser(bytes.NewReader(json)),
	}
	err := handleErrorResponse(response)

	expectedMsg := "digest invalid: provided digest does not match"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}

func TestHandleErrorResponseExpectedStatusCode404EmptyErrorSlice(t *testing.T) {
	json := []byte(`{"randomkey": "randomvalue"}`)
	response := &http.Response{
		Status:     "404 Not Found",
		StatusCode: 404,
		Body:       io.NopCloser(bytes.NewReader(json)),
	}
	err := handleErrorResponse(response)

	expectedMsg := `error parsing HTTP 404 response body: no error details found in HTTP response body: "{\"randomkey\": \"randomvalue\"}"`
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}

func TestHandleErrorResponseExpectedStatusCode404InvalidBody(t *testing.T) {
	json := []byte("{invalid json}")
	response := &http.Response{
		Status:     "404 Not Found",
		StatusCode: 404,
		Body:       io.NopCloser(bytes.NewReader(json)),
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
		Body:       io.NopCloser(bytes.NewReader([]byte("{\"Error Encountered\" : \"Function not implemented.\"}"))),
	}
	err := handleErrorResponse(response)

	expectedMsg := "received unexpected HTTP status: 501 Not Implemented"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected \"%s\", got: \"%s\"", expectedMsg, err.Error())
	}
}
