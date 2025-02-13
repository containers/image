package httpdump

import (
	"net/http"
	"net/http/httputil"

	"github.com/sirupsen/logrus"
)

// DoRequest (c, req) is the same as c.Do(req), but it may log the
// request and response at logrus trace level.
func DoRequest(c *http.Client, req *http.Request) (*http.Response, error) {
	if logrus.IsLevelEnabled(logrus.TraceLevel) {
		if dro, err := httputil.DumpRequestOut(req, false); err != nil {
			logrus.Tracef("===Can not log HTTP request: %v", err)
		} else {
			logrus.Tracef("===REQ===\n%s\n===REQ===\n", dro)
		}
	}
	res, err := c.Do(req)
	if logrus.IsLevelEnabled(logrus.TraceLevel) {
		if err != nil {
			logrus.Tracef("===RES error: %v", err)
		} else if dr, err := httputil.DumpResponse(res, false); err != nil {
			logrus.Tracef("===Can not log HTTP response: %v", err)
		} else {
			logrus.Tracef("===RES===\n%s\n===RES===\n", dr)
		}
	}
	return res, err
}
