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

<!--expanded-->
## Notes for Go developers

The module follows a few conventions that Go programmers will recognise. Every call takes a `context.Context`. Errors are values, and the typed one, `*APIError`, works with `errors.As`. Results are `map[string]any` so the schema can grow without breaking your build. There are no dependencies outside the standard library.

The one convention that surprises people is where errors live. The service answers every request with HTTP 200 and puts the real outcome in a `status` field in the body. If you write your own client you must remember to check it. This module does that for you and returns an error whenever the status is not 200.

## Typed access to the parts you use

Decode into structs for the fields you store, and leave the rest as a map:

```go
type Resume struct {
	Contact struct {
		FullName string   `json:"full_name"`
		Emails   []string `json:"emails"`
		Address  struct {
			City    *string `json:"city"`
			Country *string `json:"country"`
		} `json:"address"`
	} `json:"contact"`
	Career struct {
		Seniority *string  `json:"seniority_level"`
		Years     *float64 `json:"total_experience_years"`
	} `json:"career"`
	Skills struct {
		Technical []string `json:"technical"`
	} `json:"skills"`
}

func decode(out map[string]any) (Resume, error) {
	raw, err := json.Marshal(out["resume"])
	if err != nil {
		return Resume{}, err
	}
	var r Resume
	return r, json.Unmarshal(raw, &r)
}
```

Pointers model the fields that may be `null`, and slices model the arrays that may be empty. Because every key is always present, the decode never fails on missing fields.

## Batch processing with limits

The rate limit is 30 requests per 60 seconds per IP, which works out to one request every two seconds. A ticker is the simplest way to respect it, and an `errgroup` or a small worker pool keeps the code tidy. For a single machine, one worker with a ticker is often enough, because each parse takes a few seconds anyway.

If you run from several machines behind one IP, share the limit. A token bucket in Redis, or a queue with a fixed consumption rate, keeps the total below the ceiling. Do not rely on every worker being polite on its own.

## Storing results

A parsed resume is a JSON document, and a `jsonb` column is a natural home. Keep three things together: the raw response, the options you used, and the time of the call. Add generated columns or indexes for the fields you query most, such as `seniority_level`, `total_experience_years` and the country. That gives you fast filters without losing the full record.

Think about retention from the first day. Resumes contain personal data. Decide how long you keep them, how a candidate asks for deletion, and how that request reaches the parsed copy. A parser that retains nothing helps, since the only copy is yours, and you control its life.

## Using normalization to clean a column

Normalizers are the quiet workhorses of a candidate database. A single run over your distinct titles, skills and locations gives you canonical values you can index and group. Treat the original strings as input and the canonical ones as the thing you query.

```go
distinct := distinctSkills(db) // []string from your store
res, err := c.NormalizeSkills(ctx, distinct)
if err != nil {
	return err
}
for _, r := range res.Results {
	skill, _ := r["normalized_skill"].(string)
	kind, _ := r["skill_type"].(string)
	method, _ := r["method"].(string)
	saveSkill(r["input"].(string), skill, kind, method)
}
```

The `method` field says whether a deterministic rule or the fallback produced the answer. The published tests for the service report that the large majority of items resolve through the deterministic path, and every result carries a confidence number. Use both when you decide which rows deserve a human glance.

## Operating the service you build

A parsing service has the usual operational duties. Expose counters for parses, errors by status, normalizations and credits used. Set an alert when the 402 count rises, because it means credits are running out. Set another on the 429 count, because it means your pacing is off. Keep a dashboard of the share of resumes with an empty email, a rough but effective signal of extraction trouble.

For deployments, keep the key in a secret and the base URL in configuration. The client exposes `Base` so that tests and staging environments can point somewhere else.

## Further context

The published report of the [service's live test run](https://www.resumereaderapi.com/quality-testing.php) lists the categories it covers, including every file type, scanned documents and each error path. It is worth reading before you write your own tests, because it tells you which behaviours are already verified at the source.

Advertisers and publishers often meet this kind of data in the same company. The page about [audience data for SSPs](https://www.cookielessaudiences.com/industries/ssps.php) shows how a sell side platform can label inventory with coded audience attributes, and the guide to [water treatment target screening](https://www.acquisitionuniverse.com/industries/water-treatment.php) shows how a vertical is screened for acquisition candidates.

<!--extra-->
## Operational checklist

Before production, confirm that the key comes from a secret store, that every call has a context deadline, that you pace requests to stay under 30 per 60 seconds per IP, that you count each error status separately, and that your retry function never retries 400, 401, 402, 413 or 422. Add a dashboard panel for remaining credits, and alert when the number falls below a threshold. Keep parsed JSON and original files under one retention rule, and test the deletion path as carefully as the parse path. A deletion job that has never been run is a deletion job that does not work.

Finally, write a small runbook. It should say what to do when credits run out, when the service returns unexpected statuses, when a batch stalls and when a candidate asks for their data to be removed. A one page runbook is read at three in the morning by someone who has never seen the code, so write it for that person.

<!--further-->
## Further reading

The [official Go site](https://go.dev/) holds the language and library documentation, including `net/http`, `context` and `encoding/json`. For compliance, the [European Commission data protection pages](https://commission.europa.eu/law/law-topic/data-protection_en) summarise the principles that apply when you store candidate data, which matter as soon as the parsed JSON leaves memory and lands in a database.

## Questions

**Why maps and not structs?** The schema has 114 fields and may grow. Maps keep the module stable, and you can decode the parts you care about.

**Can I send several files at once?** Send them in separate calls and pace them. One call parses one document.

**Are keys scoped?** A key is tied to your account and plan. Treat it as a secret.

**Which Go version?** 1.20 or later.

## FAQ

**Which Go version?** 1.20 or later.

**How big can a file be?** 10 MB.

**Pricing?** See the plans at the site. MIT license. info@alpha-quantum.com
