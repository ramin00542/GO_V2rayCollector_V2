package config

import "path/filepath"

type Paths struct {
	Root       string
	ConfigDir  string
	DataDir    string
	OutputDir  string
	ArchiveDir string
	ReportsDir string
}

func DefaultPaths(root string) Paths {
	return Paths{
		Root:       root,
		ConfigDir:  filepath.Join(root, "config"),
		DataDir:    filepath.Join(root, "data"),
		OutputDir:  filepath.Join(root, "output"),
		ArchiveDir: filepath.Join(root, "archive"),
		ReportsDir: filepath.Join(root, "reports"),
	}
}

func (p Paths) ChannelsFile() string { return filepath.Join(p.ConfigDir, "channels.csv") }
func (p Paths) SourcesFile() string  { return filepath.Join(p.ConfigDir, "sources.json") }
func (p Paths) GitHubFile() string   { return filepath.Join(p.ConfigDir, "github.json") }
func (p Paths) SettingsFile() string { return filepath.Join(p.ConfigDir, "collector.json") }
