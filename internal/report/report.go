package report

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/config"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/health"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/state"
)

type RunResult struct {
	StartedAt  time.Time
	FinishedAt time.Time
	NewConfigs int
	Requests   int
	Succeeded  int
	Failed     int
	Accepted   int
	Rejected   int
	BytesRead  int
}

type ManifestFile struct {
	Path   string `json:"path"`
	Count  int    `json:"count"`
	Size   int64  `json:"size"`
	RawURL string `json:"raw_url,omitempty"`
}
type Manifest struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Files       []ManifestFile `json:"files"`
}

func Write(paths config.Paths, result RunResult, candidates state.CandidateData) error {
	if err := os.MkdirAll(paths.ReportsDir, 0755); err != nil {
		return err
	}
	if err := writeCollector(filepath.Join(paths.ReportsDir, "collector_stats.md"), result); err != nil {
		return err
	}
	if err := writeDiscovery(filepath.Join(paths.ReportsDir, "discovery_report.md"), candidates); err != nil {
		return err
	}
	healthStore, err := health.Open(filepath.Join(paths.DataDir, "state", "health.json"))
	if err == nil {
		if err = writeHealth(filepath.Join(paths.ReportsDir, "channels_report.md"), "Channels", healthStore.Data().Channels); err != nil {
			return err
		}
		if err = writeHealth(filepath.Join(paths.ReportsDir, "sources_report.md"), "Sources", healthStore.Data().Sources); err != nil {
			return err
		}
	}
	manifest, err := buildManifest(paths)
	if err != nil {
		return err
	}
	if err = writeLinks(filepath.Join(paths.ReportsDir, "links.md"), manifest); err != nil {
		return err
	}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(paths.ReportsDir, "manifest.json"), b, 0644); err != nil {
		return err
	}
	return appendHistory(filepath.Join(paths.ReportsDir, "history.csv"), result)
}
func writeCollector(path string, r RunResult) error {
	body := fmt.Sprintf("# Collector Statistics\n\n## Run\n\n| Metric | Value |\n|---|---:|\n| Started (UTC) | `%s` |\n| Finished (UTC) | `%s` |\n| New fingerprints | `%d` |\n| Requests | `%d` |\n| Successful requests | `%d` |\n| Failed requests | `%d` |\n| Accepted configs | `%d` |\n| Rejected candidates | `%d` |\n| Bytes read | `%d` |\n", r.StartedAt.Format(time.RFC3339), r.FinishedAt.Format(time.RFC3339), r.NewConfigs, r.Requests, r.Succeeded, r.Failed, r.Accepted, r.Rejected, r.BytesRead)
	return os.WriteFile(path, []byte(body), 0644)
}
func writeDiscovery(path string, data state.CandidateData) error {
	counts := map[state.CandidateStatus]int{}
	var rows []string
	for _, c := range data.Candidates {
		counts[c.Status]++
		rows = append(rows, fmt.Sprintf("| `%s` | %s | `%d` | `%d` | `%d` |", c.Kind, c.Status, c.Successes, c.NoConfigCount, len(c.Origins)))
	}
	sort.Strings(rows)
	body := "# Discovery Report\n\n## Queue\n\n| Status | Count |\n|---|---:|\n"
	for _, s := range []state.CandidateStatus{state.CandidatePending, state.CandidateQualified, state.CandidatePromoted, state.CandidateNoConfig, state.CandidateNotFound, state.CandidateUnknown, state.CandidateExpired} {
		body += fmt.Sprintf("| %s | `%d` |\n", s, counts[s])
	}
	body += "\n## Candidates\n\n| Type | State | Successes | No config | Origins |\n|---|---|---:|---:|---:|\n" + strings.Join(rows, "\n") + "\n"
	return os.WriteFile(path, []byte(body), 0644)
}
func writeHealth(path, title string, records map[string]health.Record) error {
	counts := map[health.Status]int{}
	for _, r := range records {
		counts[r.Status]++
	}
	body := fmt.Sprintf("# %s Report\n\n| State | Count |\n|---|---:|\n", title)
	for _, s := range []health.Status{health.StatusActive, health.StatusInactive, health.StatusNotFound, health.StatusUnknown, health.StatusDormant} {
		body += fmt.Sprintf("| %s | `%d` |\n", s, counts[s])
	}
	return os.WriteFile(path, []byte(body), 0644)
}
func buildManifest(paths config.Paths) (Manifest, error) {
	var files []ManifestFile
	for _, root := range []string{paths.OutputDir, paths.ArchiveDir} {
		err := filepath.Walk(root, func(p string, i os.FileInfo, e error) error {
			if e != nil {
				return e
			}
			if i.IsDir() || !strings.HasSuffix(p, ".txt") {
				return nil
			}
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			count := 0
			if strings.TrimSpace(string(b)) != "" {
				count = len(strings.Split(strings.TrimSpace(string(b)), "\n"))
			}
			rel, _ := filepath.Rel(paths.Root, p)
			files = append(files, ManifestFile{Path: filepath.ToSlash(rel), Count: count, Size: i.Size(), RawURL: rawURL(filepath.ToSlash(rel))})
			return nil
		})
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return Manifest{}, err
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return Manifest{GeneratedAt: time.Now().UTC(), Files: files}, nil
}
func rawURL(path string) string {
	repo, branch := os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_REF_NAME")
	if repo == "" || branch == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return "https://raw.githubusercontent.com/" + repo + "/" + branch + "/" + strings.Join(parts, "/")
}
func writeLinks(path string, m Manifest) error {
	body := "# Download Links\n\nGenerated: `" + m.GeneratedAt.Format(time.RFC3339) + "`\n\n| File | Configs | Size | Raw link |\n|---|---:|---:|---|\n"
	for _, f := range m.Files {
		link := "Local repository"
		if f.RawURL != "" {
			link = "[Raw](" + f.RawURL + ")"
		}
		body += fmt.Sprintf("| `%s` | `%d` | `%d B` | %s |\n", f.Path, f.Count, f.Size, link)
	}
	return os.WriteFile(path, []byte(body), 0644)
}
func appendHistory(path string, r RunResult) error {
	header := "timestamp,new_fingerprints,requests,succeeded,failed,accepted,rejected\n"
	if _, e := os.Stat(path); os.IsNotExist(e) {
		if e = os.WriteFile(path, []byte(header), 0644); e != nil {
			return e
		}
	}
	f, e := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = f.WriteString(fmt.Sprintf("%s,%d,%d,%d,%d,%d,%d\n", r.FinishedAt.Format(time.RFC3339), r.NewConfigs, r.Requests, r.Succeeded, r.Failed, r.Accepted, r.Rejected))
	return e
}

