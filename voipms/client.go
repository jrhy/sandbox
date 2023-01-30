package voipms

import (
	"fmt"
	"net/http"
	"net/url"
)

type Client struct {
	apiUsername string
	apiPassword string
	baseURL     string
}

func New(username, password string) *Client {
	return &Client{
		apiUsername: username,
		apiPassword: password,
		baseURL:     fmt.Sprintf(`https://voip.ms/api/v1/rest.php?advanced=true&api_password=%s&api_username=%s`, url.QueryEscape(password), url.QueryEscape(username)),
	}
}

type RequestURL string

func (c *Client) SendSMSURL(did, dst, message string) *RequestURL {
	s := RequestURL(fmt.Sprintf(`%s&method=sendSMS&did=%s&dst=%s&message=%s`,
		c.baseURL,
		url.QueryEscape(did),
		url.QueryEscape(dst),
		url.QueryEscape(message)))
	return &s
}

func (url *RequestURL) Request() error {
	r, err := http.Get(string(*url))
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("expected HTTP 200, got %d", r.StatusCode)
	}
	return nil
}
