package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/masahif/linktadoru/internal/config"
	"github.com/spf13/cobra"
)

func TestReadSeedURLs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seeds.txt")
	content := "# targets\n\n https://a.example \r\nhttps://b.example/path#fragment\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := readSeedURLs(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://a.example", "https://b.example/path#fragment"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readSeedURLs() = %q, want %q", got, want)
	}
}

func TestReadSeedURLsFromStdin(t *testing.T) {
	got, err := readSeedURLs("-", strings.NewReader("https://a.example\n# skip\nhttps://b.example\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://a.example", "https://b.example"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readSeedURLs() = %q, want %q", got, want)
	}
}

func TestReadSeedURLsMissingFile(t *testing.T) {
	if _, err := readSeedURLs(filepath.Join(t.TempDir(), "missing"), nil); err == nil {
		t.Fatal("readSeedURLs() succeeded for a missing file")
	}
}

func TestApplySeedURLs(t *testing.T) {
	newCommand := func() *cobra.Command {
		cmd := &cobra.Command{}
		cmd.Flags().String("seed-file", "", "")
		return cmd
	}

	t.Run("arguments override config", func(t *testing.T) {
		cfg := &config.CrawlConfig{SeedURLs: []string{"https://config.example"}}
		if err := applySeedURLs(newCommand(), []string{"https://arg.example"}, cfg); err != nil {
			t.Fatal(err)
		}
		if got := cfg.SeedURLs; !reflect.DeepEqual(got, []string{"https://arg.example"}) {
			t.Fatalf("SeedURLs = %q", got)
		}
	})

	t.Run("file overrides config and rejects arguments", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "seeds.txt")
		if err := os.WriteFile(path, []byte("https://file.example\n"), 0600); err != nil {
			t.Fatal(err)
		}
		cmd := newCommand()
		if err := cmd.Flags().Set("seed-file", path); err != nil {
			t.Fatal(err)
		}
		cfg := &config.CrawlConfig{SeedURLs: []string{"https://config.example"}}
		if err := applySeedURLs(cmd, nil, cfg); err != nil {
			t.Fatal(err)
		}
		if got := cfg.SeedURLs; !reflect.DeepEqual(got, []string{"https://file.example"}) {
			t.Fatalf("SeedURLs = %q", got)
		}
		if err := applySeedURLs(cmd, []string{"https://arg.example"}, cfg); err == nil {
			t.Fatal("combined seed inputs were accepted")
		}
	})
}
