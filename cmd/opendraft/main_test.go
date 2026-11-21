package main

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"syscall"
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

func TestResolveWebAddr(t *testing.T) {
	tests := []struct {
		name       string
		cfgPort    int
		webAddr    string
		webAddrSet bool
		webPort    int
		webPortSet bool
		want       string
		wantErr    bool
	}{
		{name: "config default", cfgPort: 8080, want: ":8080"},
		{name: "config custom port", cfgPort: 9090, want: ":9090"},
		{name: "config disabled", cfgPort: 0, want: ""},
		{name: "config invalid port", cfgPort: 70000, wantErr: true},
		{name: "web-port overrides config", cfgPort: 8080, webPort: 9191, webPortSet: true, want: ":9191"},
		{name: "web-port zero disables", cfgPort: 8080, webPort: 0, webPortSet: true, want: ""},
		{name: "web-port invalid", cfgPort: 8080, webPort: -1, webPortSet: true, wantErr: true},
		{name: "web-addr overrides config", cfgPort: 8080, webAddr: "127.0.0.1:9292", webAddrSet: true, want: "127.0.0.1:9292"},
		{name: "web-addr empty disables", cfgPort: 8080, webAddr: "", webAddrSet: true, want: ""},
		{name: "web-addr and web-port combine host and port", cfgPort: 8080, webAddr: "127.0.0.1:8080", webAddrSet: true, webPort: 9393, webPortSet: true, want: "127.0.0.1:9393"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveWebAddr(tc.cfgPort, tc.webAddr, tc.webAddrSet, tc.webPort, tc.webPortSet)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (addr=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunFailsWhenWebUIPortInUse(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen temp port: %v", err)
	}
	defer ln.Close()

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	addr := ln.Addr().String()

	for _, args := range [][]string{
		{"--config", cfgPath, "--mock", "--web-addr", addr},
		{"--config", cfgPath, "--mock", "--headless", "--web-addr", addr},
	} {
		err := run(args)
		if err == nil {
			t.Fatalf("run(%v) expected error when port %s is already in use", args, addr)
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			t.Fatalf("run(%v) expected EADDRINUSE, got: %v", args, err)
		}
		if !strings.Contains(err.Error(), "port already in use") {
			t.Fatalf("run(%v) expected 'port already in use' message, got: %v", args, err)
		}
	}
}
