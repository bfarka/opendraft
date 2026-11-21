package main

import (
	"context"
	"testing"
)

func TestMockAdapters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hr := &mockHRSource{}
	hrCh, err := hr.Scan(ctx)
	if err != nil {
		t.Fatalf("mockHRSource.Scan error: %v", err)
	}
	dev := <-hrCh
	if dev.ID != "mock-tickr" {
		t.Fatalf("unexpected mock HR device: %+v", dev)
	}

	fan := &mockFanDiscoverer{}
	fanCh, err := fan.Scan(ctx)
	if err != nil {
		t.Fatalf("mockFanDiscoverer.Scan error: %v", err)
	}
	fdev := <-fanCh
	if fdev.ID != "shellydimmer2-mock01" {
		t.Fatalf("unexpected mock fan device: %+v", fdev)
	}
}

func TestFormatWebURLWithIP(t *testing.T) {
	tests := []struct {
		name string
		addr string
		ip   string
		want string
	}{
		{name: "wildcard ipv6 resolves LAN IP", addr: "[::]:8080", ip: "192.168.1.42", want: "http://192.168.1.42:8080"},
		{name: "wildcard ipv4 resolves LAN IP", addr: "0.0.0.0:8080", ip: "10.0.0.15", want: "http://10.0.0.15:8080"},
		{name: "explicit host preserved", addr: "192.168.1.99:9090", ip: "10.0.0.15", want: "http://192.168.1.99:9090"},
		{name: "fallback to loopback if no LAN IP", addr: ":8080", ip: "", want: "http://127.0.0.1:8080"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatWebURLWithIP(tc.addr, func() string { return tc.ip })
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
