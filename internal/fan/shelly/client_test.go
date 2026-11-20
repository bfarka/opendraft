package shelly

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/bfarka/opendraft/internal/domain"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestClientGen1SetSpeedAndOff(t *testing.T) {
	var gotURLs []string

	c := NewClient(domain.FanDevice{ID: "shellydimmer2-1", IP: "192.168.1.80", Port: 80, Model: "shelly"})
	c.http = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotURLs = append(gotURLs, r.URL.RequestURI())
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"ison":true,"brightness":55}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	if err := c.SetSpeed(context.Background(), 55); err != nil {
		t.Fatalf("SetSpeed(55) failed: %v", err)
	}
	if err := c.Off(context.Background()); err != nil {
		t.Fatalf("Off() failed: %v", err)
	}

	if len(gotURLs) != 2 || gotURLs[0] != "/light/0?turn=on&brightness=55" || gotURLs[1] != "/light/0?turn=off" {
		t.Fatalf("unexpected URLs: %v", gotURLs)
	}
}

func TestClientGen2Fallback(t *testing.T) {
	var gotURLs []string

	c := NewClient(domain.FanDevice{ID: "shellyplusdimmer-1", IP: "192.168.1.81", Port: 80, Model: "shelly"})
	c.http = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotURLs = append(gotURLs, r.URL.RequestURI())
			if r.URL.Path == "/light/0" {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(bytes.NewBufferString(`Not Found`)),
					Header:     make(http.Header),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	if err := c.SetSpeed(context.Background(), 80); err != nil {
		t.Fatalf("SetSpeed(80) failed: %v", err)
	}
	if len(gotURLs) != 2 || gotURLs[1] != "/rpc/Light.Set?id=0&on=true&brightness=80" {
		t.Fatalf("expected Gen2 RPC fallback, got %v", gotURLs)
	}
}
