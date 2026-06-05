// daily-digest fetches today's PRs (including drafts) and commits across one or
// more personal GitHub accounts, summarizes them with the local Claude Code CLI
// (no extra API billing under a Max plan), and writes the result into an
// Obsidian vault as a daily note.
//
// Dependencies: standard library only. External binary required at runtime: the
// `claude` CLI (Claude Code), available on PATH.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// account is a single GitHub identity to scan.
type account struct {
	Label string // human-readable label, e.g. "personal" / "side"
	Login string // GitHub username
	Token string // personal access token (classic or fine-grained, repo scope)
}

// config holds everything resolved from the environment.
type config struct {
	Accounts   []account
	VaultDir   string // Obsidian vault root (or any output dir)
	SubDir     string // sub-folder inside the vault for notes
	ClaudeBin  string // path/name of the claude CLI
	Location   *time.Location
	HTTPClient *http.Client
	Day        time.Time // the day to report on (local midnight)
}

// pullRequest is the trimmed shape we keep from the search API.
type pullRequest struct {
	Repo      string
	Number    int
	Title     string
	State     string // open / closed
	Draft     bool
	Merged    bool
	URL       string
	UpdatedAt time.Time
	Body      string
}

// commit is the trimmed shape we keep from the commit search API.
type commit struct {
	Repo    string
	SHA     string
	Message string
	URL     string
	Date    time.Time
}

// accountActivity bundles one account's findings.
type accountActivity struct {
	Account      account
	PullRequests []pullRequest
	Commits      []commit
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var activities []accountActivity
	for _, acc := range cfg.Accounts {
		act := accountActivity{Account: acc}

		prs, err := fetchPullRequests(ctx, cfg, acc)
		if err != nil {
			return fmt.Errorf("fetch PRs for %s: %w", acc.Label, err)
		}
		act.PullRequests = prs

		commits, err := fetchCommits(ctx, cfg, acc)
		if err != nil {
			return fmt.Errorf("fetch commits for %s: %w", acc.Label, err)
		}
		act.Commits = commits

		activities = append(activities, act)
	}

	if countItems(activities) == 0 {
		fmt.Println("本日のPR・コミットは見つかりませんでした。ノートは作成しません。")
		return nil
	}

	rawReport := buildRawReport(cfg, activities)

	summary, err := summarizeWithClaude(ctx, cfg, rawReport)
	if err != nil {
		// Claude failed; still save the raw report so the data is not lost.
		fmt.Fprintln(os.Stderr, "warning: Claude summary failed, saving raw report only:", err)
		summary = "> [!warning] AI要約に失敗しました。以下は生データです。\n\n" + rawReport
	}

	notePath, err := writeNote(cfg, summary, rawReport)
	if err != nil {
		return fmt.Errorf("write note: %w", err)
	}

	fmt.Printf("保存しました: %s\n", notePath)
	return nil
}

// ---- configuration ----------------------------------------------------------

func loadConfig() (config, error) {
	var cfg config

	cfg.VaultDir = os.Getenv("OBSIDIAN_VAULT")
	if cfg.VaultDir == "" {
		return cfg, fmt.Errorf("OBSIDIAN_VAULT が未設定です（Obsidian vault のパスを指定してください）")
	}
	// Expand a leading ~ so quoted paths like "~/Documents/vault" (which the
	// shell does not expand) still resolve to the user's home directory.
	expanded, err := expandHome(cfg.VaultDir)
	if err != nil {
		return cfg, fmt.Errorf("OBSIDIAN_VAULT のパスを展開できません: %w", err)
	}
	cfg.VaultDir = expanded

	cfg.SubDir = envOr("DIGEST_SUBDIR", "DailyDigest")
	cfg.ClaudeBin = envOr("CLAUDE_BIN", "claude")

	locName := envOr("DIGEST_TZ", "Asia/Tokyo")
	loc, err := time.LoadLocation(locName)
	if err != nil {
		return cfg, fmt.Errorf("タイムゾーン %q を読み込めません: %w", locName, err)
	}
	cfg.Location = loc

	now := time.Now().In(loc)
	if d := os.Getenv("DIGEST_DATE"); d != "" {
		parsed, err := time.ParseInLocation("2006-01-02", d, loc)
		if err != nil {
			return cfg, fmt.Errorf("DIGEST_DATE は YYYY-MM-DD 形式です: %w", err)
		}
		now = parsed
	}
	cfg.Day = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	// Accounts: GH_LOGIN_1/GH_TOKEN_1/GH_LABEL_1, GH_LOGIN_2/... up to 9.
	for i := 1; i <= 9; i++ {
		login := os.Getenv(fmt.Sprintf("GH_LOGIN_%d", i))
		token := os.Getenv(fmt.Sprintf("GH_TOKEN_%d", i))
		if login == "" || token == "" {
			continue
		}
		label := envOr(fmt.Sprintf("GH_LABEL_%d", i), login)
		cfg.Accounts = append(cfg.Accounts, account{Label: label, Login: login, Token: token})
	}
	if len(cfg.Accounts) == 0 {
		return cfg, fmt.Errorf("アカウントが未設定です（GH_LOGIN_1 / GH_TOKEN_1 などを設定してください）")
	}

	cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	return cfg, nil
}

// expandHome replaces a leading ~ (or ~/) in path with the user's home
// directory. Other ~user forms are left untouched. Paths without a leading ~
// are returned unchanged.
func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ---- GitHub fetching ---------------------------------------------------------

const githubAPI = "https://api.github.com"

// dayBounds returns the inclusive date string range for the configured day.
func (c config) dayString() string {
	return c.Day.Format("2006-01-02")
}

// ghGet performs an authenticated GET and decodes JSON into v.
func ghGet(ctx context.Context, cfg config, token, endpoint string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "daily-digest")

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, v)
}

// fetchPullRequests uses the search API to find PRs authored by the account and
// updated on the target day. Drafts are included (search returns them; we keep
// the draft flag). Closed/merged PRs touched today are included too.
func fetchPullRequests(ctx context.Context, cfg config, acc account) ([]pullRequest, error) {
	day := cfg.dayString()
	// author:<login> type:pr updated:<day>
	q := fmt.Sprintf("author:%s type:pr updated:%s", acc.Login, day)
	endpoint := fmt.Sprintf("%s/search/issues?q=%s&per_page=100", githubAPI, url.QueryEscape(q))

	var sr struct {
		Items []struct {
			Number        int    `json:"number"`
			Title         string `json:"title"`
			State         string `json:"state"`
			Draft         bool   `json:"draft"`
			HTMLURL       string `json:"html_url"`
			Body          string `json:"body"`
			UpdatedAt     string `json:"updated_at"`
			RepositoryURL string `json:"repository_url"`
			PullRequest   *struct {
				MergedAt *string `json:"merged_at"`
			} `json:"pull_request"`
		} `json:"items"`
	}
	if err := ghGet(ctx, cfg, acc.Token, endpoint, &sr); err != nil {
		return nil, err
	}

	var out []pullRequest
	for _, it := range sr.Items {
		updated, _ := time.Parse(time.RFC3339, it.UpdatedAt)
		pr := pullRequest{
			Repo:      repoFromAPIURL(it.RepositoryURL),
			Number:    it.Number,
			Title:     it.Title,
			State:     it.State,
			Draft:     it.Draft,
			URL:       it.HTMLURL,
			UpdatedAt: updated.In(cfg.Location),
			Body:      truncate(it.Body, 1500),
		}
		if it.PullRequest != nil && it.PullRequest.MergedAt != nil {
			pr.Merged = true
		}
		out = append(out, pr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// fetchCommits uses the commit search API for commits authored by the account
// on the target day. Requires the cloak preview accept header historically, but
// the current API version returns commit search without it.
func fetchCommits(ctx context.Context, cfg config, acc account) ([]commit, error) {
	day := cfg.dayString()
	q := fmt.Sprintf("author:%s author-date:%s", acc.Login, day)
	endpoint := fmt.Sprintf("%s/search/commits?q=%s&per_page=100&sort=author-date&order=desc",
		githubAPI, url.QueryEscape(q))

	var sr struct {
		Items []struct {
			SHA     string `json:"sha"`
			HTMLURL string `json:"html_url"`
			Commit  struct {
				Message string `json:"message"`
				Author  struct {
					Date string `json:"date"`
				} `json:"author"`
			} `json:"commit"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
		} `json:"items"`
	}
	if err := ghGet(ctx, cfg, acc.Token, endpoint, &sr); err != nil {
		return nil, err
	}

	var out []commit
	for _, it := range sr.Items {
		date, _ := time.Parse(time.RFC3339, it.Commit.Author.Date)
		out = append(out, commit{
			Repo:    it.Repository.FullName,
			SHA:     shortSHA(it.SHA),
			Message: firstLine(it.Commit.Message),
			URL:     it.HTMLURL,
			Date:    date.In(cfg.Location),
		})
	}
	return out, nil
}

func repoFromAPIURL(apiURL string) string {
	// https://api.github.com/repos/owner/name -> owner/name
	const marker = "/repos/"
	if i := strings.Index(apiURL, marker); i >= 0 {
		return apiURL[i+len(marker):]
	}
	return apiURL
}

// ---- report assembly ---------------------------------------------------------

func countItems(acts []accountActivity) int {
	n := 0
	for _, a := range acts {
		n += len(a.PullRequests) + len(a.Commits)
	}
	return n
}

// buildRawReport produces a compact Markdown summary of the raw data. This is
// both the prompt input for Claude and the fallback content if Claude fails.
func buildRawReport(cfg config, acts []accountActivity) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# GitHub Activity %s\n\n", cfg.dayString())

	for _, a := range acts {
		fmt.Fprintf(&b, "## アカウント: %s (@%s)\n\n", a.Account.Label, a.Account.Login)

		fmt.Fprintf(&b, "### Pull Requests (%d)\n\n", len(a.PullRequests))
		if len(a.PullRequests) == 0 {
			b.WriteString("（なし）\n\n")
		}
		for _, pr := range a.PullRequests {
			status := pr.State
			switch {
			case pr.Merged:
				status = "merged"
			case pr.Draft:
				status = "draft"
			}
			fmt.Fprintf(&b, "- [%s] %s #%d — %s\n  %s\n", status, pr.Repo, pr.Number, pr.Title, pr.URL)
			if pr.Body != "" {
				fmt.Fprintf(&b, "  > %s\n", strings.ReplaceAll(pr.Body, "\n", "\n  > "))
			}
		}
		b.WriteString("\n")

		fmt.Fprintf(&b, "### Commits (%d)\n\n", len(a.Commits))
		if len(a.Commits) == 0 {
			b.WriteString("（なし）\n\n")
		}
		for _, c := range a.Commits {
			fmt.Fprintf(&b, "- %s `%s` %s\n  %s\n", c.Repo, c.SHA, c.Message, c.URL)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ---- Claude summarization ----------------------------------------------------

const promptHeader = `あなたは開発者の作業ログをまとめるアシスタントです。
以下はある開発者の本日のGitHub活動（PR・コミット）の生データです。
これを日本語で簡潔にまとめてください。出力はObsidianに貼るMarkdownです。

要件:
- 冒頭に箇条書きで「本日のハイライト」を3〜5点
- アカウントごと、リポジトリごとに何をしたかを整理
- draftのPRは「進行中」として明示
- 推測や誇張はせず、データにある事実のみ
- 見出しレベルは ## から開始（# は使わない）

--- 生データ ---
`

// summarizeWithClaude pipes the raw report to the claude CLI in non-interactive
// (print) mode and returns its stdout. This uses the user's Max-plan session,
// avoiding per-token API billing.
func summarizeWithClaude(ctx context.Context, cfg config, rawReport string) (string, error) {
	prompt := promptHeader + rawReport

	// `claude -p <prompt>` runs a single non-interactive turn and prints the
	// result to stdout. We pass the prompt on stdin to avoid argv length limits.
	cmd := exec.CommandContext(ctx, cfg.ClaudeBin, "-p", "--output-format", "text")
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", fmt.Errorf("claude が空の出力を返しました")
	}
	return out, nil
}

// ---- Obsidian output ---------------------------------------------------------

func writeNote(cfg config, summary, rawReport string) (string, error) {
	dir := filepath.Join(cfg.VaultDir, cfg.SubDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	notePath := filepath.Join(dir, cfg.dayString()+".md")

	var b strings.Builder
	// YAML frontmatter for Obsidian.
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "date: %s\n", cfg.dayString())
	fmt.Fprintf(&b, "tags: [github, daily-digest]\n")
	fmt.Fprintf(&b, "generated: %s\n", time.Now().In(cfg.Location).Format(time.RFC3339))
	fmt.Fprintf(&b, "---\n\n")

	fmt.Fprintf(&b, "# GitHub Daily Digest — %s\n\n", cfg.dayString())
	b.WriteString(summary)
	b.WriteString("\n\n---\n\n")
	b.WriteString("> [!note]- 生データ（折りたたみ）\n")
	for _, line := range strings.Split(rawReport, "\n") {
		b.WriteString("> " + line + "\n")
	}

	if err := writeFileAtomic(notePath, []byte(b.String())); err != nil {
		return "", err
	}
	return notePath, nil
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	if _, err := w.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ---- small helpers -----------------------------------------------------------

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
