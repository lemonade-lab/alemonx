package robot

import "testing"

func TestSplitGitPackageSourceKeepsTag(t *testing.T) {
	repository, ref, err := splitGitPackageSource("git+https://github.com/lemonade-lab/example.git#v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if repository != "https://github.com/lemonade-lab/example.git" || ref != "v1.2.3" {
		t.Fatalf("got %q, %q", repository, ref)
	}
	if name := localPackageName("git+https://github.com/lemonade-lab/example.git#v1.2.3"); name != "example" {
		t.Fatalf("name = %q", name)
	}
	if _, _, err := splitGitPackageSource("git+https://github.com/lemonade-lab/example.git#--upload-pack=x"); err == nil {
		t.Fatal("unsafe ref must be rejected")
	}
}

func TestGitHubPackageMirrorUsesOnlyPublicHTTPSRepository(t *testing.T) {
	tests := []struct {
		name       string
		repository string
		want       string
		mirrored   bool
	}{
		{
			name:       "github public repository",
			repository: "https://github.com/lemonade-lab/example.git",
			want:       "https://ghfast.top/https://github.com/lemonade-lab/example.git",
			mirrored:   true,
		},
		{
			name:       "gitee stays direct",
			repository: "https://gitee.com/lemonade-lab/example.git",
			want:       "https://gitee.com/lemonade-lab/example.git",
		},
		{
			name:       "github ssh stays direct",
			repository: "ssh://git@github.com/lemonade-lab/example.git",
			want:       "ssh://git@github.com/lemonade-lab/example.git",
		},
		{
			name:       "github credentials stay direct",
			repository: "https://token@github.com/lemonade-lab/example.git",
			want:       "https://token@github.com/lemonade-lab/example.git",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, mirrored := githubPackageMirror(test.repository)
			if got != test.want || mirrored != test.mirrored {
				t.Fatalf("githubPackageMirror(%q) = %q, %v; want %q, %v", test.repository, got, mirrored, test.want, test.mirrored)
			}
		})
	}
}
