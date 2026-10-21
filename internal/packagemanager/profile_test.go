package packagemanager

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPortableLaunchUsesDeclaredVersion(t *testing.T) {
	for _, tc := range []struct {
		manager, declaration string
		want                 []string
	}{
		{"yarn", "", []string{"--yes", "yarn@1.22.22"}},
		{"pnpm", "", []string{"--yes", "pnpm@9.15.0"}},
		{"pnpm", "pnpm@10.17.1", []string{"--yes", "pnpm@10.17.1"}},
		{"pnpm", "pnpm@9.15.0+sha512.abc", []string{"--yes", "pnpm@9.15.0"}},
		{"yarn", "yarn@4.9.2", []string{"--yes", "--package=@yarnpkg/cli-dist@4.9.2", "yarn"}},
	} {
		t.Run(tc.manager+"/"+tc.declaration, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"packageManager":"`+tc.declaration+`"}`), 0600); err != nil {
				t.Fatal(err)
			}
			if got := NPXArgs(root, tc.manager); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("fallback = %#v, want %#v", got, tc.want)
			}
		})
	}
}
