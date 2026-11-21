package shelly

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
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

func TestIsDimmerEntry(t *testing.T) {
	tests := []struct {
		name  string
		entry *zeroconf.ServiceEntry
		want  bool
	}{
		{
			name: "Gen1 Dimmer 2",
			entry: &zeroconf.ServiceEntry{
				ServiceRecord: zeroconf.ServiceRecord{Instance: "shellydimmer2-3C6105E12345"},
				HostName:      "shellydimmer2-3C6105E12345.local.",
			},
			want: true,
		},
		{
			name: "Gen1 Dimmer 1",
			entry: &zeroconf.ServiceEntry{
				ServiceRecord: zeroconf.ServiceRecord{Instance: "shellydimmer-AABBCC112233"},
				HostName:      "shellydimmer-AABBCC112233.local.",
			},
			want: true,
		},
		{
			name: "Gen2 Plus Wall Dimmer via TXT app",
			entry: &zeroconf.ServiceEntry{
				ServiceRecord: zeroconf.ServiceRecord{Instance: "shellypluswdus-AABBCC"},
				HostName:      "shellypluswdus-AABBCC.local.",
				Text:          []string{"gen=2", "app=PlusWallDimmer"},
			},
			want: true,
		},
		{
			name: "Gen3 0-10V Dimmer via TXT app",
			entry: &zeroconf.ServiceEntry{
				ServiceRecord: zeroconf.ServiceRecord{Instance: "shelly0110dimg3-AABBCC"},
				HostName:      "shelly0110dimg3-AABBCC.local.",
				Text:          []string{"gen=3", "app=Dimmer010VPMG3"},
			},
			want: true,
		},
		{
			name: "Gen1 Shelly 1 switch ignored",
			entry: &zeroconf.ServiceEntry{
				ServiceRecord: zeroconf.ServiceRecord{Instance: "shelly1-AABBCC112233"},
				HostName:      "shelly1-AABBCC112233.local.",
			},
			want: false,
		},
		{
			name: "Gen1 Shelly Plug S ignored",
			entry: &zeroconf.ServiceEntry{
				ServiceRecord: zeroconf.ServiceRecord{Instance: "shellyplug-s-112233"},
				HostName:      "shellyplug-s-112233.local.",
			},
			want: false,
		},
		{
			name: "Gen2 Plus 1PM via TXT app ignored",
			entry: &zeroconf.ServiceEntry{
				ServiceRecord: zeroconf.ServiceRecord{Instance: "shellyplus1pm-AABBCC"},
				HostName:      "shellyplus1pm-AABBCC.local.",
				Text:          []string{"gen=2", "app=Plus1PM"},
			},
			want: false,
		},
		{
			name: "Gen1 Shelly RGBW2 ignored",
			entry: &zeroconf.ServiceEntry{
				ServiceRecord: zeroconf.ServiceRecord{Instance: "shellyrgbw2-AABBCC"},
				HostName:      "shellyrgbw2-AABBCC.local.",
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDimmerEntry(tc.entry); got != tc.want {
				t.Fatalf("isDimmerEntry() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsDimmerInfo(t *testing.T) {
	tests := []struct {
		name       string
		info       shellyInfo
		wantKnown  bool
		wantDimmer bool
	}{
		{name: "Gen1 SHDM-1", info: shellyInfo{Type: "SHDM-1"}, wantKnown: true, wantDimmer: true},
		{name: "Gen1 SHDM-2", info: shellyInfo{Type: "SHDM-2"}, wantKnown: true, wantDimmer: true},
		{name: "Gen1 SHSW-1", info: shellyInfo{Type: "SHSW-1"}, wantKnown: true, wantDimmer: false},
		{name: "Gen1 SHPLG-S", info: shellyInfo{Type: "SHPLG-S"}, wantKnown: true, wantDimmer: false},
		{name: "Gen1 SHBDUO-1", info: shellyInfo{Type: "SHBDUO-1"}, wantKnown: true, wantDimmer: false},
		{name: "Gen2 PlusWallDimmer", info: shellyInfo{App: "PlusWallDimmer", Gen: 2}, wantKnown: true, wantDimmer: true},
		{name: "Gen2 Plus010V", info: shellyInfo{App: "Plus010V", Gen: 2}, wantKnown: true, wantDimmer: true},
		{name: "Gen3 DimmerG3", info: shellyInfo{App: "DimmerG3", Gen: 3}, wantKnown: true, wantDimmer: true},
		{name: "Gen3 Dimmer010VPMG3", info: shellyInfo{App: "Dimmer010VPMG3", Gen: 3}, wantKnown: true, wantDimmer: true},
		{name: "Gen2 ProDimmer2PM", info: shellyInfo{App: "ProDimmer2PM", Gen: 2}, wantKnown: true, wantDimmer: true},
		{name: "Gen2 Plus1PM", info: shellyInfo{App: "Plus1PM", Gen: 2}, wantKnown: true, wantDimmer: false},
		{name: "Gen3 PlugSG3", info: shellyInfo{App: "PlugSG3", Gen: 3}, wantKnown: true, wantDimmer: false},
		{name: "Model prefix SNWD", info: shellyInfo{Model: "SNWD-00112EU"}, wantKnown: true, wantDimmer: true},
		{name: "Empty info unknown", info: shellyInfo{}, wantKnown: false, wantDimmer: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			known, dimmer := isDimmerInfo(tc.info)
			if known != tc.wantKnown || dimmer != tc.wantDimmer {
				t.Fatalf("isDimmerInfo(%+v) = (%v, %v), want (%v, %v)", tc.info, known, dimmer, tc.wantKnown, tc.wantDimmer)
			}
		})
	}
}

func TestVerifyDimmerHTTP(t *testing.T) {
	tests := []struct {
		name          string
		instance      string
		txt           []string
		responseJSON  string
		statusCode    int
		unreachable   bool
		wantAccepted  bool
		wantEncrypted bool
	}{
		{
			name:         "Gen1 dimmer accepted",
			instance:     "shellydimmer2-123456",
			responseJSON: `{"type":"SHDM-2","auth":false}`,
			statusCode:   http.StatusOK,
			wantAccepted: true,
		},
		{
			name:         "Renamed Gen1 dimmer accepted via HTTP /shelly",
			instance:     "shelly-fan-office",
			responseJSON: `{"type":"SHDM-2","auth":false}`,
			statusCode:   http.StatusOK,
			wantAccepted: true,
		},
		{
			name:         "Gen1 switch rejected via HTTP /shelly",
			instance:     "shelly1-AABBCC",
			responseJSON: `{"type":"SHSW-1","auth":false}`,
			statusCode:   http.StatusOK,
			wantAccepted: false,
		},
		{
			name:          "Gen2 dimmer with auth_en accepted and marked encrypted",
			instance:      "shellypluswdus-AABBCC",
			txt:           []string{"gen=2", "app=PlusWallDimmer"},
			responseJSON:  `{"app":"PlusWallDimmer","gen":2,"auth_en":true}`,
			statusCode:    http.StatusOK,
			wantAccepted:  true,
			wantEncrypted: true,
		},
		{
			name:         "Gen2 non-dimmer rejected via TXT without HTTP",
			instance:     "shellyplus1pm-AABBCC",
			txt:          []string{"gen=2", "app=Plus1PM"},
			responseJSON: `{"app":"Plus1PM","gen":2}`,
			statusCode:   http.StatusOK,
			wantAccepted: false,
		},
		{
			name:         "Unreachable dimmer falls back to mDNS name",
			instance:     "shellydimmer2-offline",
			unreachable:  true,
			wantAccepted: true,
		},
		{
			name:         "Unreachable non-dimmer falls back to mDNS name and is rejected",
			instance:     "shellyplug-s-offline",
			unreachable:  true,
			wantAccepted: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := &zeroconf.ServiceEntry{
				ServiceRecord: zeroconf.ServiceRecord{
					Instance: tc.instance,
				},
				HostName: tc.instance + ".local.",
				Port:     80,
				AddrIPv4: []net.IP{net.ParseIP("192.168.1.80")},
				Text:     tc.txt,
			}
			dev, ok := parseServiceEntry(entry)
			if !ok {
				t.Fatalf("parseServiceEntry failed for %s", tc.instance)
			}

			d := NewDiscoverer()
			d.httpClient = &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					if tc.unreachable {
						return nil, errors.New("connection refused")
					}
					if r.URL.Path != "/shelly" {
						return &http.Response{
							StatusCode: http.StatusNotFound,
							Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
							Header:     make(http.Header),
						}, nil
					}
					return &http.Response{
						StatusCode: tc.statusCode,
						Body:       io.NopCloser(bytes.NewBufferString(tc.responseJSON)),
						Header:     make(http.Header),
					}, nil
				}),
			}

			got, accepted := d.verifyDimmer(context.Background(), entry, dev)
			if accepted != tc.wantAccepted {
				t.Fatalf("verifyDimmer() accepted = %v, want %v", accepted, tc.wantAccepted)
			}
			if accepted && got.Encrypted != tc.wantEncrypted {
				t.Fatalf("verifyDimmer() Encrypted = %v, want %v", got.Encrypted, tc.wantEncrypted)
			}
		})
	}
}
