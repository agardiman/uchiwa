package sensu

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"

	"github.com/sensu/uchiwa/uchiwa/logger"
	"io/ioutil"
)

// ...
func (api *API) doRequest(req *http.Request) ([]byte, *http.Response, error) {
	if api.User != "" && api.Pass != "" {
		req.SetBasicAuth(api.User, api.Pass)
	}

	req.Close = api.CloseRequest

	if api.Tracing {
		trace := &httptrace.ClientTrace{
			ConnectStart: func(network, addr string) {
				logger.Customf("httptrace", "Dial started for request %s: %s %s", req.URL, network, addr)
			},
			ConnectDone: func(network, addr string, err error) {
				if err == nil {
					err = fmt.Errorf("nil")
				}
				logger.Customf("httptrace", "Dial done with error=%s", err.Error())
			},
			GotConn: func(connInfo httptrace.GotConnInfo) {
				logger.Customf("httptrace", "Successful connection details: %+v", connInfo)
			},
			GotFirstResponseByte: func() {
				logger.Custom("httptrace", "Got first response byte for the request")
			},
			TLSHandshakeDone: func(connectionState tls.ConnectionState, err error) {
				if err == nil {
					err = fmt.Errorf("nil")
				}
				logger.Customf("httptrace", "TLS handshake done with error=%s", err)
			},
		}
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	}

	res, err := api.Client.Do(req)
	if err != nil {
		status, ok := err.(*url.Error)
		if !ok {
			return nil, nil, fmt.Errorf("Unexpected error, got %T, wanted *url.Error", err)
		}
		return nil, nil, status.Err
	}

	defer res.Body.Close()

	if api.Tracing {
		logger.Customf("httptrace", "Length of response body: %d bytes", res.ContentLength)
	}

	if res.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("%v", res.Status)
	}

	var body []byte
	if res.ContentLength < 0 {
		// manage situations where the Content-Length is not present because the
		// package has been chuncked or compressed by proxies between Uchiwa and the Sensu API
		body, err = ioutil.ReadAll(res.Body)
		if err != nil {
			if api.Tracing {
				logger.Infof("Got %s", string(body))
			}
			return nil, nil, fmt.Errorf("error reading body when Content-Length is %d: %s", res.ContentLength, err)
		}
	} else {
		body = make([]byte, res.ContentLength)
		n, err := io.ReadFull(res.Body, body)
		if err != nil {
			if err == io.ErrUnexpectedEOF {
				logger.Warningf("Tried to read %d bytes, got %d", res.ContentLength, n)
				if api.Tracing {
					logger.Infof("Got %s", string(body[0:n]))
				}
			}
			return nil, nil, fmt.Errorf("Parsing response body returned: %v", err)
		}
	}

	if api.Tracing {
		logger.Customf("httptrace", "Closing connection")
	}

	return body, res, nil
}
