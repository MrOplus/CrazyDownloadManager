package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// The UI calls .map / .join / .includes on every list in Settings, so none of
// them may reach it as JSON null (the Scheduler crashed on "days": null).
func TestNormalizeNoNullLists(t *testing.T) {
	for name, raw := range map[string]string{
		"defaults": "",
		"old file with nulls": `{"skipSites":null,"smb":null,"interceptTypes":null,
			"categories":[{"name":"X","extensions":null}],
			"queues":[{"id":"main","name":"Main","maxConcurrent":3,"schedule":{"days":null}}]}`,
	} {
		st := &Store{s: Defaults()}
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &st.s); err != nil {
				t.Fatal(err)
			}
		}
		st.normalize()
		out, err := json.Marshal(st.s)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "null") {
			t.Errorf("%s: settings JSON still contains null: %s", name, out)
		}
		if got := st.s.Queues[0].Schedule.OnComplete; got != "none" {
			t.Errorf("%s: onComplete = %q, want none", name, got)
		}
	}
}

func TestCloseToTrayDefaultsOnForOldFiles(t *testing.T) {
	st := &Store{s: Defaults()}
	if err := json.Unmarshal([]byte(`{"theme":"dark"}`), &st.s); err != nil {
		t.Fatal(err)
	}
	st.normalize()
	if !st.s.CloseToTray {
		t.Error("a settings file without closeToTray should keep the default (true)")
	}
}
