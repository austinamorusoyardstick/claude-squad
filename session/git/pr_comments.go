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

type PRReview struct {
	ID          int       `json:"id"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	Author      string    `json:"author"`
	SubmittedAt time.Time `json:"submitted_at"`
	CommitID    string    `json:"commit_id"`
	Comments    []PRComment
}

type PullRequest struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	State    string `json:"state"`
	HeadRef  string `json:"headRef"`
	BaseRef  string `json:"baseRef"`
	URL      string `json:"url"`
	HeadSHA  string `json:"headSHA"`
	Comments []PRComment
	Reviews  []PRReview
}

func GetCurrentPR(workingDir string) (*PullRequest, error) {
	cmd := exec.Command("gh", "pr", "view", "--json", "number,title,state,headRefName,baseRefName,url,headRefOid")
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
		HeadRefOid   string `json:"headRefOid"`
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
		HeadSHA: prData.HeadRefOid,
	}

	return pr, nil
}

func (pr *PullRequest) FetchComments(workingDir string) error {
	// Always clear existing data to ensure fresh fetch
	pr.Comments = []PRComment{}
	pr.Reviews = []PRReview{}

	// First fetch resolved status for review threads
	resolvedMap, err := pr.fetchResolvedStatus(workingDir)
	if err != nil {
		// Log but don't fail - resolved status is optional
		fmt.Printf("Warning: Could not fetch resolved status: %v\n", err)
		resolvedMap = make(map[int]bool)
	}

	// Fetch PR reviews first
	if err := pr.fetchReviews(workingDir); err != nil {
		return err
	}

	// Fetch review comments (line-specific comments) with resolved status
	if err := pr.fetchReviewComments(workingDir, resolvedMap); err != nil {
		return err
	}

	// Fetch general issue comments
	if err := pr.fetchIssueComments(workingDir); err != nil {
		return err
	}

	return nil
}

func (pr *PullRequest) fetchResolvedStatus(workingDir string) (map[int]bool, error) {
	// Get repository info first
	repoCmd := exec.Command("gh", "repo", "view", "--json", "owner,name")
	repoCmd.Dir = workingDir
	repoOutput, err := repoCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get repository info: %w", err)
	}

	var repoInfo struct {
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	}

	if err := json.Unmarshal(repoOutput, &repoInfo); err != nil {
		return nil, fmt.Errorf("failed to parse repository info: %w", err)
	}

	// Use GraphQL to get review thread resolution status
	query := fmt.Sprintf(`
{
  repository(owner: "%s", name: "%s") {
    pullRequest(number: %d) {
      reviewThreads(first: 100) {
        nodes {
          id
          isResolved
          comments(first: 1) {
            nodes {
              databaseId
            }
          }
        }
      }
    }
  }
}`, repoInfo.Owner.Login, repoInfo.Name, pr.Number)

	// Execute GraphQL query
	cmd := exec.Command("gh", "api", "graphql", "-f", fmt.Sprintf("query=%s", query))
	cmd.Dir = workingDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch resolved status: %w", err)
	}

	var response struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						Nodes []struct {
							ID         string `json:"id"`
							IsResolved bool   `json:"isResolved"`
							Comments   struct {
								Nodes []struct {
									DatabaseID int `json:"databaseId"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("failed to parse resolved status response: %w", err)
	}

	// Build map of comment ID to resolved status
	resolvedMap := make(map[int]bool)
	for _, thread := range response.Data.Repository.PullRequest.ReviewThreads.Nodes {
		if len(thread.Comments.Nodes) > 0 {
			commentID := thread.Comments.Nodes[0].DatabaseID
			resolvedMap[commentID] = thread.IsResolved
		}
	}

	return resolvedMap, nil
}

func (pr *PullRequest) fetchReviews(workingDir string) error {
	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/{owner}/{repo}/pulls/%d/reviews", pr.Number))
	cmd.Dir = workingDir
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to fetch reviews: %w", err)
	}

	var reviews []struct {
		ID          int    `json:"id"`
		Body        string `json:"body"`
		State       string `json:"state"`
		User        struct{ Login string `json:"login"` } `json:"user"`
		SubmittedAt string `json:"submitted_at"`
		CommitID    string `json:"commit_id"`
	}

	if err := json.Unmarshal(output, &reviews); err != nil {
		return fmt.Errorf("failed to parse reviews: %w", err)
	}

	for _, r := range reviews {
		// Skip empty review bodies and dismissed reviews
		if strings.TrimSpace(r.Body) == "" || r.State == "DISMISSED" {
			continue
		}

		submittedAt, _ := time.Parse(time.RFC3339, r.SubmittedAt)
		
		// Check if review is outdated (not from the current head commit)
		isOutdated := r.CommitID != pr.HeadSHA

		review := PRReview{
			ID:          r.ID,
			Body:        r.Body,
			State:       r.State,
			Author:      r.User.Login,
			SubmittedAt: submittedAt,
			CommitID:    r.CommitID,
		}

		// Convert review to comment format if not outdated
		if !isOutdated {
			reviewComment := PRComment{
				ID:                 r.ID,
				Body:               r.Body,
				Author:             r.User.Login,
				CreatedAt:          submittedAt,
				UpdatedAt:          submittedAt,
				State:              strings.ToLower(r.State),
				Type:               "review",
				CommitID:           r.CommitID,
				PullRequestReviewID: r.ID,
				IsOutdated:         isOutdated,
				IsResolved:         r.State == "DISMISSED",
				Accepted:           false,
			}
			pr.Comments = append(pr.Comments, reviewComment)
		}

		pr.Reviews = append(pr.Reviews, review)
	}

	return nil
}

func (pr *PullRequest) fetchReviewComments(workingDir string, resolvedMap map[int]bool) error {
	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/{owner}/{repo}/pulls/%d/comments", pr.Number))
	cmd.Dir = workingDir
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to fetch review comments: %w", err)
	}

	var reviewComments []struct {
		ID                  int     `json:"id"`
		Body                string  `json:"body"`
		Path                string  `json:"path"`
		Line                *int    `json:"line"`
		OriginalLine        int     `json:"original_line"`
		Position            *int    `json:"position"`
		OriginalPosition    *int    `json:"original_position"`
		User                struct{ Login string `json:"login"` } `json:"user"`
		CreatedAt           string  `json:"created_at"`
		UpdatedAt           string  `json:"updated_at"`
		CommitID            string  `json:"commit_id"`
		OriginalCommitID    string  `json:"original_commit_id"`
		PullRequestReviewID int     `json:"pull_request_review_id"`
	}

	if err := json.Unmarshal(output, &reviewComments); err != nil {
		return fmt.Errorf("failed to parse review comments: %w", err)
	}

	for _, rc := range reviewComments {
		// Skip empty comments
		if strings.TrimSpace(rc.Body) == "" {
			continue
		}

		createdAt, _ := time.Parse(time.RFC3339, rc.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339, rc.UpdatedAt)
		
		// Check if comment is outdated
		isOutdated := rc.Position == nil || rc.CommitID != pr.HeadSHA
		
		// Check if comment is resolved using the resolved map
		isResolved := resolvedMap[rc.ID]

		// Skip outdated or resolved comments
		if isOutdated || isResolved {
			continue
		}

		line := 0
		if rc.Line != nil {
			line = *rc.Line
		}

		comment := PRComment{
			ID:                 rc.ID,
			Body:               rc.Body,
			Path:               rc.Path,
			Line:               line,
			OriginalLine:       rc.OriginalLine,
			Author:             rc.User.Login,
			CreatedAt:          createdAt,
			UpdatedAt:          updatedAt,
			State:              "pending",
			Type:               "review_comment",
			CommitID:           rc.CommitID,
			OriginalCommitID:   rc.OriginalCommitID,
			Position:           rc.Position,
			OriginalPosition:   rc.OriginalPosition,
			PullRequestReviewID: rc.PullRequestReviewID,
			IsOutdated:         isOutdated,
			IsResolved:         isResolved,
			Accepted:           false,
		}
		pr.Comments = append(pr.Comments, comment)
	}

	return nil
}

func (pr *PullRequest) fetchIssueComments(workingDir string) error {
	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/{owner}/{repo}/issues/%d/comments", pr.Number))
	cmd.Dir = workingDir
	output, err := cmd.Output()
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
		// Skip empty comments
		if strings.TrimSpace(ic.Body) == "" {
			continue
		}

		createdAt, _ := time.Parse(time.RFC3339, ic.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339, ic.UpdatedAt)
		
		comment := PRComment{
			ID:         ic.ID,
			Body:       ic.Body,
			Author:     ic.User.Login,
			CreatedAt:  createdAt,
			UpdatedAt:  updatedAt,
			State:      "pending",
			Type:       "issue_comment",
			IsOutdated: false, // Issue comments are never outdated
			IsResolved: false,
			Accepted:   false,
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

// GetCommentStats returns statistics about the comments (includes both shown and filtered)
func (pr *PullRequest) GetCommentStats() (total, reviews, reviewComments, issueComments, outdated, resolved int) {
	for _, comment := range pr.Comments {
		total++
		if comment.IsOutdated {
			outdated++
		}
		if comment.IsResolved {
			resolved++
		}
		switch comment.Type {
		case "review":
			reviews++
		case "review_comment":
			reviewComments++
		case "issue_comment":
			issueComments++
		}
	}
	return
}

// GetAllCommentStats returns statistics about all comments including filtered ones
func (pr *PullRequest) GetAllCommentStats(workingDir string) (total, shown, outdated, resolved int, err error) {
	// We need to refetch to get complete stats including filtered comments
	resolvedMap, err := pr.fetchResolvedStatus(workingDir)
	if err != nil {
		resolvedMap = make(map[int]bool)
	}

	// Count all review comments including filtered ones
	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/{owner}/{repo}/pulls/%d/comments", pr.Number))
	cmd.Dir = workingDir
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to fetch all comments for stats: %w", err)
	}

	var reviewComments []struct {
		ID               int     `json:"id"`
		Position         *int    `json:"position"`
		CommitID         string  `json:"commit_id"`
	}

	if err := json.Unmarshal(output, &reviewComments); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to parse all comments for stats: %w", err)
	}

	for _, rc := range reviewComments {
		total++
		isOutdated := rc.Position == nil || rc.CommitID != pr.HeadSHA
		isResolved := resolvedMap[rc.ID]
		
		if isOutdated {
			outdated++
		}
		if isResolved {
			resolved++
		}
		if !isOutdated && !isResolved {
			shown++
		}
	}

	// Add issue comments (they're never filtered)
	cmdIssue := exec.Command("gh", "api", fmt.Sprintf("repos/{owner}/{repo}/issues/%d/comments", pr.Number))
	cmdIssue.Dir = workingDir
	issueOutput, err := cmdIssue.Output()
	if err != nil {
		return total, shown, outdated, resolved, nil // Return what we have
	}

	var issueComments []struct {
		ID int `json:"id"`
	}

	if err := json.Unmarshal(issueOutput, &issueComments); err != nil {
		return total, shown, outdated, resolved, nil
	}

	total += len(issueComments)
	shown += len(issueComments)

	return total, shown, outdated, resolved, nil
}

// GetFilteredComments returns comments that are not outdated or resolved
func (pr *PullRequest) GetFilteredComments() []PRComment {
	filtered := []PRComment{}
	for _, comment := range pr.Comments {
		if !comment.IsOutdated && !comment.IsResolved {
			filtered = append(filtered, comment)
		}
	}
	return filtered
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