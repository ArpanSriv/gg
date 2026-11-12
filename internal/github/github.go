// Package github is a small client for the github.com REST API.
package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

const apiBase = "https://api.github.com"

// Token is a GitHub token and where it came from.
type Token struct {
	Value  string
	Source string
}

// ResolveToken finds a token, in order: $GH_TOKEN, $GITHUB_TOKEN, the PAT
// stored in gg's config, then `gh auth token`. It returns an empty Token
// when there is none, which is fine for public repos.
func ResolveToken(configPAT string) Token {
	for _, env := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return Token{v, "$" + env}
		}
	}
	if configPAT != "" {
		return Token{configPAT, "gg config"}
	}
	out, err := exec.Command("gh", "auth", "token", "--hostname", "github.com").Output()
	if v := strings.TrimSpace(string(out)); err == nil && v != "" {
		return Token{v, "gh CLI"}
	}
	return Token{}
}

// Client calls the API, with a token when it has one.
type Client struct {
	token string
	http  *http.Client
}

func New(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 30 * time.Second}}
}

// APIError is a non-200 API response.
type APIError struct {
	Status  int
	Message string
	Path    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitHub API %s: %d %s", e.Path, e.Status, e.Message)
}

// IsNotFound reports whether err is an API 404.
func IsNotFound(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

func (c *Client) get(path string, out any) (http.Header, error) {
	req, err := http.NewRequest(http.MethodGet, apiBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "gg-cli")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var body struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		msg := body.Message
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			msg = "API rate limit exceeded"
			if c.token == "" {
				msg += "; log in with `gh auth login` or `gg auth login`"
			}
		} else if resp.StatusCode == http.StatusNotFound && c.token == "" {
			msg += " (private repos need a token: `gh auth login` or `gg auth login`)"
		}
		return nil, &APIError{Status: resp.StatusCode, Message: msg, Path: path}
	}
	return resp.Header, json.NewDecoder(resp.Body).Decode(out)
}

type User struct {
	Login string `json:"login"`
}

type Repo struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Description   string `json:"description"`
	Private       bool   `json:"private"`
	Archived      bool   `json:"archived"`
	Fork          bool   `json:"fork"`
	Owner         User   `json:"owner"`
	DefaultBranch string `json:"default_branch"`
}

type Branch struct {
	Ref  string `json:"ref"`
	User User   `json:"user"`
	Repo *Repo  `json:"repo"` // nil when the fork was deleted
}

type PullRequest struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Head   Branch `json:"head"`
	Base   Branch `json:"base"`
}

// CurrentUser returns the token's user and its OAuth scopes (empty for
// fine-grained tokens).
func (c *Client) CurrentUser() (*User, string, error) {
	var u User
	h, err := c.get("/user", &u)
	if err != nil {
		return nil, "", err
	}
	return &u, h.Get("X-OAuth-Scopes"), nil
}

func (c *Client) PullRequest(owner, repo string, number int) (*PullRequest, error) {
	var pr PullRequest
	_, err := c.get(fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(owner), url.PathEscape(repo), number), &pr)
	if err != nil {
		return nil, err
	}
	return &pr, nil
}

// ListRepos lists the repos of an org or user. For the authenticated
// user it includes their private repos.
func (c *Client) ListRepos(owner string) ([]Repo, error) {
	if c.token != "" {
		if u, _, err := c.CurrentUser(); err == nil && strings.EqualFold(u.Login, owner) {
			return c.paginate("/user/repos?affiliation=owner")
		}
	}
	repos, err := c.paginate("/orgs/" + url.PathEscape(owner) + "/repos?type=all")
	if IsNotFound(err) {
		return c.paginate("/users/" + url.PathEscape(owner) + "/repos?type=owner")
	}
	return repos, err
}

func (c *Client) paginate(path string) ([]Repo, error) {
	var all []Repo
	for page := 1; ; page++ {
		var batch []Repo
		if _, err := c.get(fmt.Sprintf("%s&per_page=100&page=%d", path, page), &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			return all, nil
		}
	}
}
