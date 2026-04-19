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
// Pass -config to extend or override built-ins. The file uses the same JSON
// format as builtins.json (a "templates" array and a "presets" array).
//
//	logsim -config myapp.json -pattern my-app
//
// # Template variables
//
//	{ts}            current timestamp (2006/01/02 15:04:05)
//	{seq}           line sequence number, starting at 0
//	{ip}            random IPv4 address
//	{rand:N-M}      random integer in [N, M]
//	{choice:a,b,c}  random pick from the comma-separated list
//	{cr}            carriage return (\r) — for testing logwiz CR handling
//	{nl}            newline (\n) — emit multiple lines from one template
package main

import (
	_ "embed"
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

//go:embed builtins.json
var builtinsData []byte

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

var (
	tmplMap     = map[string]string{}
	presetMap   = map[string]PresetDef{}
	presetOrder []string
)

func init() {
	if err := applyConfig(builtinsData, "builtins.json"); err != nil {
		panic("logsim: invalid built-in data: " + err.Error())
	}
}

// configFile is the JSON structure accepted by both builtins.json and -config files.
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

// applyConfig parses data and merges its templates and presets into the global
// registries. Duplicate names override existing entries.
func applyConfig(data []byte, source string) error {
	var cf configFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return fmt.Errorf("parsing %s: %w", source, err)
	}
	for i, t := range cf.Templates {
		if t.Name == "" {
			return fmt.Errorf("%s: template %d missing name", source, i)
		}
		tmplMap[t.Name] = t.Template
	}
	for _, p := range cf.Presets {
		if p.Name == "" {
			return fmt.Errorf("%s: preset missing name", source)
		}
		if len(p.Templates) == 0 {
			return fmt.Errorf("%s: preset %q has no templates", source, p.Name)
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

// loadConfig reads a JSON config file and merges it via applyConfig.
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	return applyConfig(data, path)
}

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
		case inner == "nl":
			return "\n"
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
		os.Stdout.Sync()
	}
}
