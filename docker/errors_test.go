package docker

import (
	"bufio"
	"bytes"
	"errors"
	"net/http"
	"testing"

	"github.com/docker/distribution/registry/api/errcode"
	v2 "github.com/docker/distribution/registry/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: This test records expected text strings, but NEITHER the returned error types
// NOR the error texts are an API commitment subject to API stability expectations;
// they can change at any time for any reason.
func TestRegistryHTTPResponseToError(t *testing.T) {
	var unwrappedUnexpectedHTTPResponseError *unexpectedHTTPResponseError
	var unwrappedErrcodeError errcode.Error
	for _, c := range []struct {
		name              string
		response          string
		errorString       string
		errorType         any                           // A value of the same type as the expected error, or nil
		unwrappedErrorPtr any                           // A pointer to a value expected to be reachable using errors.As, or nil
		errorCode         *errcode.ErrorCode            // A matching ErrorCode, or nil
		fn                func(t *testing.T, err error) // A more specialized test, or nil
	}{
		{
			name: "HTTP status out of registry error range",
			response: "HTTP/1.1 333 HTTP status out of range\r\n" +
				"Header1: Value1\r\n" +
				"\r\n" +
				"Body of the request\r\n",
			errorString: "received unexpected HTTP status: 333 HTTP status out of range",
			errorType:   &unexpectedHTTPStatusError{},
		},
		{
			name: "HTTP body not in expected format",
			response: "HTTP/1.1 400 I don't like this request\r\n" +
				"Header1: Value1\r\n" +
				"\r\n" +
				"<html><body>JSON? What JSON?</body></html>\r\n",
			errorString:       `StatusCode: 400, "<html><body>JSON? What JSON?</body></html>\r\n"`,
			errorType:         nil,
			unwrappedErrorPtr: &unwrappedUnexpectedHTTPResponseError,
		},
		{
			name: "401 body not in expected format",
			response: "HTTP/1.1 401 I don't like this request\r\n" +
				"Header1: Value1\r\n" +
				"\r\n" +
				"<html><body>JSON? What JSON?</body></html>\r\n",
			errorString:       "authentication required",
			errorType:         nil,
			unwrappedErrorPtr: &unwrappedErrcodeError,
			errorCode:         &errcode.ErrorCodeUnauthorized,
		},
		{ // docker.io when an image is not found
			name: "GET https://registry-1.docker.io/v2/library/this-does-not-exist/manifests/latest",
			response: "HTTP/1.1 401 Unauthorized\r\n" +
				"Connection: close\r\n" +
				"Content-Length: 170\r\n" +
				"Content-Type: application/json\r\n" +
				"Date: Thu, 12 Aug 2021 20:11:01 GMT\r\n" +
				"Docker-Distribution-Api-Version: registry/2.0\r\n" +
				"Strict-Transport-Security: max-age=31536000\r\n" +
				"Www-Authenticate: Bearer realm=\"https://auth.docker.io/token\",service=\"registry.docker.io\",scope=\"repository:library/this-does-not-exist:pull\",error=\"insufficient_scope\"\r\n" +
				"\r\n" +
				"{\"errors\":[{\"code\":\"UNAUTHORIZED\",\"message\":\"authentication required\",\"detail\":[{\"Type\":\"repository\",\"Class\":\"\",\"Name\":\"library/this-does-not-exist\",\"Action\":\"pull\"}]}]}\n",
			errorString:       "requested access to the resource is denied",
			errorType:         nil,
			unwrappedErrorPtr: &unwrappedErrcodeError,
			errorCode:         &errcode.ErrorCodeDenied,
		},
		{ // docker.io when a tag is not found
			name: "GET https://registry-1.docker.io/v2/library/busybox/manifests/this-does-not-exist",
			response: "HTTP/1.1 404 Not Found\r\n" +
				"Connection: close\r\n" +
				"Content-Length: 109\r\n" +
				"Content-Type: application/json\r\n" +
				"Date: Thu, 12 Aug 2021 20:51:32 GMT\r\n" +
				"Docker-Distribution-Api-Version: registry/2.0\r\n" +
				"Ratelimit-Limit: 100;w=21600\r\n" +
				"Ratelimit-Remaining: 100;w=21600\r\n" +
				"Strict-Transport-Security: max-age=31536000\r\n" +
				"\r\n" +
				"{\"errors\":[{\"code\":\"MANIFEST_UNKNOWN\",\"message\":\"manifest unknown\",\"detail\":{\"Tag\":\"this-does-not-exist\"}}]}\n",
			errorString:       "manifest unknown",
			errorType:         nil,
			unwrappedErrorPtr: &unwrappedErrcodeError,
			errorCode:         &v2.ErrorCodeManifestUnknown,
		},
		{ // public.ecr.aws does not implement tag list
			name: "GET https://public.ecr.aws/v2/nginx/nginx/tags/list",
			response: "HTTP/1.1 404 Not Found\r\n" +
				"Connection: close\r\n" +
				"Content-Length: 65\r\n" +
				"Content-Type: application/json; charset=utf-8\r\n" +
				"Date: Tue, 06 Sep 2022 21:19:02 GMT\r\n" +
				"Docker-Distribution-Api-Version: registry/2.0\r\n" +
				"\r\n" +
				"{\"errors\":[{\"code\":\"NOT_FOUND\",\"message\":\"404 page not found\"}]}\r\n",
			errorString:       "unknown: 404 page not found",
			errorType:         nil,
			unwrappedErrorPtr: &unwrappedErrcodeError,
			errorCode:         &errcode.ErrorCodeUnknown,
			fn: func(t *testing.T, err error) {
				var e errcode.Error
				ok := errors.As(err, &e)
				require.True(t, ok)
				// Note: (skopeo inspect) is checking for this errcode.Error value
				assert.Equal(t, errcode.Error{
					Code:    errcode.ErrorCodeUnknown, // The NOT_FOUND value is not defined, and turns into Unknown
					Message: "404 page not found",
					Detail:  nil,
				}, e)
			},
		},
		{ // registry.redhat.io is not compliant, variant 1: invalid "code" value
			name: "registry.redhat.io/v2/this-does-not-exist/manifests/latest",
			response: "HTTP/1.1 404 Not Found\r\n" +
				"Connection: close\r\n" +
				"Content-Length: 53\r\n" +
				"Cache-Control: max-age=0, no-cache, no-store\r\n" +
				"Content-Type: application/json\r\n" +
				"Date: Thu, 13 Oct 2022 18:15:15 GMT\r\n" +
				"Expires: Thu, 13 Oct 2022 18:15:15 GMT\r\n" +
				"Pragma: no-cache\r\n" +
				"Server: Apache\r\n" +
				"Strict-Transport-Security: max-age=63072000; includeSubdomains; preload\r\n" +
				"X-Hostname: crane-tbr06.cran-001.prod.iad2.dc.redhat.com\r\n" +
				"\r\n" +
				"{\"errors\": [{\"code\": \"404\", \"message\": \"Not Found\"}]}\r\n",
			errorString:       "unknown: Not Found",
			errorType:         errcode.Error{},
			unwrappedErrorPtr: &unwrappedErrcodeError,
			errorCode:         &errcode.ErrorCodeUnknown,
			fn: func(t *testing.T, err error) {
				var e errcode.Error
				ok := errors.As(err, &e)
				require.True(t, ok)
				// isManifestUnknownError is checking for this
				assert.Equal(t, errcode.Error{
					Code:    errcode.ErrorCodeUnknown, // The 404 value is not defined, and turns into Unknown
					Message: "Not Found",
					Detail:  nil,
				}, e)
			},
		},
		{ // registry.redhat.io is not compliant, variant 2: a completely out-of-protocol response
			name: "registry.redhat.io/v2/rhosp15-rhel8/openstack-cron/manifests/sha256-8df5e60c42668706ac108b59c559b9187fa2de7e4e262e2967e3e9da35d5a8d7.sig",
			response: "HTTP/1.1 404 Not Found\r\n" +
				"Connection: close\r\n" +
				"Content-Length: 10\r\n" +
				"Accept-Ranges: bytes\r\n" +
				"Date: Thu, 13 Oct 2022 18:13:53 GMT\r\n" +
				"Server: AkamaiNetStorage\r\n" +
				"X-Docker-Size: -1\r\n" +
				"\r\n" +
				"Not found\r\n",
			errorString:       `StatusCode: 404, "Not found\r"`,
			errorType:         nil,
			unwrappedErrorPtr: &unwrappedUnexpectedHTTPResponseError,
			fn: func(t *testing.T, err error) {
				var e *unexpectedHTTPResponseError
				ok := errors.As(err, &e)
				require.True(t, ok)
				// isManifestUnknownError is checking for this
				assert.Equal(t, 404, e.StatusCode)
				assert.Equal(t, []byte("Not found\r"), e.Response)
			},
		},
	} {
		res, err := http.ReadResponse(bufio.NewReader(bytes.NewReader([]byte(c.response))), nil)
		require.NoError(t, err, c.name)
		defer res.Body.Close()

		err = registryHTTPResponseToError(res)
		assert.Equal(t, c.errorString, err.Error(), c.name)
		if c.errorType != nil {
			assert.IsType(t, c.errorType, err, c.name)
		}
		if c.unwrappedErrorPtr != nil {
			found := errors.As(err, c.unwrappedErrorPtr)
			assert.True(t, found, c.name)
		}
		if c.errorCode != nil {
			var ec errcode.ErrorCoder
			ok := errors.As(err, &ec)
			require.True(t, ok, c.name)
			assert.Equal(t, *c.errorCode, ec.ErrorCode(), c.name)
		}
		if c.fn != nil {
			c.fn(t, err)
		}
	}
}
