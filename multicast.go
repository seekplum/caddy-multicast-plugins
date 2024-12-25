// https://www.flysnow.org/2021/09/21/caddy-in-action-extending-caddy
// go build -o multicast.so -buildmode=c-shared multicast.go

package multicast

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

const MaxIdleConns int = 10000
const MaxIdleConnections int = 10000
const RequestTimeout int = 10

func createHTTPClient() *http.Client {
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        MaxIdleConns,
			MaxIdleConnsPerHost: MaxIdleConnections,
			DisableCompression:  true,
		},
		Timeout: time.Duration(RequestTimeout) * time.Second,
	}
	return client
}

var httpClient *http.Client

func init() {
	caddy.RegisterModule(Multicast{})
	httpcaddyfile.RegisterHandlerDirective("multicast", parseCaddyfile)
	httpClient = createHTTPClient()
}

// CaddyModule returns the Caddy module information
func (m Multicast) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.multicast",
		New: func() caddy.Module { return new(Multicast) },
	}
}

type ModeEnum string

const (
	ANY ModeEnum = "any"
	ALL ModeEnum = "all"
)

// Multicast is the module definition
type Multicast struct {
	Backends []string `json:"backends,omitempty"`
	Mode     ModeEnum `json:"mode,omitempty"`
	logger   *zap.Logger
}

type DataResponse struct {
	err  error
	body []byte
}

func IsInvalidMode(mode string) error {
	if mode == string(ANY) || mode == string(ALL) {
		return nil
	}
	return fmt.Errorf("invalid mode: %s. mode must be %s or %s. ", mode, ANY, ALL)
}

// Provision sets up the module.
func (m *Multicast) Provision(ctx caddy.Context) error {
	m.logger = ctx.Logger()
	if m.Mode == "" {
		m.Mode = ANY
	}
	return nil
}

// Validate ensures the configuration is valid
func (m *Multicast) Validate() error {
	if len(m.Backends) == 0 {
		return fmt.Errorf("at least one backend is required. ")
	}
	if err := IsInvalidMode(string(m.Mode)); err != nil {
		return err
	}
	return nil
}

func (m *Multicast) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		if d.NextArg() {
			return d.ArgErr()
		}

		for d.NextBlock(0) {
			switch d.Val() {
			case "backends":
				m.Backends = append(m.Backends, d.RemainingArgs()...)
			case "mode":
				if !d.NextArg() {
					return d.ArgErr()
				}
				mode := ModeEnum(d.Val())
				if err := IsInvalidMode(string(mode)); err != nil {
					return err
				}
				m.Mode = mode
				if d.NextArg() {
					return d.ArgErr()
				}
			default:
				return d.ArgErr()
			}
		}
	}
	return nil
}

// parseCaddyfile parses the Caddyfile directive
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	m := &Multicast{}
	err := m.UnmarshalCaddyfile(h.Dispenser)
	if err != nil {
		return nil, err
	}
	return m, err
}

func executeRequest(method string, url string, headers http.Header, body []byte) ([]byte, error) {
	// Create a new request to the backend
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("%s: %w. ", "Error creating request", err)
	}
	// Copy headers
	for k, v := range headers {
		req.Header[k] = v
	}

	// Send the request
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w. ", "Error sending request", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %d. ", "Error forwarding request", resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w. ", "Error reading response body", err)
	}
	return respBody, nil
}

func GenUriByRequest(r *http.Request) string {
	uri := r.URL.Path
	if r.URL.RawQuery != "" {
		uri += "?" + r.URL.RawQuery
	}
	return uri
}

func GenBodyByRequest(r *http.Request) ([]byte, error) {
	var body []byte
	var err error
	if r.Body != nil {
		body, err = io.ReadAll(r.Body)
		if err != nil {
			return body, err
		}
	}
	return body, nil
}

func GenHeadersByRequest(r *http.Request) http.Header {
	headers := http.Header{}
	for k, v := range r.Header {
		if strings.ToLower(k) == "content-length" {
			continue
		}
		headers[k] = v
	}
	return headers
}

// ServeHTTP handles the HTTP request
func (m Multicast) ServeHTTP(w http.ResponseWriter, r *http.Request, _ caddyhttp.Handler) error {
	body, err := GenBodyByRequest(r)
	if err != nil {
		return err
	}
	uri := GenUriByRequest(r)
	headers := GenHeadersByRequest(r)

	ch := make(chan DataResponse, len(m.Backends))
	for _, backend := range m.Backends {
		go func(b string) {
			respBody, err := executeRequest(r.Method, b+uri, headers, body)
			ch <- DataResponse{err: err, body: respBody}
		}(backend)
	}
	dataResponse := DataResponse{}
	success := 0
	for i := 0; i < len(m.Backends); i++ {
		tmpResp := <-ch
		if tmpResp.err == nil {
			dataResponse = tmpResp
			success += 1
		}
	}
	close(ch)
	m.logger.Info(fmt.Sprintf("Multicast request %s %s %s %d/%d", r.Method, r.URL.Path, m.Mode, success, len(m.Backends)))
	// Respond immediately to the client
	if success > 0 && m.Mode == ANY || success == len(m.Backends) && m.Mode == ALL {
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(dataResponse.body)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, err = w.Write([]byte(`{"code": 503, "message": "Service Unavailable"}`))
	}
	return err
}

// Interface guards
var (
	_ caddy.Provisioner           = (*Multicast)(nil)
	_ caddy.Validator             = (*Multicast)(nil)
	_ caddyhttp.MiddlewareHandler = (*Multicast)(nil)
	_ caddyfile.Unmarshaler       = (*Multicast)(nil)
)
