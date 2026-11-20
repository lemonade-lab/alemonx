package access

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSingleMCPKeyKeepsOperatorAndRevokesLegacyDiagnostic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	manager, err := NewAt(path)
	if err != nil {
		t.Fatal(err)
	}
	session, err := manager.Enable("legacy-mcp", "test-password", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := manager.Status(session)
	if err != nil {
		t.Fatal(err)
	}
	oldOperator, err := manager.UpdateMCPKey(session, identity, "token")
	if err != nil {
		t.Fatal(err)
	}
	oldDiagnostic := mcpKeyPrefix + strings.Repeat("d", len(oldOperator)-len(mcpKeyPrefix))
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]json.RawMessage
	if err := json.Unmarshal(payload, &legacy); err != nil {
		t.Fatal(err)
	}
	legacy["mcpDiagnosticKeys"], err = json.Marshal(map[string]string{identity.RobotScope: mcpKeyDigest(oldDiagnostic)})
	if err != nil {
		t.Fatal(err)
	}
	payload, err = json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}

	manager, err = NewAt(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token string
		valid bool
	}{{oldOperator, true}, {oldDiagnostic, false}} {
		status, valid, err := manager.MCPKeyStatus(tc.token)
		if err != nil || valid != tc.valid || (valid && status.RobotScope != identity.RobotScope) {
			t.Fatalf("unexpected legacy key status valid=%t (%v)", valid, err)
		}
	}
	if _, err := manager.UpdateMCPKey(session, identity, "rotate"); err != nil {
		t.Fatal(err)
	}
	payload, err = os.ReadFile(path)
	if err != nil || strings.Contains(string(payload), "mcpDiagnosticKeys") {
		t.Fatal("legacy diagnostic keys were retained after saving configuration")
	}
}
