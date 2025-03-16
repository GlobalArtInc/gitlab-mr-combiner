package gitlab

type RepoInfo struct {
	DefaultBranch string `json:"default_branch"`
	RepoURL       string `json:"ssh_url_to_repo"`
}

type MergeRequest struct {
    IID    int    `json:"iid"`
    Title  string `json:"title"`
}