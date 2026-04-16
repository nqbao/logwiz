// Command logsim is a log stream simulator for manual testing of logwiz.
//
// It writes configurable log lines to stdout at a controlled rate so you
// can pipe them straight into logwiz:
//
//	logsim | logwiz
//	logsim -pattern errors -rate 50 | logwiz
//	logsim -pattern web-access -lines 200 | logwiz
//
// # Patterns
//
// A pattern is a named set of line templates. One template is picked at
// random for each emitted line. Templates support the following variables:
//
//	{ts}            current timestamp (2006/01/02 15:04:05)
//	{seq}           line sequence number, starting at 0
//	{ip}            random IPv4 address
//	{rand:N-M}      random integer in [N, M]
//	{choice:a,b,c}  random pick from the comma-separated list
//	{cr}            carriage return (\r) — for testing logwiz CR handling
//
// # Custom patterns
//
// Pass -config to load extra patterns from a JSON file:
//
//	logsim -config mypatterns.json -pattern my-app
//
// The file must be a JSON array of pattern objects:
//
//	[
//	  {
//	    "name": "my-app",
//	    "templates": [
//	      "[{ts}] INFO  request handled in {rand:1-500}ms",
//	      "[{ts}] ERROR {choice:timeout,refused,reset}: upstream"
//	    ]
//	  }
//	]
//
// Custom patterns are appended after built-ins, so they can be referenced
// by -pattern. Built-in names can also be overridden by using the same name.
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

// PatternDef is a named collection of line templates.
type PatternDef struct {
	Name      string   `json:"name"`
	Templates []string `json:"templates"`
}

// builtinPatterns are the patterns shipped with logsim.
var builtinPatterns = []PatternDef{
	{
		Name: "web-access",
		Templates: []string{
			`{ip} - - [{ts}] "GET /api/v1/users HTTP/1.1" 200 {rand:100-5000}`,
			`{ip} - - [{ts}] "POST /api/v1/login HTTP/1.1" {choice:200,201,400,401} {rand:50-500}`,
			`{ip} - - [{ts}] "GET /static/app.{choice:js,css,woff2} HTTP/1.1" {choice:200,304} {rand:1000-50000}`,
			`{ip} - - [{ts}] "PUT /api/v1/items/{rand:1-999} HTTP/1.1" {choice:200,400,404} {rand:50-800}`,
			`{ip} - - [{ts}] "DELETE /api/v1/items/{rand:1-999} HTTP/1.1" {choice:200,404} {rand:30-200}`,
			`{ip} - - [{ts}] "GET /healthz HTTP/1.1" 200 4`,
		},
	},
	{
		Name: "errors",
		Templates: []string{
			`{ts} INFO  starting worker {rand:1-8}`,
			`{ts} INFO  serving on :{rand:8000-9000}`,
			`{ts} DEBUG processed {rand:1-100} items in {rand:1-50}ms`,
			`{ts} DEBUG cache {choice:hit,hit,hit,miss} for key={rand:1-9999}`,
			`{ts} WARN  queue depth {rand:80-200} exceeds threshold`,
			`{ts} WARN  high memory usage: {rand:75-99}%`,
			`{ts} ERROR {choice:connection refused,timeout,unexpected EOF,context deadline exceeded}: worker {rand:1-8}`,
			`{ts} ERROR failed to reach upstream after {rand:1-5} retries`,
			`{ts} INFO  retrying in {rand:1-30}s`,
		},
	},
	{
		Name: "json",
		Templates: []string{
			`{"time":"{ts}","level":"info","msg":"request handled","ms":{rand:1-500},"status":{choice:200,201,204},"path":"/api/v{rand:1-2}/items"}`,
			`{"time":"{ts}","level":"debug","msg":"cache {choice:hit,hit,miss}","key":"item:{rand:1-9999}","ttl":{rand:60-3600}}`,
			`{"time":"{ts}","level":"warn","msg":"slow query","ms":{rand:200-2000},"table":"{choice:users,orders,events,sessions}","rows":{rand:1-50000}}`,
			`{"time":"{ts}","level":"error","msg":"handler failed","error":"{choice:timeout,nil pointer dereference,EOF,permission denied}","path":"/api/v1/items/{rand:1-999}"}`,
			`{"time":"{ts}","level":"info","msg":"worker heartbeat","id":{rand:1-8},"queue_depth":{rand:0-200}}`,
		},
	},
	{
		Name: "spinner",
		// Lines contain {cr} to test logwiz's carriage-return handling.
		// logwiz splits on \r and keeps only the last non-empty segment,
		// so each of these lines should appear as just the final segment.
		Templates: []string{
			`building... {rand:1-33}%{cr}building... {rand:34-66}%{cr}building... {rand:67-99}%{cr}building... done`,
			`downloading packages ({rand:1-50}/{rand:51-100}){cr}downloading packages ({rand:51-99}/{rand:51-100}){cr}download complete`,
			`compiling {rand:1-300} files...{cr}compiled {rand:300-400} files in {rand:1-9}.{rand:0-9}s`,
			`[{seq}] running tests...{cr}[{seq}] {rand:1-200} passed, 0 failed`,
		},
	},
	{
		Name: "crash",
		// A mix of startup lines and a Go panic traceback. Useful for
		// testing how logwiz renders multi-line crash output.
		Templates: []string{
			`{ts} INFO  application starting`,
			`{ts} INFO  connected to db at localhost:5432`,
			`{ts} INFO  serving on :8080`,
			`{ts} DEBUG received {choice:GET,POST,PUT} /{choice:users,orders,items}`,
			`panic: runtime error: index out of range [{rand:1-5}] with length {rand:1-3}`,
			``,
			`goroutine {rand:1-20} [running]:`,
			`main.processItems(...)`,
			`	/home/user/app/main.go:{rand:20-80} +0x{rand:100-999}`,
			`main.handler(0x{rand:100-999}, 0x{rand:100-999})`,
			`	/home/user/app/handler.go:{rand:10-60} +0x{rand:50-200}`,
			`net/http.(*ServeMux).ServeHTTP(...)`,
			`	/usr/local/go/src/net/http/server.go:{rand:2000-2600}`,
			`exit status 2`,
		},
	},
}

// varRe matches template variables like {ts}, {rand:1-100}, {choice:a,b}.
// The leading [a-zA-Z] ensures plain JSON braces ({"key":...}) are not matched.
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

// loadConfig reads a JSON file containing an array of PatternDef.
func loadConfig(path string) ([]PatternDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var defs []PatternDef
	if err := json.Unmarshal(data, &defs); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	for i, d := range defs {
		if d.Name == "" {
			return nil, fmt.Errorf("pattern %d has no name", i)
		}
		if len(d.Templates) == 0 {
			return nil, fmt.Errorf("pattern %q has no templates", d.Name)
		}
	}
	return defs, nil
}

func main() {
	patternName := flag.String("pattern", "web-access", "comma-separated pattern names to mix (e.g. web-access,errors)")
	rate := flag.Float64("rate", 10, "lines per second (0 = burst as fast as possible)")
	lines := flag.Int("lines", 0, "number of lines to emit (0 = infinite)")
	configFile := flag.String("config", "", "JSON file with custom pattern definitions")
	listFlag := flag.Bool("list", false, "list available pattern names and exit")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: logsim [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Emit log lines to stdout for piping into logwiz:")
		fmt.Fprintln(os.Stderr, "  logsim | logwiz")
		fmt.Fprintln(os.Stderr, "  logsim -pattern errors -rate 50 -lines 1000 | logwiz")
		fmt.Fprintln(os.Stderr, "  logsim -pattern web-access,errors,json | logwiz")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Flags:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Template variables: {ts} {seq} {ip} {rand:N-M} {choice:a,b,c} {cr}")
	}
	flag.Parse()

	// Merge built-ins with any custom patterns from -config.
	patterns := make([]PatternDef, len(builtinPatterns))
	copy(patterns, builtinPatterns)

	if *configFile != "" {
		extra, err := loadConfig(*configFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "logsim:", err)
			os.Exit(1)
		}
		patterns = append(patterns, extra...)
	}

	if *listFlag {
		for _, p := range patterns {
			fmt.Println(p.Name)
		}
		return
	}

	// Collect templates from all requested patterns (comma-separated).
	// Templates from all named patterns are merged into one pool; a
	// random pick from that pool determines each emitted line.
	var templates []string
	for _, name := range strings.Split(*patternName, ",") {
		name = strings.TrimSpace(name)
		var found bool
		for i := range patterns {
			if patterns[i].Name == name {
				templates = append(templates, patterns[i].Templates...)
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "logsim: unknown pattern %q (use -list to see available patterns)\n", name)
			os.Exit(1)
		}
	}

	// Emit lines.
	var ticker *time.Ticker
	if *rate > 0 {
		interval := time.Duration(float64(time.Second) / *rate)
		ticker = time.NewTicker(interval)
		defer ticker.Stop()
	}

	for seq := 0; *lines == 0 || seq < *lines; seq++ {
		if ticker != nil {
			<-ticker.C
		}
		tmpl := templates[rand.Intn(len(templates))]
		fmt.Println(expand(tmpl, seq))
	}
}
