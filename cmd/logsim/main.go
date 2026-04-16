// Command logsim is a log stream simulator for manual testing of logwiz.
//
// It writes configurable log lines to stdout at a controlled rate so you
// can pipe them straight into logwiz:
//
//	logsim | logwiz
//	logsim -pattern errors -rate 50 | logwiz
//	logsim -pattern web-access,errors -lines 500 | logwiz
//
// # Concepts
//
// A template is a named line format containing optional variables (see below).
// A preset is a named weighted distribution over templates — the weights make
// the output look like real logs (e.g. mostly 200s, rare 500s).
//
// When multiple presets are mixed with a comma-separated -pattern, each preset
// contributes equal total probability regardless of how many templates it has.
// So "web-access,errors" always produces a 50/50 split between the two
// distributions.
//
// # Custom templates and presets
//
// Pass -config to extend or override built-ins:
//
//	logsim -config myapp.json -pattern my-app
//
// Config file format (JSON):
//
//	{
//	  "templates": [
//	    {"name": "ok",  "template": "[{ts}] INFO  OK in {rand:1-500}ms"},
//	    {"name": "err", "template": "[{ts}] ERROR {choice:timeout,reset}"}
//	  ],
//	  "presets": [
//	    {
//	      "name": "my-app",
//	      "templates": [
//	        {"name": "ok",  "weight": 90},
//	        {"name": "err", "weight": 10}
//	      ]
//	    }
//	  ]
//	}
//
// Custom templates/presets are merged with built-ins. Duplicate names override
// the built-in. Use -list to see all available preset names.
//
// # Template variables
//
//	{ts}            current timestamp (2006/01/02 15:04:05)
//	{seq}           line sequence number, starting at 0
//	{ip}            random IPv4 address
//	{rand:N-M}      random integer in [N, M]
//	{choice:a,b,c}  random pick from the comma-separated list
//	{cr}            carriage return (\r) — for testing logwiz CR handling
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WeightedRef pairs a template name with a relative sampling weight.
type WeightedRef struct {
	Name   string
	Weight float64
}

// PresetDef is a named weighted distribution over templates.
type PresetDef struct {
	Name      string
	Templates []WeightedRef
}

// entry is a resolved (template string, normalised weight) pair used for sampling.
type entry struct {
	tmpl   string
	weight float64
}

// tmplMap is the template registry: name → raw template string.
// Populated from built-ins; -config files extend or override it.
var tmplMap = map[string]string{
	// --- web-access ---
	"web-get-ok":      `{ip} - - [{ts}] "GET /api/v1/{choice:users,orders,items,events} HTTP/1.1" 200 {rand:100-8000}`,
	"web-get-cached":  `{ip} - - [{ts}] "GET /static/{choice:app.js,app.css,logo.png,font.woff2} HTTP/1.1" 304 0`,
	"web-get-static":  `{ip} - - [{ts}] "GET /static/{choice:app.js,app.css,logo.png} HTTP/1.1" 200 {rand:5000-80000}`,
	"web-post-ok":     `{ip} - - [{ts}] "POST /api/v1/{choice:users,orders,sessions} HTTP/1.1" {choice:200,201} {rand:200-1000}`,
	"web-post-unauth": `{ip} - - [{ts}] "POST /api/v1/sessions HTTP/1.1" 401 45`,
	"web-post-bad":    `{ip} - - [{ts}] "POST /api/v1/{choice:users,orders} HTTP/1.1" 400 {rand:50-300}`,
	"web-notfound":    `{ip} - - [{ts}] "GET /{choice:admin,wp-login,phpmyadmin,.env} HTTP/1.1" 404 12`,
	"web-error":       `{ip} - - [{ts}] "GET /api/v1/{choice:users,orders} HTTP/1.1" 500 {rand:100-500}`,
	"web-healthz":     `127.0.0.1 - - [{ts}] "GET /healthz HTTP/1.1" 200 4`,

	// --- errors (structured leveled log) ---
	"log-info-complete":  `{ts} INFO  processed batch of {rand:10-200} items in {rand:10-500}ms`,
	"log-info-retry":     `{ts} INFO  retrying in {rand:1-30}s (attempt {rand:1-5}/5)`,
	"log-info-connected": `{ts} INFO  connected to {choice:db,cache,queue} at localhost:{rand:5432-6380}`,
	"log-debug-cache":    `{ts} DEBUG cache {choice:hit,hit,hit,miss} key={rand:1-9999}`,
	"log-debug-query":    `{ts} DEBUG query took {rand:1-80}ms rows={rand:0-5000}`,
	"log-warn-queue":     `{ts} WARN  queue depth {rand:80-200} exceeds threshold`,
	"log-warn-memory":    `{ts} WARN  high memory usage: {rand:75-95}%`,
	"log-error-conn":     `{ts} ERROR {choice:connection refused,timeout,unexpected EOF,context deadline exceeded}: worker {rand:1-8}`,
	"log-error-upstream": `{ts} ERROR failed to reach upstream after {rand:2-5} retries`,

	// --- json (structured JSON logs) ---
	"json-request":   `{"time":"{ts}","level":"info","msg":"request","ms":{rand:1-500},"status":{choice:200,200,200,201,204},"path":"/api/v1/{choice:users,orders,items}"}`,
	"json-cache":     `{"time":"{ts}","level":"debug","msg":"cache {choice:hit,hit,miss}","key":"{rand:1-9999}","ttl":{rand:60-3600}}`,
	"json-slowquery": `{"time":"{ts}","level":"warn","msg":"slow query","ms":{rand:200-2000},"table":"{choice:users,orders,events}"}`,
	"json-error":     `{"time":"{ts}","level":"error","msg":"handler failed","error":"{choice:timeout,nil pointer dereference,EOF}","path":"/api/v1/items/{rand:1-999}"}`,
	"json-worker":    `{"time":"{ts}","level":"info","msg":"worker heartbeat","id":{rand:1-8},"queue":{rand:0-200}}`,

	// --- spinner (lines with {cr} to stress logwiz CR handling) ---
	"spin-build":    `building... {rand:1-33}%{cr}building... {rand:34-66}%{cr}building... {rand:67-99}%{cr}building... done`,
	"spin-download": `downloading ({rand:1-50}/{rand:51-100}){cr}downloading ({rand:51-99}/{rand:51-100}){cr}download complete`,
	"spin-compile":  `compiling {rand:100-300} files...{cr}compiled in {rand:1-9}.{rand:0-9}s`,

	// --- crash (Go panic traceback fragments) ---
	"crash-startup":   `{ts} INFO  application started, serving on :8080`,
	"crash-request":   `{ts} DEBUG received {choice:GET,POST,PUT} /{choice:users,orders,items}`,
	"crash-panic":     `panic: runtime error: {choice:index out of range,nil pointer dereference,interface conversion failed}`,
	"crash-goroutine": `goroutine {rand:1-20} [running]:`,
	"crash-frame-fn":  `main.{choice:handler,processItems,run,serve}(...)`,
	"crash-frame-loc": "\t/app/{choice:main,handler,server}.go:{rand:20-120} +0x{rand:100-999}",
	"crash-stdlib":    `net/http.(*ServeMux).ServeHTTP(...)`,
	"crash-exit":      `exit status 2`,
}

// presetMap is the preset registry: name → PresetDef.
var presetMap = map[string]PresetDef{
	// Weights reflect real-world traffic distributions.
	"web-access": {Name: "web-access", Templates: []WeightedRef{
		{"web-get-ok", 60},    // most requests succeed
		{"web-get-cached", 15}, // many assets are cached
		{"web-get-static", 10}, // uncached asset loads
		{"web-post-ok", 7},    // form submissions
		{"web-post-unauth", 3}, // auth failures
		{"web-post-bad", 2},   // malformed requests
		{"web-notfound", 2},   // 404s (scanners, typos)
		{"web-error", 0.5},    // server errors are rare
		{"web-healthz", 0.5},  // health checks trickle in
	}},
	// Weights reflect typical app log distributions: mostly debug/info, few errors.
	"errors": {Name: "errors", Templates: []WeightedRef{
		{"log-info-complete", 20},
		{"log-info-retry", 5},
		{"log-info-connected", 5},
		{"log-debug-cache", 25},
		{"log-debug-query", 20},
		{"log-warn-queue", 10},
		{"log-warn-memory", 5},
		{"log-error-conn", 7},
		{"log-error-upstream", 3},
	}},
	"json": {Name: "json", Templates: []WeightedRef{
		{"json-request", 60},
		{"json-cache", 20},
		{"json-slowquery", 10},
		{"json-error", 5},
		{"json-worker", 5},
	}},
	"spinner": {Name: "spinner", Templates: []WeightedRef{
		{"spin-build", 33},
		{"spin-download", 33},
		{"spin-compile", 34},
	}},
	"crash": {Name: "crash", Templates: []WeightedRef{
		{"crash-startup", 20},
		{"crash-request", 20},
		{"crash-panic", 8},
		{"crash-goroutine", 7},
		{"crash-frame-fn", 15},
		{"crash-frame-loc", 15},
		{"crash-stdlib", 10},
		{"crash-exit", 5},
	}},
}

// presetOrder controls the display order of -list output.
var presetOrder = []string{"web-access", "errors", "json", "spinner", "crash"}

// varRe matches template variables. The leading [a-zA-Z] prevents matching
// plain JSON object braces ({"key":...}) which would otherwise be confused
// for variables.
var varRe = regexp.MustCompile(`\{[a-zA-Z][^}]*\}`)

// expand replaces all template variables in tmpl with generated values.
func expand(tmpl string, seq int) string {
	now := time.Now()
	return varRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		inner := m[1 : len(m)-1] // strip braces
		switch {
		case inner == "ts":
			return now.Format("2006/01/02 15:04:05")
		case inner == "seq":
			return strconv.Itoa(seq)
		case inner == "ip":
			return fmt.Sprintf("%d.%d.%d.%d",
				rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(256))
		case inner == "cr":
			return "\r"
		case strings.HasPrefix(inner, "rand:"):
			parts := strings.SplitN(inner[5:], "-", 2)
			if len(parts) != 2 {
				return m
			}
			lo, errLo := strconv.Atoi(parts[0])
			hi, errHi := strconv.Atoi(parts[1])
			if errLo != nil || errHi != nil || hi < lo {
				return m
			}
			return strconv.Itoa(lo + rand.Intn(hi-lo+1))
		case strings.HasPrefix(inner, "choice:"):
			choices := strings.Split(inner[7:], ",")
			return choices[rand.Intn(len(choices))]
		default:
			return m // unknown variable — leave as-is
		}
	})
}

// configFile is the JSON structure accepted by -config.
type configFile struct {
	Templates []struct {
		Name     string `json:"name"`
		Template string `json:"template"`
	} `json:"templates"`
	Presets []struct {
		Name      string `json:"name"`
		Templates []struct {
			Name   string  `json:"name"`
			Weight float64 `json:"weight"`
		} `json:"templates"`
	} `json:"presets"`
}

// loadConfig reads a JSON config file and merges its templates and presets
// into the global registries. Duplicate names override built-ins.
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	var cf configFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	for i, t := range cf.Templates {
		if t.Name == "" {
			return fmt.Errorf("%s: template %d missing name", path, i)
		}
		tmplMap[t.Name] = t.Template
	}
	for _, p := range cf.Presets {
		if p.Name == "" {
			return fmt.Errorf("%s: preset missing name", path)
		}
		if len(p.Templates) == 0 {
			return fmt.Errorf("%s: preset %q has no templates", path, p.Name)
		}
		refs := make([]WeightedRef, len(p.Templates))
		for i, t := range p.Templates {
			refs[i] = WeightedRef{Name: t.Name, Weight: t.Weight}
		}
		if _, exists := presetMap[p.Name]; !exists {
			presetOrder = append(presetOrder, p.Name)
		}
		presetMap[p.Name] = PresetDef{Name: p.Name, Templates: refs}
	}
	return nil
}

// buildEntries resolves the named presets into a flat weighted sampling table.
// Each preset contributes equal total probability (1/n) regardless of its
// template count, so mixing presets always gives an even split between them.
func buildEntries(names []string) ([]entry, error) {
	n := float64(len(names))
	var entries []entry
	for _, name := range names {
		preset, ok := presetMap[name]
		if !ok {
			return nil, fmt.Errorf("unknown preset %q (use -list to see available)", name)
		}
		var sum float64
		for _, ref := range preset.Templates {
			sum += ref.Weight
		}
		if sum == 0 {
			return nil, fmt.Errorf("preset %q has zero total weight", name)
		}
		for _, ref := range preset.Templates {
			tmpl, ok := tmplMap[ref.Name]
			if !ok {
				return nil, fmt.Errorf("preset %q references unknown template %q", preset.Name, ref.Name)
			}
			// Normalise: this template's share = (its weight / preset total) / num presets.
			entries = append(entries, entry{
				tmpl:   tmpl,
				weight: (ref.Weight / sum) / n,
			})
		}
	}
	return entries, nil
}

// pickEntry does a weighted random pick from the sampling table.
func pickEntry(entries []entry) string {
	r := rand.Float64()
	var cum float64
	for _, e := range entries {
		cum += e.weight
		if r <= cum {
			return e.tmpl
		}
	}
	return entries[len(entries)-1].tmpl // floating-point safety net
}

func main() {
	patternName := flag.String("pattern", "web-access", "comma-separated preset names to mix (e.g. web-access,errors)")
	rate := flag.Float64("rate", 10, "lines per second (0 = burst as fast as possible)")
	lines := flag.Int("lines", 0, "number of lines to emit (0 = infinite)")
	configPath := flag.String("config", "", "JSON file with custom templates and presets")
	listFlag := flag.Bool("list", false, "list available preset names and exit")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: logsim [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Emit log lines to stdout for piping into logwiz:")
		fmt.Fprintln(os.Stderr, "  logsim | logwiz")
		fmt.Fprintln(os.Stderr, "  logsim -pattern errors -rate 50 -lines 1000 | logwiz")
		fmt.Fprintln(os.Stderr, "  logsim -pattern web-access,errors | logwiz")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Flags:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Template variables: {ts} {seq} {ip} {rand:N-M} {choice:a,b,c} {cr}")
	}
	flag.Parse()

	if *configPath != "" {
		if err := loadConfig(*configPath); err != nil {
			fmt.Fprintln(os.Stderr, "logsim:", err)
			os.Exit(1)
		}
	}

	if *listFlag {
		for _, name := range presetOrder {
			fmt.Println(name)
		}
		return
	}

	names := strings.Split(*patternName, ",")
	for i, n := range names {
		names[i] = strings.TrimSpace(n)
	}

	entries, err := buildEntries(names)
	if err != nil {
		fmt.Fprintln(os.Stderr, "logsim:", err)
		os.Exit(1)
	}

	var ticker *time.Ticker
	if *rate > 0 {
		ticker = time.NewTicker(time.Duration(float64(time.Second) / *rate))
		defer ticker.Stop()
	}

	for seq := 0; *lines == 0 || seq < *lines; seq++ {
		if ticker != nil {
			<-ticker.C
		}
		fmt.Println(expand(pickEntry(entries), seq))
	}
}
