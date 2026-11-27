// Command k1rothic is a terminal LLM chat + coding agent and a launcher for
// the official vendor CLIs.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/akutor/k1rothic-cli/internal/config"
	"github.com/akutor/k1rothic-cli/internal/core/provider/registry"
	"github.com/akutor/k1rothic-cli/internal/secrets"
	"github.com/akutor/k1rothic-cli/internal/ui"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "k1rothic:", err)
		os.Exit(1)
	}
}

func run() error {
	showVersion := flag.Bool("version", false, "print version and exit")
	cfgPath := flag.String("config", "", "path to config.toml (default: OS config dir)")
	flag.Parse()

	if *showVersion {
		fmt.Println("k1rothic", version)
		return nil
	}

	path := *cfgPath
	if path == "" {
		p, err := config.Path()
		if err != nil {
			return err
		}
		path = p
	}
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	if _, ok := registry.Lookup(cfg.Provider); !ok {
		return fmt.Errorf("config: unknown provider %q", cfg.Provider)
	}

	m := ui.New(ui.Options{
		Config:     cfg,
		ConfigPath: path,
		Registry:   registry.New(secrets.OS{}),
		Cwd:        cwd,
		Version:    version,
	})
	_, err = tea.NewProgram(m).Run()
	return err
}
