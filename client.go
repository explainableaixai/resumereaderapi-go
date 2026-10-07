// Package resumereaderapi is a client for the Resume Reader API: resume parsing
// into 114 structured fields plus job title, skill and location normalization.
package resumereaderapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	// Version of this module.
	Version = "1.0.0"
	baseURL = "https://www.resumereaderapi.com/api/"
)

var statusText = map[int]string{
	400: "missing or invalid input",
	401: "invalid API key",
	402: "insufficient credits, nothing was billed",
	413: "document beyond 20 pages, about 30,000 tokens or 10 MB, nothing was billed",
	422: "file could not be read as a resume",
	429: "rate limit exceeded, 30 requests per 60 seconds per IP",
}

// APIError carries the application status from the JSON body (HTTP status is always 200).
type APIError struct {
	Status  int
	Message string
	Body    map[string]any
}

func (e *APIError) Error() string {
	return fmt.Sprintf("resumereaderapi: status %d: %s", e.Status, e.Message)
}

// ParseOptions are the optional parse parameters.
type ParseOptions struct {
	FieldNames       string // "en" (default) or "fr"
	ExcludeSensitive bool
	Anonymize        bool
	MaxPages         int
	Sections         []string
	Language         string
}

// Normalized is the result of a batched normalization call.
type Normalized struct {
	Results          []map[string]any
	CreditsUsed      float64
	RemainingCredits float64
}

// Client calls the API with one key.
type Client struct {
	APIKey string
	HTTP   *http.Client
	Base   string
}

// New returns a client with a 120 second timeout.
func New(apiKey string) *Client {
	return &Client{APIKey: apiKey, HTTP: &http.Client{Timeout: 120 * time.Second}, Base: baseURL}
}

func (c *Client) post(ctx context.Context, endpoint string, payload map[string]any) (map[string]any, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "resumereaderapi-go/"+Version+" (+https://www.resumereaderapi.com)")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("resumereaderapi: response was not a JSON object: %w", err)
	}
	if s, ok := body["status"].(float64); ok && int(s) != 200 {
		msg := statusText[int(s)]
		if msg == "" {
			msg = "API error"
		}
		return nil, &APIError{Status: int(s), Message: msg, Body: body}
	}
	return body, nil
}

func (c *Client) parsePayload(source map[string]any, o ParseOptions) map[string]any {
	p := map[string]any{"api_key": c.APIKey, "schema_version": 2}
	for k, v := range source {
		p[k] = v
	}
	if o.FieldNames != "" {
		p["field_names"] = o.FieldNames
	}
	if o.ExcludeSensitive {
		p["exclude_sensitive"] = true
	}
	if o.Anonymize {
		p["anonymize"] = true
	}
	if o.MaxPages > 0 {
		p["max_pages"] = o.MaxPages
	}
	if len(o.Sections) > 0 {
		p["sections"] = o.Sections
	}
	if o.Language != "" {
		p["language"] = o.Language
	}
	return p
}

// ParseText parses resume text.
func (c *Client) ParseText(ctx context.Context, text string, o ParseOptions) (map[string]any, error) {
	return c.post(ctx, "parse.php", c.parsePayload(map[string]any{"text": text}, o))
}

// ParseFile parses a local resume file (PDF, DOCX, TXT, RTF, HTML, ODT, XLSX, images).
func (c *Client) ParseFile(ctx context.Context, path string, o ParseOptions) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src := map[string]any{"file_base64": base64.StdEncoding.EncodeToString(data), "filename": filepath.Base(path)}
	return c.post(ctx, "parse.php", c.parsePayload(src, o))
}

// ParseURL parses a resume the API fetches from an https URL.
func (c *Client) ParseURL(ctx context.Context, fileURL string, o ParseOptions) (map[string]any, error) {
	return c.post(ctx, "parse.php", c.parsePayload(map[string]any{"file_url": fileURL}, o))
}

func (c *Client) normalize(ctx context.Context, endpoint, key string, items []string) (*Normalized, error) {
	out := &Normalized{}
	for i := 0; i < len(items); i += 100 {
		end := i + 100
		if end > len(items) {
			end = len(items)
		}
		body, err := c.post(ctx, endpoint, map[string]any{"api_key": c.APIKey, key: items[i:end]})
		if err != nil {
			return nil, err
		}
		if list, ok := body["results"].([]any); ok {
			for _, r := range list {
				if m, ok := r.(map[string]any); ok {
					out.Results = append(out.Results, m)
				}
			}
		}
		if v, ok := body["credits_used"].(float64); ok {
			out.CreditsUsed += v
		}
		if v, ok := body["remaining_credits"].(float64); ok {
			out.RemainingCredits = v
		}
	}
	return out, nil
}

// NormalizeTitles standardizes raw job titles.
func (c *Client) NormalizeTitles(ctx context.Context, titles []string) (*Normalized, error) {
	return c.normalize(ctx, "normalize_title.php", "titles", titles)
}

// NormalizeSkills collapses aliases and typos into canonical skills.
func (c *Client) NormalizeSkills(ctx context.Context, skills []string) (*Normalized, error) {
	return c.normalize(ctx, "normalize_skills.php", "skills", skills)
}

// NormalizeLocations resolves raw location strings into city, region and country.
func (c *Client) NormalizeLocations(ctx context.Context, locations []string) (*Normalized, error) {
	return c.normalize(ctx, "normalize_locations.php", "locations", locations)
}
