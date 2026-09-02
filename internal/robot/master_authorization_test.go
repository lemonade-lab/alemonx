package robot

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSetMasterAuthorizationOnlyRewritesMasterIDSection(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	before := "port: 17117\n# keep this comment\nlogin: qq\nmaster_id:\n  'existing': true\n  'disabled': false\nother:\n  value: keep\n"
	if _, err := (Manager{}).Write(root, "alemon.config.yaml", before); err != nil {
		t.Fatal(err)
	}
	if _, err := (Manager{}).SetMasterAuthorization(root, "37B6A23E5394585D485C60456486EDD4", true); err != nil {
		t.Fatal(err)
	}
	content, err := (Manager{}).Read(root, "alemon.config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"port: 17117", "# keep this comment", "login: qq", "other:\n  value: keep", "'existing': true", "'37B6A23E5394585D485C60456486EDD4': true"} {
		if !strings.Contains(content.Output, want) {
			t.Fatalf("updated config missing %q:\n%s", want, content.Output)
		}
	}
	if strings.Contains(content.Output, "'disabled': false") {
		t.Fatalf("disabled entry should not remain in generated master section:\n%s", content.Output)
	}
	if _, err := (Manager{}).SetMasterAuthorization(root, "37B6A23E5394585D485C60456486EDD4", false); err != nil {
		t.Fatal(err)
	}
	content, err = (Manager{}).Read(root, "alemon.config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content.Output, "37B6A23E5394585D485C60456486EDD4") {
		t.Fatalf("master ID was not removed:\n%s", content.Output)
	}
}

func TestSetMasterAuthorizationRejectsUnsafeUserID(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	if _, err := (Manager{}).SetMasterAuthorization(root, "id: true\nlogin: hijacked", true); err == nil {
		t.Fatal("unsafe ID should be rejected")
	}
}
