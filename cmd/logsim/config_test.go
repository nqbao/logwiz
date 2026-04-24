package main

import (
	"testing"
)

func TestBuiltinsValid(t *testing.T) {
	reg, err := loadRegistry(builtinsData, "builtins.json")
	if err != nil {
		t.Fatalf("builtins.json validation failed: %v", err)
	}
	if len(reg.apps) == 0 {
		t.Fatal("no apps defined in builtins.json")
	}
	if len(reg.commonTemplates) == 0 {
		t.Fatal("no common templates defined in builtins.json")
	}
	for appName, a := range reg.apps {
		if len(a.Templates) == 0 && len(a.Scenarios) == 0 {
			t.Errorf("app %q has no templates or scenarios", appName)
		}
		for sn, s := range a.Scenarios {
			if len(s.Phases) == 0 {
				t.Errorf("app %q scenario %q has no phases", appName, sn)
			}
			for _, ph := range s.Phases {
				if len(ph.Templates) == 0 {
					t.Errorf("app %q scenario %q phase %q has no templates", appName, sn, ph.Name)
				}
				if ph.Rate < 0 {
					t.Errorf("app %q scenario %q phase %q: negative rate", appName, sn, ph.Name)
				}
				for _, ref := range ph.Templates {
					if _, ok := reg.lookupTemplate(appName, ref.Name); !ok {
						t.Errorf("app %q scenario %q phase %q: references unknown template %q", appName, sn, ph.Name, ref.Name)
					}
				}
			}
		}
	}
}

func TestScenarioResolution(t *testing.T) {
	reg, err := loadRegistry(builtinsData, "builtins.json")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		input   string
		wantApp string
		wantSc  string
		wantErr bool
	}{
		{"webapp", "webapp", "default", false},
		{"webapp-incident", "webapp", "incident", false},
		{"webapp-deploy", "webapp", "deploy", false},
		{"nginx", "nginx", "default", false},
		{"pyecom", "pyecom", "default", false},
		{"nonexistent", "", "", true},
		{"webapp-nonexistent", "", "", true},
	}
	for _, tt := range tests {
		app, scen, err := reg.resolveScenario(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("resolveScenario(%q): expected error, got app=%s scen=%s", tt.input, app.Name, scen.Name)
			}
			continue
		}
		if err != nil {
			t.Errorf("resolveScenario(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if app.Name != tt.wantApp {
			t.Errorf("resolveScenario(%q): app = %q, want %q", tt.input, app.Name, tt.wantApp)
		}
		if scen.Name != tt.wantSc {
			t.Errorf("resolveScenario(%q): scenario = %q, want %q", tt.input, scen.Name, tt.wantSc)
		}
	}
}

func TestListScenarios(t *testing.T) {
	reg, err := loadRegistry(builtinsData, "builtins.json")
	if err != nil {
		t.Fatal(err)
	}
	entries := reg.listScenarios()
	if len(entries) == 0 {
		t.Fatal("listScenarios returned no entries")
	}

	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.FullName] {
			t.Errorf("duplicate scenario full name: %q", e.FullName)
		}
		seen[e.FullName] = true
	}

	for _, name := range []string{"webapp", "webapp-incident", "nginx", "pyecom"} {
		if !seen[name] {
			t.Errorf("expected scenario %q not found in list", name)
		}
	}
}
