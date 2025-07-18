package git

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type PRComment struct {
	ID                 int       `json:"id"`
	Body               string    `json:"body"`
	Path               string    `json:"path"`
	Line               int       `json:"line"`
	OriginalLine       int       `json:"original_line"`
	Author             string    `json:"author"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
	State              string    `json:"state"`
	Type               string    `json:"type"`
	CommitID           string    `json:"commit_id"`
	OriginalCommitID   string    `json:"original_commit_id"`
	Position           *int      `json:"position"`
	OriginalPosition   *int      `json:"original_position"`
	PullRequestReviewID int      `json:"pull_request_review_id"`
	IsOutdated         bool      `json:"is_outdated"`
	IsResolved         bool      `json:"is_resolved"`
	Accepted           bool      `json:"-"`
}

type PullRequest struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	State    string `json:"state"`
	HeadRef  string `json:"headRef"`
	BaseRef  string `json:"baseRef"`
	URL      string `json:"url"`
	Comments []PRComment
}

func GetCurrentPR(workingDir string) (*PullRequest, error) {
	cmd := exec.Command("gh", "pr", "view", "--json", "number,title,state,headRefName,baseRefName,url")
	cmd.Dir = workingDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get current PR from %s: %w", workingDir, err)
	}

	var prData struct {
		Number       int    `json:"number"`
		Title        string `json:"title"`
		State        string `json:"state"`
		HeadRefName  string `json:"headRefName"`
		BaseRefName  string `json:"baseRefName"`
		URL          string `json:"url"`
	}

	if err := json.Unmarshal(output, &prData); err != nil {
		return nil, fmt.Errorf("failed to parse PR data: %w", err)
	}

	pr := &PullRequest{
		Number:  prData.Number,
		Title:   prData.Title,
		State:   prData.State,
		HeadRef: prData.HeadRefName,
		BaseRef: prData.BaseRefName,
		URL:     prData.URL,
	}

	return pr, nil
}

func (pr *PullRequest) FetchComments(workingDir string) error {
	cmdReviewComments := exec.Command("gh", "api", fmt.Sprintf("repos/{owner}/{repo}/pulls/%d/comments", pr.Number))
	cmdReviewComments.Dir = workingDir
	output, err := cmdReviewComments.Output()
	if err != nil {
		return fmt.Errorf("failed to fetch review comments: %w", err)
	}

	var reviewComments []struct {
		ID               int    `json:"id"`
		Body             string `json:"body"`
		Path             string `json:"path"`
		Line             int    `json:"line"`
		OriginalLine     int    `json:"original_line"`
		User             struct{ Login string `json:"login"` } `json:"user"`
		CreatedAt        string `json:"created_at"`
		UpdatedAt        string `json:"updated_at"`
		PullRequestReviewID int `json:"pull_request_review_id"`
	}

	if err := json.Unmarshal(output, &reviewComments); err != nil {
		return fmt.Errorf("failed to parse review comments: %w", err)
	}

	pr.Comments = []PRComment{}
	for _, rc := range reviewComments {
		createdAt, _ := time.Parse(time.RFC3339, rc.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339, rc.UpdatedAt)
		
		comment := PRComment{
			ID:        rc.ID,
			Body:      rc.Body,
			Path:      rc.Path,
			Line:      rc.Line,
			Author:    rc.User.Login,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			State:     "pending",
			Type:      "review",
			Accepted:  false,
		}
		pr.Comments = append(pr.Comments, comment)
	}

	cmdIssueComments := exec.Command("gh", "api", fmt.Sprintf("repos/{owner}/{repo}/issues/%d/comments", pr.Number))
	cmdIssueComments.Dir = workingDir
	output, err = cmdIssueComments.Output()
	if err != nil {
		return fmt.Errorf("failed to fetch issue comments: %w", err)
	}

	var issueComments []struct {
		ID        int    `json:"id"`
		Body      string `json:"body"`
		User      struct{ Login string `json:"login"` } `json:"user"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	if err := json.Unmarshal(output, &issueComments); err != nil {
		return fmt.Errorf("failed to parse issue comments: %w", err)
	}

	for _, ic := range issueComments {
		createdAt, _ := time.Parse(time.RFC3339, ic.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339, ic.UpdatedAt)
		
		comment := PRComment{
			ID:        ic.ID,
			Body:      ic.Body,
			Author:    ic.User.Login,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			State:     "pending",
			Type:      "issue",
			Accepted:  false,
		}
		pr.Comments = append(pr.Comments, comment)
	}

	return nil
}

func (pr *PullRequest) GetAcceptedComments() []PRComment {
	accepted := []PRComment{}
	for _, comment := range pr.Comments {
		if comment.Accepted {
			accepted = append(accepted, comment)
		}
	}
	return accepted
}

func (comment *PRComment) GetFormattedBody() string {
	lines := strings.Split(comment.Body, "\n")
	formattedLines := []string{}
	for _, line := range lines {
		if len(line) > 80 {
			for i := 0; i < len(line); i += 80 {
				end := i + 80
				if end > len(line) {
					end = len(line)
				}
				formattedLines = append(formattedLines, line[i:end])
			}
		} else {
			formattedLines = append(formattedLines, line)
		}
	}
	return strings.Join(formattedLines, "\n")
}