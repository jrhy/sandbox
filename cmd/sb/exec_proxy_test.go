//go:build darwin

package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"syscall"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// proxyTestResolver supplies A records over an in-memory DNS connection.
// No request reaches the system's DNS servers. A nil answer means NXDOMAIN.
func proxyTestResolver(t *testing.T, addresses []string) *net.Resolver {
	t.Helper()
	return &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			client, server := net.Pipe()
			done := make(chan struct{})
			t.Cleanup(func() { client.Close(); server.Close(); <-done })
			go func() {
				defer close(done)
				defer server.Close()
				const dnsTimeout = 5 * time.Second
				if err := server.SetDeadline(time.Now().Add(dnsTimeout)); err != nil {
					t.Error(err)
					return
				}
				// Resolver.Dial uses TCP framing for connections without PacketConn.
				var size [2]byte
				if _, err := io.ReadFull(server, size[:]); err != nil {
					t.Error(err)
					return
				}
				wire := make([]byte, binary.BigEndian.Uint16(size[:]))
				if _, err := io.ReadFull(server, wire); err != nil {
					t.Error(err)
					return
				}
				var query dnsmessage.Message
				if err := query.Unpack(wire); err != nil {
					t.Error(err)
					return
				}
				response := dnsmessage.Message{
					Header:    dnsmessage.Header{ID: query.ID, Response: true, RecursionAvailable: true},
					Questions: query.Questions,
				}
				if addresses == nil {
					response.RCode = dnsmessage.RCodeNameError
				}
				for _, q := range query.Questions {
					if q.Type != dnsmessage.TypeA {
						continue
					}
					for _, address := range addresses {
						ip := net.ParseIP(address).To4()
						if ip == nil {
							t.Errorf("invalid fixture IPv4 address %q", address)
							return
						}
						response.Answers = append(response.Answers, dnsmessage.Resource{
							Header: dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET},
							Body:   &dnsmessage.AResource{A: [4]byte(ip)},
						})
					}
				}
				wire, err := response.Pack()
				if err != nil {
					t.Error(err)
					return
				}
				binary.BigEndian.PutUint16(size[:], uint16(len(wire)))
				if _, err := server.Write(append(size[:], wire...)); err != nil {
					t.Error(err)
				}
			}()
			return client, nil
		},
	}
}

func TestExecShort_ProxyDialerValidatesEveryResolvedAddress(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		addresses []string
		allowed   []string
	}{
		{"public", []string{"203.0.113.5"}, []string{"203.0.113.5:8443"}},
		{"subnet", []string{"192.168.1.10"}, []string{"192.168.1.10:8443"}},
		{"tailnet", []string{"100.64.0.1"}, []string{"100.64.0.1:8443"}},
		{"loopback", []string{"127.0.0.1"}, nil},
		{"mixed", []string{"127.0.0.1", "192.168.1.10"}, []string{"192.168.1.10:8443"}},
		{"fallback", []string{"203.0.113.5", "203.0.113.6"}, []string{"203.0.113.5:8443", "203.0.113.6:8443"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dialer := buildProxyDialer()
			dialer.Resolver = proxyTestResolver(t, tc.addresses)
			check := dialer.Control
			var checked, allowed []string
			stopBeforeConnect := errors.New("test connection failure")
			dialer.Control = func(network, address string, conn syscall.RawConn) error {
				checked = append(checked, address)
				if err := check(network, address, conn); err != nil {
					return err
				}
				allowed = append(allowed, address)
				// Force fallback without connecting to any external address. Each
				// candidate must pass the real policy hook, using the resolved IP.
				return stopBeforeConnect
			}
			_, err := dialer.DialContext(context.Background(), "tcp4", "upstream.test.:8443")
			if err == nil {
				t.Fatal("expected dial failure")
			}
			wantChecked := make([]string, len(tc.addresses))
			for i, ip := range tc.addresses {
				wantChecked[i] = net.JoinHostPort(ip, "8443")
			}
			// The resolver may reorder answers according to local routing rules.
			sort.Strings(checked)
			sort.Strings(wantChecked)
			sort.Strings(allowed)
			if !reflect.DeepEqual(checked, wantChecked) {
				t.Errorf("checked %v, want %v", checked, wantChecked)
			}
			if !reflect.DeepEqual(allowed, tc.allowed) {
				t.Errorf("allowed %v, want %v", allowed, tc.allowed)
			}
		})
	}
}

func TestExecShort_HTTPAllowProxyDNSFailure(t *testing.T) {
	t.Parallel()
	for _, method := range []string{http.MethodGet, http.MethodConnect} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			dialer := buildProxyDialer()
			dialer.Resolver = proxyTestResolver(t, nil)
			transport := &http.Transport{DialContext: dialer.DialContext}
			defer transport.CloseIdleConnections()
			proxy := &allowlistHTTPProxy{patterns: []string{"upstream.test"}, dialer: dialer, transport: transport}
			req := httptest.NewRequest(method, "http://upstream.test.:8443/demo", nil)
			req.Host = "upstream.test.:8443"
			response := httptest.NewRecorder()
			proxy.ServeHTTP(response, req)
			if response.Code != http.StatusBadGateway {
				t.Fatalf("DNS failure: status=%d body=%q", response.Code, response.Body.String())
			}
		})
	}
}

// Exercise successful HTTP forwarding and a real TLS handshake through CONNECT
// without relying on a usable LAN interface. Address policy is tested separately;
// this fixture's dialer permits its loopback backend but still uses real DNS and
// connection handling. The subnet integration test uses the production policy.
func TestExecShort_HTTPAllowProxyForwards(t *testing.T) {
	t.Parallel()
	for _, scheme := range []string{"http", "https"} {
		t.Run(scheme, func(t *testing.T) {
			t.Parallel()
			backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				fmt.Fprintf(w, "%s%s", req.Host, req.URL.RequestURI())
			}))
			if scheme == "https" {
				backend.StartTLS()
			} else {
				backend.Start()
			}
			defer backend.Close()
			_, port, err := net.SplitHostPort(backend.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			target := net.JoinHostPort("upstream.test.", port)
			dialed := make(chan string, 2)
			const testTimeout = 5 * time.Second
			dialer := &net.Dialer{
				Timeout:  testTimeout,
				Resolver: proxyTestResolver(t, []string{"127.0.0.1"}),
				Control: func(_, address string, _ syscall.RawConn) error {
					dialed <- address
					return nil
				},
			}
			upstreamTransport := &http.Transport{DialContext: dialer.DialContext}
			defer upstreamTransport.CloseIdleConnections()
			proxy := httptest.NewServer(&allowlistHTTPProxy{patterns: []string{"upstream.test"}, dialer: dialer, transport: upstreamTransport})
			defer proxy.Close()
			proxyURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Fatal(err)
			}
			transport := &http.Transport{
				Proxy:           http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // local test certificate
			}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: testTimeout}
			resp, err := client.Get(scheme + "://" + target + "/forwarded?x=1")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusOK || string(body) != target+"/forwarded?x=1" {
				t.Fatalf("status=%d body=%q", resp.StatusCode, body)
			}
			select {
			case address := <-dialed:
				if want := net.JoinHostPort("127.0.0.1", port); address != want {
					t.Fatalf("dialed %q, want resolved address %q", address, want)
				}
			default:
				t.Fatal("upstream was not dialed")
			}
		})
	}
}
