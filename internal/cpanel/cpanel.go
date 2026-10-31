package cpanel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the cPanel UAPI DNS endpoints.
type Client struct {
	Base     string
	Username string
	APIKey   string
	HTTP     *http.Client
}

// New builds a client from separate URL + port (.env style).
func New(rawURL string, port int, username, apiKey string) (*Client, error) {
	rawURL = strings.TrimSpace(rawURL)
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	rawURL = strings.TrimRight(rawURL, "/")
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("CPANEL_URL inválido: %w", err)
	}
	if _, _, err := net.SplitHostPort(u.Host); err != nil {
		if port > 0 {
			u.Host = fmt.Sprintf("%s:%d", u.Host, port)
		}
	}
	return &Client{
		Base:     strings.TrimRight(u.String(), "/"),
		Username: username,
		APIKey:   apiKey,
		HTTP: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		},
	}, nil
}

// userAgent identifica o cliente. Alguns hosts com mod_security/WAF bloqueiam
// o default "Go-http-client"; o bash antigo passava com o UA do curl.
const userAgent = "dnsupdater/1.0"

// ZoneRecord is one row of parse_zone data.
type ZoneRecord struct {
	DnameB64   string   `json:"dname_b64"`
	DataB64    []string `json:"data_b64"`
	LineIndex  int      `json:"line_index"`
	RecordType string   `json:"record_type"`
}

type parseResp struct {
	Status int          `json:"status"`
	Data   []ZoneRecord `json:"data"`
	Errors interface{}  `json:"errors"`
}

type editResp struct {
	Status int         `json:"status"`
	Errors interface{} `json:"errors"`
}

// ParseZone returns the full zone data (source of truth, no local DNS).
func (c *Client) ParseZone(ctx context.Context, domain string) ([]ZoneRecord, error) {
	endpoint := c.Base + "/execute/DNS/parse_zone?zone=" + url.QueryEscape(domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "cpanel "+c.Username+":"+c.APIKey)
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("parse_zone: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("parse_zone HTTP %d: %s (verifique token, privilégios DNS do token e Host Access Control; se curl funciona, reporte o User-Agent %q ao suporte do host)", resp.StatusCode, trunc(string(body), 200), userAgent)
		}
		return nil, fmt.Errorf("parse_zone HTTP %d: %s", resp.StatusCode, trunc(string(body), 200))
	}
	var pr parseResp
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("parse_zone JSON inválido: %w", err)
	}
	if pr.Status == 0 {
		return nil, fmt.Errorf("parse_zone erro: %v", pr.Errors)
	}
	if pr.Data == nil {
		return nil, fmt.Errorf("parse_zone sem data")
	}
	return pr.Data, nil
}

// Found is a matched A record.
type Found struct {
	LineIndex  int
	Dname      string // decoded FQDN from zone
	RecordType string
	IP         string
}

// FindRecord matches subdomain against zone data.
// Accepts short ("homeip") or FQDN ("homeip.exemplo.pt", with/without dot).
func FindRecord(records []ZoneRecord, subdomain, domain string) (*Found, error) {
	cands := candidates(subdomain, domain)
	for _, r := range records {
		if !strings.EqualFold(r.RecordType, "A") {
			continue
		}
		name := DecodeB64(r.DnameB64)
		norm := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
		for _, cand := range cands {
			if norm == cand {
				if len(r.DataB64) == 0 {
					return nil, fmt.Errorf("registro %q sem data", name)
				}
				return &Found{
					LineIndex:  r.LineIndex,
					Dname:      strings.TrimSuffix(strings.TrimSpace(name), "."),
					RecordType: r.RecordType,
					IP:         DecodeB64(r.DataB64[0]),
				}, nil
			}
		}
	}
	return nil, errNotFound
}

var errNotFound = fmt.Errorf("registro A não encontrado")

// IsNotFound reports a no-match (caller should ADD).
func IsNotFound(err error) bool { return err == errNotFound }

// Serial extracts the SOA serial (data_b64[2] decoded).
func Serial(records []ZoneRecord) (string, error) {
	for _, r := range records {
		if strings.EqualFold(r.RecordType, "SOA") {
			if len(r.DataB64) < 3 {
				return "", fmt.Errorf("SOA sem serial")
			}
			s := DecodeB64(r.DataB64[2])
			if strings.TrimSpace(s) == "" {
				return "", fmt.Errorf("SOA serial vazio")
			}
			return strings.TrimSpace(s), nil
		}
	}
	return "", fmt.Errorf("nenhum SOA na zona")
}

// MassEdit calls add|edit with the given payload.
func (c *Client) MassEdit(ctx context.Context, domain, serial, operation string, payload map[string]interface{}) error {
	if operation != "add" && operation != "edit" {
		return fmt.Errorf("operation inválida %q", operation)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	form := url.Values{}
	form.Set("zone", domain)
	form.Set("serial", serial)
	form.Set(operation, string(raw))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.Base+"/execute/DNS/mass_edit_zone", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "cpanel "+c.Username+":"+c.APIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("mass_edit_zone: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	var er editResp
	if err := json.Unmarshal(body, &er); err != nil {
		return fmt.Errorf("mass_edit_zone resposta inválida HTTP %d: %s", resp.StatusCode, trunc(string(body), 200))
	}
	if er.Status == 0 {
		return fmt.Errorf("mass_edit_zone falhou: %v", er.Errors)
	}
	return nil
}

// EditPayload builds the JSON for operation=edit.
func EditPayload(lineIndex int, dname string, ttl int, recordType, ip string) map[string]interface{} {
	return map[string]interface{}{
		"line_index":  lineIndex,
		"dname":       dname,
		"ttl":         ttl,
		"record_type": recordType,
		"data":        []string{ip},
	}
}

// AddPayload builds the JSON for operation=add.
func AddPayload(dname string, ttl int, ip string) map[string]interface{} {
	return map[string]interface{}{
		"dname":       dname,
		"ttl":         ttl,
		"record_type": "A",
		"data":        []string{ip},
	}
}

// DecodeB64 decodes standard/base64 cPanel fields, tolerant to missing padding.
func DecodeB64(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return string(b)
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return string(b)
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return string(b)
	}
	return s
}

func candidates(subdomain, domain string) []string {
	sub := strings.ToLower(strings.Trim(strings.TrimSpace(subdomain), "."))
	dom := strings.ToLower(strings.Trim(strings.TrimSpace(domain), "."))
	set := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), "."))
		if s != "" && !set[s] {
			set[s] = true
			out = append(out, s)
		}
	}
	add(sub)
	if dom != "" {
		if sub == "@" {
			add(dom)
		} else if sub != dom && !strings.HasSuffix(sub, "."+dom) {
			add(sub + "." + dom)
		} else {
			add(sub)
		}
	}
	return out
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
