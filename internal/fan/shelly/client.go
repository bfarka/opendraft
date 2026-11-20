package shelly

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bfarka/opendraft/internal/domain"
)

// Client controls a Shelly Dimmer 2 (Gen1 /light/0, with Gen2 /rpc/Light.Set fallback) over local HTTP.
type Client struct {
	device  domain.FanDevice
	http    *http.Client
	baseURL string
}

// NewClient creates a Shelly HTTP Client for dev.
func NewClient(dev domain.FanDevice) *Client {
	host := dev.IP
	if host == "" {
		host = dev.Host
	}
	port := dev.Port
	if port == 0 {
		port = 80
	}
	return &Client{
		device:  dev,
		http:    &http.Client{Timeout: 10 * time.Second},
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
	}
}

// Info returns metadata about the controlled Shelly device.
func (c *Client) Info() domain.FanDevice {
	return c.device
}

// Off turns the Shelly dimmer off.
func (c *Client) Off(ctx context.Context) error {
	return c.SetSpeed(ctx, 0)
}

// SetSpeed sets the Shelly dimmer brightness between 0 (off) and 100 (max).
func (c *Client) SetSpeed(ctx context.Context, percent int) error {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	var gen1Path, gen2Path string
	if percent == 0 {
		gen1Path = "/light/0?turn=off"
		gen2Path = "/rpc/Light.Set?id=0&on=false"
	} else {
		gen1Path = fmt.Sprintf("/light/0?turn=on&brightness=%d", percent)
		gen2Path = fmt.Sprintf("/rpc/Light.Set?id=0&on=true&brightness=%d", percent)
	}

	status, body, err := c.doGet(ctx, gen1Path)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		// Fall back to Shelly Gen2/Gen3 RPC API.
		status, body, err = c.doGet(ctx, gen2Path)
		if err != nil {
			return err
		}
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("shelly HTTP %d: %s", status, body)
	}
	return nil
}

func (c *Client) doGet(ctx context.Context, path string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return 0, "", fmt.Errorf("create shelly request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("send shelly command: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, string(b), nil
}
