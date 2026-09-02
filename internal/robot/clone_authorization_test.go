package robot

import (
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestHTTPSAuthorizationOnlyAllowsOfficialHTTPSClone(t *testing.T) {
	httpsURL, err := url.Parse("https://github.com/acme/private.git")
	if err != nil {
		t.Fatal(err)
	}
	sshURL, err := url.Parse("ssh://github.com/acme/private.git")
	if err != nil {
		t.Fatal(err)
	}
	authorization := HTTPSAuthorization{Username: "lemonade", Token: "secret"}
	if err := authorization.validFor(httpsURL, "official"); err != nil {
		t.Fatalf("official HTTPS authorization rejected: %v", err)
	}
	if err := authorization.validFor(httpsURL, "gh-proxy"); err == nil {
		t.Fatal("mirror authorization should be rejected")
	}
	if err := authorization.validFor(sshURL, "official"); err == nil {
		t.Fatal("SSH authorization should be rejected")
	}
}

func TestApplyHTTPSAuthorizationUsesAskPassEnvironment(t *testing.T) {
	command := exec.Command("git", "clone", "https://github.com/acme/private.git")
	cleanup, err := applyHTTPSAuthorization(command, HTTPSAuthorization{
		Username: "lemonade",
		Token:    "token-must-not-be-an-argument",
	})
	if err != nil {
		t.Fatal(err)
	}
	askPass := ""
	for _, value := range command.Env {
		if strings.HasPrefix(value, "GIT_ASKPASS=") {
			askPass = strings.TrimPrefix(value, "GIT_ASKPASS=")
		}
		if strings.Contains(value, "token-must-not-be-an-argument") && !strings.HasPrefix(value, "ALEMONX_GIT_TOKEN=") {
			t.Fatalf("token stored in unexpected environment value: %q", value)
		}
	}
	if askPass == "" {
		t.Fatal("GIT_ASKPASS was not configured")
	}
	if _, err := os.Stat(askPass); err != nil {
		t.Fatalf("askpass script is unavailable: %v", err)
	}
	for _, value := range command.Args {
		if strings.Contains(value, "token-must-not-be-an-argument") {
			t.Fatalf("token leaked into command argument: %q", value)
		}
	}
	cleanup()
	if _, err := os.Stat(askPass); !os.IsNotExist(err) {
		t.Fatalf("askpass script was not removed: %v", err)
	}
}
