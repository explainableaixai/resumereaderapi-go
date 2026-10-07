# resumereaderapi-go

Parse resumes from Go. One package, standard library only, results as plain maps.

```bash
go get github.com/explainableaixai/resumereaderapi-go
```

## Minimal program

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	rr "github.com/explainableaixai/resumereaderapi-go"
)

func main() {
	c := rr.New(os.Getenv("RESUME_KEY"))
	out, err := c.ParseFile(context.Background(), "cv.pdf", rr.ParseOptions{})
	if err != nil {
		log.Fatal(err)
	}
	resume := out["resume"].(map[string]any)
	fmt.Println(resume["contact"].(map[string]any)["full_name"])
}
```

## Exported surface

```go
func New(apiKey string) *Client
func (c *Client) ParseText(ctx, text string, o ParseOptions) (map[string]any, error)
func (c *Client) ParseFile(ctx, path string, o ParseOptions) (map[string]any, error)
func (c *Client) ParseURL(ctx, url string, o ParseOptions) (map[string]any, error)
func (c *Client) NormalizeTitles(ctx, []string) (*Normalized, error)
func (c *Client) NormalizeSkills(ctx, []string) (*Normalized, error)
func (c *Client) NormalizeLocations(ctx, []string) (*Normalized, error)
type APIError struct { Status int; Message string; Body map[string]any }
```

`ParseOptions` fields: `FieldNames`, `ExcludeSensitive`, `Anonymize`, `MaxPages`, `Sections`, `Language`.

## Agencies and job boards

Recruiters receive resumes in every format. A small Go service can accept uploads, call `ParseFile`, and write rows to a database. Agencies that process hundreds of candidates a day use exactly this loop in [recruitment agency](https://www.resumereaderapi.com/use-cases/recruitment-agencies.php) workflows.

Marketplaces go one step further and normalize titles so search works. The [job board](https://www.resumereaderapi.com/use-cases/job-boards.php) page shows how normalized titles improve matching.

## HTTP handler example

```go
func handler(c *rr.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		file, hdr, err := r.FormFile("cv")
		if err != nil {
			http.Error(w, "missing file", 400)
			return
		}
		defer file.Close()
		tmp, _ := os.CreateTemp("", "cv-*"+filepath.Ext(hdr.Filename))
		defer os.Remove(tmp.Name())
		io.Copy(tmp, file)
		tmp.Close()

		out, err := c.ParseFile(r.Context(), tmp.Name(), rr.ParseOptions{})
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		json.NewEncoder(w).Encode(out["resume"])
	}
}
```

## Rate limit aware worker

The service allows 30 requests per 60 seconds per IP. A ticker keeps you under it.

```go
tick := time.NewTicker(2100 * time.Millisecond)
defer tick.Stop()
for _, p := range paths {
	<-tick.C
	if _, err := c.ParseFile(ctx, p, rr.ParseOptions{}); err != nil {
		log.Println(p, err)
	}
}
```

## Errors

```go
var ae *rr.APIError
if errors.As(err, &ae) {
	switch ae.Status {
	case 402:
		log.Println("no credits")
	case 413, 422:
		log.Println("document problem:", ae.Message)
	}
}
```

## Normalizers in one example

```go
res, err := c.NormalizeTitles(ctx, []string{"Sr. SWE II", "VP Sales EMEA"})
if err != nil {
	log.Fatal(err)
}
for _, r := range res.Results {
	fmt.Println(r["input"], "=>", r["normalized_title"], r["seniority_level"])
}
fmt.Printf("%.1f credits\n", res.CreditsUsed)
```

## Tests

`Client.Base` can point to an `httptest.Server`, so unit tests never hit the network.

## FAQ

**Which Go version?** 1.20 or later.

**How big can a file be?** 10 MB.

**Pricing?** See the plans at the site. MIT license. info@alpha-quantum.com
