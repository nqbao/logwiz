package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

//go:embed scenarios/*.json
var scenariosFS embed.FS

func loadEmbeddedRegistry() (*registry, error) {
	entries, err := scenariosFS.ReadDir("scenarios")
	if err != nil {
		return nil, fmt.Errorf("reading embedded scenarios: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var reg *registry
	for _, name := range names {
		path := "scenarios/" + name
		data, err := scenariosFS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		next, err := loadRegistry(data, name)
		if err != nil {
			return nil, err
		}
		if reg == nil {
			reg = next
		} else {
			reg = mergeRegistries(reg, next)
		}
	}
	if reg == nil {
		return nil, fmt.Errorf("no embedded scenario files found")
	}
	return reg, nil
}

type rawConfig struct {
	Common struct {
		Templates []rawTemplate `json:"templates"`
	} `json:"common"`
	Apps map[string]rawApp `json:"apps"`
}

type rawTemplate struct {
	Name     string   `json:"name"`
	Template string   `json:"template,omitempty"`
	Lines    []string `json:"lines,omitempty"`
}

type rawApp struct {
	Templates []rawTemplate              `json:"templates"`
	Scenarios map[string]rawScenarioDef  `json:"scenarios"`
}

type rawScenarioDef struct {
	Templates []rawWeightedRef `json:"templates,omitempty"`
	Phases    []rawPhase       `json:"phases,omitempty"`
}

type rawWeightedRef struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
}

type rawPhase struct {
	Name      string          `json:"name"`
	Duration  string          `json:"duration"`
	Rate      float64         `json:"rate"`
	Templates []rawWeightedRef `json:"templates"`
}

type templateDef struct {
	Name  string
	Line  string
	Lines []string
}

type weightedRef struct {
	Name   string
	Weight float64
}

type phase struct {
	Name      string
	Duration  time.Duration
	Rate      float64
	Templates []weightedRef
}

type scenario struct {
	Name   string
	Phases []phase
}

type app struct {
	Name      string
	Templates map[string]templateDef
	Scenarios map[string]scenario
}

type registry struct {
	commonTemplates map[string]templateDef
	apps            map[string]app
}

type scenarioEntry struct {
	AppName  string
	ScenName string
	FullName string
}

func loadRegistry(data []byte, source string) (*registry, error) {
	var rc rawConfig
	if err := json.Unmarshal(data, &rc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", source, err)
	}
	reg := &registry{
		commonTemplates: make(map[string]templateDef),
		apps:            make(map[string]app),
	}
	for _, t := range rc.Common.Templates {
		td, err := parseTemplateDef(t)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", source, err)
		}
		reg.commonTemplates[t.Name] = td
	}
	for appName, ra := range rc.Apps {
		a := app{
			Name:      appName,
			Templates: make(map[string]templateDef),
			Scenarios: make(map[string]scenario),
		}
		for _, t := range ra.Templates {
			td, err := parseTemplateDef(t)
			if err != nil {
				return nil, fmt.Errorf("%s app %q: %w", source, appName, err)
			}
			a.Templates[t.Name] = td
		}
		for sn, rs := range ra.Scenarios {
			s, err := resolveScenario(sn, rs)
			if err != nil {
				return nil, fmt.Errorf("%s app %q scenario %q: %w", source, appName, sn, err)
			}
			a.Scenarios[sn] = s
		}
		reg.apps[appName] = a
	}
	return reg, nil
}

func loadRegistryFile(path string) (*registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return loadRegistry(data, path)
}

func parseTemplateDef(t rawTemplate) (templateDef, error) {
	if t.Name == "" {
		return templateDef{}, fmt.Errorf("template missing name")
	}
	hasTmpl := t.Template != ""
	hasLines := len(t.Lines) > 0
	if hasTmpl && hasLines {
		return templateDef{}, fmt.Errorf("template %q: cannot have both template and lines", t.Name)
	}
	if !hasTmpl && !hasLines {
		return templateDef{}, fmt.Errorf("template %q: must have template or lines", t.Name)
	}
	if hasLines {
		return templateDef{Name: t.Name, Lines: t.Lines}, nil
	}
	return templateDef{Name: t.Name, Line: t.Template}, nil
}

func resolveScenario(name string, rs rawScenarioDef) (scenario, error) {
	hasTmpl := len(rs.Templates) > 0
	hasPhases := len(rs.Phases) > 0
	if hasTmpl && hasPhases {
		return scenario{}, fmt.Errorf("scenario %q: cannot have both templates and phases", name)
	}
	if !hasTmpl && !hasPhases {
		return scenario{}, fmt.Errorf("scenario %q: must have templates or phases", name)
	}
	if hasTmpl {
		refs, err := toWeightedRefs(rs.Templates)
		if err != nil {
			return scenario{}, err
		}
		return scenario{
			Name: name,
			Phases: []phase{{
				Name:      name,
				Duration:  0,
				Rate:      0,
				Templates: refs,
			}},
		}, nil
	}
	var phases []phase
	for _, rp := range rs.Phases {
		dur, err := parseDuration(rp.Duration)
		if err != nil {
			return scenario{}, fmt.Errorf("phase %q: %w", rp.Name, err)
		}
		refs, err := toWeightedRefs(rp.Templates)
		if err != nil {
			return scenario{}, fmt.Errorf("phase %q: %w", rp.Name, err)
		}
		if rp.Rate <= 0 {
			return scenario{}, fmt.Errorf("phase %q: rate must be > 0", rp.Name)
		}
		phases = append(phases, phase{
			Name:      rp.Name,
			Duration:  dur,
			Rate:      rp.Rate,
			Templates: refs,
		})
	}
	return scenario{Name: name, Phases: phases}, nil
}

func toWeightedRefs(raw []rawWeightedRef) ([]weightedRef, error) {
	var refs []weightedRef
	for _, r := range raw {
		if r.Name == "" {
			return nil, fmt.Errorf("weighted ref missing name")
		}
		if r.Weight <= 0 {
			return nil, fmt.Errorf("ref %q: weight must be > 0", r.Name)
		}
		refs = append(refs, weightedRef{Name: r.Name, Weight: r.Weight})
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("no templates")
	}
	return refs, nil
}

func parseDuration(s string) (time.Duration, error) {
	if s == "0s" || s == "0" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

func (reg *registry) resolveScenario(fullName string) (*app, *scenario, error) {
	appName, scenName := reg.splitScenarioName(fullName)
	a, ok := reg.apps[appName]
	if !ok {
		return nil, nil, fmt.Errorf("unknown app %q\navailable: %s", appName, strings.Join(reg.appNames(), ", "))
	}
	s, ok := a.Scenarios[scenName]
	if !ok {
		return nil, nil, fmt.Errorf("unknown scenario %q for app %q\navailable: %s", scenName, appName, strings.Join(a.scenarioNames(), ", "))
	}
	return &a, &s, nil
}

func (reg *registry) splitScenarioName(name string) (appName, scenName string) {
	if a, ok := reg.apps[name]; ok {
		if _, ok := a.Scenarios["default"]; ok {
			return name, "default"
		}
	}
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '-' {
			candidate := name[:i]
			suffix := name[i+1:]
			if a, ok := reg.apps[candidate]; ok {
				if _, ok := a.Scenarios[suffix]; ok {
					return candidate, suffix
				}
			}
		}
	}
	return name, "default"
}

func (reg *registry) appNames() []string {
	names := make([]string, 0, len(reg.apps))
	for n := range reg.apps {
		names = append(names, n)
	}
	return names
}

func (a *app) scenarioNames() []string {
	names := make([]string, 0, len(a.Scenarios))
	for n := range a.Scenarios {
		names = append(names, n)
	}
	return names
}

func (reg *registry) lookupTemplate(appName, tmplName string) (templateDef, bool) {
	if a, ok := reg.apps[appName]; ok {
		if td, ok := a.Templates[tmplName]; ok {
			return td, true
		}
	}
	if td, ok := reg.commonTemplates[tmplName]; ok {
		return td, true
	}
	return templateDef{}, false
}

func (reg *registry) listScenarios() []scenarioEntry {
	var entries []scenarioEntry
	for _, a := range reg.apps {
		for sn := range a.Scenarios {
			fullName := a.Name
			if sn != "default" {
				fullName = a.Name + "-" + sn
			}
			entries = append(entries, scenarioEntry{
				AppName:  a.Name,
				ScenName: sn,
				FullName: fullName,
			})
		}
	}
	return entries
}
