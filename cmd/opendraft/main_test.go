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
