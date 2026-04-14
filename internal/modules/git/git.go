package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// Module vérifie le statut des dépôts git pour les répertoires configurés.
type Module struct {
	repos []string // liste des répertoires de dépôts à surveiller
}

func New(repos []string) *Module {
	return &Module{repos: repos}
}

func (m *Module) Name() string { return "git" }

func (m *Module) Detect() bool {
	if len(m.repos) == 0 {
		return false
	}
	_, err := exec.LookPath("git")
	return err == nil
}

func (m *Module) Collect(ctx context.Context) (transport.ModuleData, error) {
	now := time.Now()
	var metrics []transport.MetricPoint
	metadata := make(map[string]string)

	for _, repo := range m.repos {
		if _, err := os.Stat(repo + "/.git"); err != nil {
			continue
		}

		name := repoName(repo)
		labels := map[string]string{"repo": name}

		branch, err := gitCmd(ctx, repo, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			continue
		}
		metadata["branch_"+name] = branch

		// Nombre de fichiers modifiés
		status, _ := gitCmd(ctx, repo, "status", "--porcelain")
		dirtyCount := 0
		if status != "" {
			dirtyCount = len(strings.Split(status, "\n"))
		}
		metrics = append(metrics, transport.MetricPoint{
			Name: "git_dirty_files", Value: float64(dirtyCount), Labels: labels,
		})

		dirty := 0.0
		if dirtyCount > 0 {
			dirty = 1.0
		}
		metrics = append(metrics, transport.MetricPoint{
			Name: "git_dirty", Value: dirty, Labels: labels,
		})

		// En avance/en retard
		ahead, behind := aheadBehind(ctx, repo, branch)
		metrics = append(metrics,
			transport.MetricPoint{Name: "git_ahead", Value: float64(ahead), Labels: labels},
			transport.MetricPoint{Name: "git_behind", Value: float64(behind), Labels: labels},
		)

		// Âge du dernier commit
		lastCommit, err := gitCmd(ctx, repo, "log", "-1", "--format=%ct")
		if err == nil {
			lastCommit = strings.TrimSpace(lastCommit)
			var ts int64
			if _, err := fmt.Sscanf(lastCommit, "%d", &ts); err == nil {
				age := now.Sub(time.Unix(ts, 0))
				metrics = append(metrics, transport.MetricPoint{
					Name: "git_last_commit_age_hours", Value: age.Hours(), Labels: labels,
				})
			}
		}
	}

	return transport.ModuleData{
		Module:    "git",
		Metrics:   metrics,
		Metadata:  metadata,
		Timestamp: now,
	}, nil
}

func gitCmd(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func aheadBehind(ctx context.Context, dir, branch string) (ahead, behind int) {
	output, err := gitCmd(ctx, dir, "rev-list", "--left-right", "--count", branch+"...origin/"+branch)
	if err != nil {
		return 0, 0
	}
	fmt.Sscanf(output, "%d\t%d", &ahead, &behind)
	return
}

func repoName(path string) string {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}
