package chat

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/thwoodle/open-pilot/internal/domain"
	"gopkg.in/yaml.v3"
)

var commitRecommendationRegex = regexp.MustCompile(`(?s)<COMMIT_RECOMMENDATION>(.*?)</COMMIT_RECOMMENDATION>`)

type safePilotCommitRecommendation struct {
	Status          string            `yaml:"status"`
	SecurityScan    string            `yaml:"security_scan"`
	IssuesFound     int               `yaml:"issues_found"`
	CommitsProposed int               `yaml:"commits_proposed"`
	Commits         []safePilotCommit `yaml:"commits"`
}

type safePilotCommit struct {
	ID       int      `yaml:"id"`
	Type     string   `yaml:"type"`
	Scope    string   `yaml:"scope"`
	Subject  string   `yaml:"subject"`
	Body     string   `yaml:"body"`
	Files    []string `yaml:"files"`
	Breaking bool     `yaml:"breaking"`
	Closes   string   `yaml:"closes"`
}

func parseSafePilotCommitRecommendation(content string) (safePilotCommitRecommendation, string, bool, error) {
	matches := commitRecommendationRegex.FindStringSubmatch(content)
	if len(matches) < 2 {
		return safePilotCommitRecommendation{}, "", false, nil
	}
	raw := strings.TrimSpace(matches[1])
	if raw == "" {
		return safePilotCommitRecommendation{}, "", true, fmt.Errorf("empty commit recommendation block")
	}

	var rec safePilotCommitRecommendation
	if err := yaml.Unmarshal([]byte(raw), &rec); err != nil {
		return safePilotCommitRecommendation{}, raw, true, fmt.Errorf("parse commit recommendation yaml: %w", err)
	}
	return rec, raw, true, nil
}

func (e *Engine) runSafePilotCommitRecommendation(s *domain.Session, requestID, content string) {
	if s == nil || strings.TrimSpace(content) == "" || e.runCmd == nil {
		return
	}

	rec, rawBlock, found, err := parseSafePilotCommitRecommendation(content)
	if !found {
		return
	}
	key := s.ID + "|" + strings.TrimSpace(requestID) + "|" + rawBlock
	if _, seen := e.processedCommitRec[key]; seen {
		return
	}
	e.processedCommitRec[key] = struct{}{}

	if err != nil {
		e.Store.AddSessionSystemMessage(s.ID, "Safe-pilot committing skipped: "+err.Error())
		return
	}
	if strings.ToLower(strings.TrimSpace(rec.Status)) != "safe" ||
		strings.ToLower(strings.TrimSpace(rec.SecurityScan)) != "pass" ||
		rec.IssuesFound > 0 {
		e.Store.AddSessionSystemMessage(
			s.ID,
			fmt.Sprintf(
				"Safe-pilot committing blocked by recommendation (status=%q, security_scan=%q, issues_found=%d).",
				strings.TrimSpace(rec.Status),
				strings.TrimSpace(rec.SecurityScan),
				rec.IssuesFound,
			),
		)
		return
	}
	if len(rec.Commits) == 0 {
		e.Store.AddSessionSystemMessage(s.ID, "Safe-pilot committing skipped: no commits proposed.")
		return
	}

	repoPath := e.repoPathForSession(s)
	if strings.TrimSpace(repoPath) == "" {
		e.Store.AddSessionSystemMessage(s.ID, "Safe-pilot committing skipped: no active repo path.")
		return
	}

	completed := 0
	for i, commit := range rec.Commits {
		if err := e.executeRecommendedCommit(repoPath, commit); err != nil {
			e.Store.AddSessionSystemMessage(
				s.ID,
				fmt.Sprintf("Safe-pilot committing failed at commit %d/%d: %v", i+1, len(rec.Commits), err),
			)
			return
		}
		completed++
	}

	e.Store.AddSessionSystemMessage(s.ID, fmt.Sprintf("Safe-pilot committing created %d commit(s).", completed))
}

func (e *Engine) executeRecommendedCommit(repoPath string, commit safePilotCommit) error {
	files := cleanCommitFiles(commit.Files)
	if len(files) == 0 {
		return fmt.Errorf("commit %d has no files", commit.ID)
	}
	header, bodies, err := buildCommitMessage(commit)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if out, err := e.runCmd(ctx, repoPath, "git", append([]string{"add", "--"}, files...)...); err != nil {
		return fmt.Errorf("git add failed: %w (%s)", err, strings.TrimSpace(out))
	}

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := e.runCmd(ctx, repoPath, "git", "diff", "--cached", "--quiet"); err == nil {
		return fmt.Errorf("no staged changes for commit %d after git add", commit.ID)
	} else if code, ok := commandExitCode(err); !ok || code != 1 {
		return fmt.Errorf("git diff --cached --quiet failed: %w", err)
	}

	args := []string{"commit", "-m", header}
	for _, body := range bodies {
		args = append(args, "-m", body)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if out, err := e.runCmd(ctx, repoPath, "git", args...); err != nil {
		return fmt.Errorf("git commit failed: %w (%s)", err, strings.TrimSpace(out))
	}
	return nil
}

func cleanCommitFiles(files []string) []string {
	seen := make(map[string]struct{}, len(files))
	out := make([]string, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}
		out = append(out, file)
	}
	return out
}

func buildCommitMessage(commit safePilotCommit) (string, []string, error) {
	typ := strings.TrimSpace(commit.Type)
	if typ == "" {
		typ = "chore"
	}
	subject := strings.TrimSpace(commit.Subject)
	if subject == "" {
		return "", nil, fmt.Errorf("commit %d missing subject", commit.ID)
	}

	header := typ
	if scope := strings.TrimSpace(commit.Scope); scope != "" {
		header += "(" + scope + ")"
	}
	header += ": " + subject

	bodies := make([]string, 0, 3)
	body := strings.TrimSpace(commit.Body)
	if body != "" {
		bodies = append(bodies, body)
	}
	if commit.Breaking && !strings.Contains(strings.ToUpper(body), "BREAKING CHANGE:") {
		bodies = append(bodies, "BREAKING CHANGE: this commit introduces a breaking change.")
	}
	if closes := strings.TrimSpace(commit.Closes); closes != "" {
		bodies = append(bodies, closes)
	}
	return header, bodies, nil
}
