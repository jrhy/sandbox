package ollama

import (
	"io"
	"net/http"
	"strings"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newTestClient(fn roundTripFunc) *Client {
	httpClient := &http.Client{Transport: fn}
	return NewClient("http://ollama.test", httpClient)
}

func jsonResponse(statusCode int, body string) *http.Response {
	response := &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	response.Header.Set("Content-Type", "application/json")
	return response
}
