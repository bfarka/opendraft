package shelly

import (
	"net"
	"testing"

	"github.com/grandcat/zeroconf"
)

func TestParseServiceEntryShelly(t *testing.T) {
	entry := &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{
			Instance: "shellydimmer2-3C6105E12345",
		},
		HostName: "shellydimmer2-3C6105E12345.local.",
		Port:     80,
		AddrIPv4: []net.IP{net.ParseIP("192.168.1.80")},
	}

	dev, ok := parseServiceEntry(entry)
	if !ok {
		t.Fatalf("expected Shelly device to be recognized")
	}
	if dev.ID != "shellydimmer2-3C6105E12345" || dev.IP != "192.168.1.80" || dev.Port != 80 {
		t.Fatalf("unexpected device: %+v", dev)
	}
	if dev.Encrypted || dev.Model != "shelly" {
		t.Fatalf("expected unencrypted shelly model, got %+v", dev)
	}
}

func TestParseServiceEntryNonShellyIgnored(t *testing.T) {
	entry := &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{
			Instance: "MyPrinter",
		},
		HostName: "MyPrinter.local.",
		Port:     631,
	}
	if _, ok := parseServiceEntry(entry); ok {
		t.Fatalf("expected non-Shelly mDNS entry to be ignored")
	}
}
