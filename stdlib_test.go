package stdlib_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abemedia/stdlib"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	tests := []struct {
		dir string
	}{
		{dir: "go1.18"},
		{dir: "go1.23"},
	}

	tmp := t.TempDir()

	if err := copyFiles(analysistest.TestData(), tmp); err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.dir, func(t *testing.T) {
			wd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			defer os.Chdir(wd)

			dir := filepath.Join(tmp, test.dir)

			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}

			output, err := exec.Command("go", "mod", "vendor").CombinedOutput()
			if err != nil {
				t.Fatal(err, strings.TrimSpace(string(output)))
			}

			analysistest.RunWithSuggestedFixes(t, dir, stdlib.NewAnalyzer())
		})
	}
}

func copyFiles(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		rel := strings.Replace(path, source, "", 1)
		if rel == "" || err != nil {
			return err
		}
		if info.IsDir() {
			return os.Mkdir(filepath.Join(destination, rel), info.Mode().Perm())
		}
		data, err := os.ReadFile(filepath.Join(source, rel))
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(destination, rel), data, info.Mode().Perm())
	})
}
