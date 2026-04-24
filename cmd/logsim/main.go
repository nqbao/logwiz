package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var varRe = regexp.MustCompile(`\{[a-zA-Z][^}]*\}`)

func main() {
	scenarioName := flag.String("scenario", "webapp", "scenario name (app or app-scenario, e.g. webapp, webapp-incident, nginx)")
	rate := flag.Float64("rate", 10, "lines per second for static scenarios (0 = burst)")
	lines := flag.Int("lines", 0, "total lines to emit (0 = infinite)")
	configPath := flag.String("config", "", "JSON config file to extend/override builtins")
	listFlag := flag.Bool("list", false, "list available scenarios and exit")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: logsim [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Emit log lines to stdout for piping into logwiz:")
		fmt.Fprintln(os.Stderr, "  logsim | logwiz")
		fmt.Fprintln(os.Stderr, "  logsim -scenario webapp-incident | logwiz")
		fmt.Fprintln(os.Stderr, "  logsim -scenario nginx -rate 50 -lines 1000 | logwiz")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Flags:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Template variables: {ts} {seq} {ip} {rand:N-M} {choice:a,b,c} {cr}")
	}
	flag.Parse()

	reg, err := loadRegistry(builtinsData, "builtins.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, "logsim:", err)
		os.Exit(1)
	}

	if *configPath != "" {
		reg2, err := loadRegistryFile(*configPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "logsim:", err)
			os.Exit(1)
		}
		reg = mergeRegistries(reg, reg2)
	}

	if *listFlag {
		for _, e := range reg.listScenarios() {
			fmt.Println(e.FullName)
		}
		return
	}

	app, scen, err := reg.resolveScenario(*scenarioName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "logsim:", err)
		os.Exit(1)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	seq := 0
	maxLines := *lines

	for _, ph := range scen.Phases {
		entries, err := buildEntries(reg, app.Name, ph.Templates)
		if err != nil {
			fmt.Fprintln(os.Stderr, "logsim:", err)
			os.Exit(1)
		}

		phRate := ph.Rate
		if phRate == 0 {
			phRate = *rate
		}
		if phRate <= 0 {
			phRate = 1
		}

		fmt.Printf("──── %s", ph.Name)
		if ph.Duration > 0 {
			fmt.Printf(" (%s)", ph.Duration)
		} else {
			fmt.Print(" (∞)")
		}
		fmt.Println(" ────")
		os.Stdout.Sync()

		var ticker *time.Ticker
		if phRate > 0 {
			ticker = time.NewTicker(time.Duration(float64(time.Second) / phRate))
			defer ticker.Stop()
		}

		phStart := time.Now()
		phLineCount := 0

		for {
			if maxLines > 0 && seq >= maxLines {
				return
			}
			if ph.Duration > 0 && time.Since(phStart) >= ph.Duration {
				break
			}

			if ticker != nil {
				<-ticker.C
			}

			td := pickEntry(rng, entries)
			emitted := emitTemplate(td, seq)
			seq += emitted
			phLineCount += emitted

			if ph.Duration == 0 && maxLines == 0 {
				// infinite phase, infinite lines — just keep going
			}
		}
	}
}

func emitTemplate(td templateDef, seq int) int {
	if len(td.Lines) > 0 {
		for _, line := range td.Lines {
			fmt.Println(expand(line, seq))
			seq++
		}
		os.Stdout.Sync()
		return len(td.Lines)
	}
	fmt.Println(expand(td.Line, seq))
	os.Stdout.Sync()
	return 1
}

type entry struct {
	td     templateDef
	weight float64
}

func buildEntries(reg *registry, appName string, refs []weightedRef) ([]entry, error) {
	var entries []entry
	for _, ref := range refs {
		td, ok := reg.lookupTemplate(appName, ref.Name)
		if !ok {
			return nil, fmt.Errorf("unknown template %q", ref.Name)
		}
		entries = append(entries, entry{td: td, weight: ref.Weight})
	}
	return entries, nil
}

func pickEntry(rng *rand.Rand, entries []entry) templateDef {
	var total float64
	for _, e := range entries {
		total += e.weight
	}
	r := rng.Float64() * total
	var cum float64
	for _, e := range entries {
		cum += e.weight
		if r <= cum {
			return e.td
		}
	}
	return entries[len(entries)-1].td
}

func expand(tmpl string, seq int) string {
	now := time.Now()
	return varRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		inner := m[1 : len(m)-1]
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
			return expandRand(inner[5:])
		case strings.HasPrefix(inner, "choice:"):
			return expandChoice(inner[7:])
		default:
			return m
		}
	})
}

func expandRand(s string) string {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return "{" + s + "}"
	}
	lo, errLo := strconv.Atoi(parts[0])
	hi, errHi := strconv.Atoi(parts[1])
	if errLo != nil || errHi != nil || hi < lo {
		return "{rand:" + s + "}"
	}
	return strconv.Itoa(lo + rand.Intn(hi-lo+1))
}

func expandChoice(s string) string {
	choices := strings.Split(s, ",")
	return choices[rand.Intn(len(choices))]
}

func mergeRegistries(base, override *registry) *registry {
	for k, v := range override.commonTemplates {
		base.commonTemplates[k] = v
	}
	for k, v := range override.apps {
		base.apps[k] = v
	}
	return base
}
