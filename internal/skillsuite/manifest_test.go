package skillsuite

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestManifestValidatesExactMembersAndDestinations(t *testing.T) {
	m := Manifest{Schema: 1, Entry: "team/one", Members: []Member{{Namespace: "team", Name: "one", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64)}}}
	raw, _ := json.Marshal(m)
	if _, e := Parse(raw); e != nil {
		t.Fatal(e)
	}
	for name, modify := range map[string]func(*Manifest){"unknown-schema": func(m *Manifest) { m.Schema = 2 }, "floating-version": func(m *Manifest) { m.Members[0].Version = "latest" }, "missing-digest": func(m *Manifest) { m.Members[0].Digest = "" }, "wrong-entry": func(m *Manifest) { m.Entry = "team/missing" }, "duplicate-target": func(m *Manifest) { m.Members = append(m.Members, m.Members[0]); m.Members[1].Namespace = "other" }} {
		t.Run(name, func(t *testing.T) {
			var copy Manifest
			_ = json.Unmarshal(raw, &copy)
			modify(&copy)
			bad, _ := json.Marshal(copy)
			if _, e := Parse(bad); e == nil {
				t.Fatal("accepted invalid manifest")
			}
		})
	}
	if _, e := Parse(append(raw, []byte(" {}")...)); e == nil {
		t.Fatal("accepted trailing JSON")
	}
}
