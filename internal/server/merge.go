package server

import (
	"encoding/json"
	"fmt"
	"gitlab-mr-combiner/internal/config"
	"gitlab-mr-combiner/internal/gitlab"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

func (s *Server) combineAllMRs(projectID, mergeRequestID int, r *http.Request) {
	log.Println("Processing MRs for project:", projectID)
	targetBranch := s.GetQueryParam("branch", config.TargetBranch, r)

	repoInfo, err := s.getRepoInfo(projectID)
	if err != nil {
		s.handleErrorAndNotify(projectID, mergeRequestID, fmt.Sprintf("Error fetching repo info: %v", err), r)
		return
	}

	s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Repo Info: Branch=%s, URL=%s", repoInfo.DefaultBranch, repoInfo.RepoURL))

	clonePath := filepath.Join("/gitlab-combiner", fmt.Sprintf("project-%d", projectID))
	hasError := false

	if err := s.prepareRepository(clonePath, repoInfo, targetBranch); err != nil {
		s.handleErrorAndNotify(projectID, mergeRequestID, err.Error(), r)
		return
	}

	if targetBranch == repoInfo.DefaultBranch {
		s.handleErrorAndNotify(projectID, mergeRequestID, "Target branch is the same as the default branch", r)
		return
	}

	mergeRequests, err := s.fetchMergeRequests(projectID)
	if err != nil {
		s.handleErrorAndNotify(projectID, mergeRequestID, fmt.Sprintf("Error fetching MRs: %v", err), r)
		return
	}

	s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Found %d MRs", len(mergeRequests)))

	hasError = s.processMergeRequests(clonePath, mergeRequests, targetBranch, mergeRequestID)

	if err := s.pushChanges(clonePath, targetBranch); err != nil {
		s.handleErrorAndNotify(projectID, mergeRequestID, fmt.Sprintf("Error pushing to remote: %v", err), r)
		return
	}

	s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Merged MRs into %s", targetBranch))
	s.sendComments(projectID, mergeRequestID, hasError, r)
}

func (s *Server) prepareRepository(clonePath string, repoInfo *gitlab.RepoInfo, targetBranch string) error {
	if _, err := os.Stat(clonePath); !os.IsNotExist(err) {
		log.Infof("Repository directory exists, removing it: %s", clonePath)
		if err := os.RemoveAll(clonePath); err != nil {
			return fmt.Errorf("error removing existing repository directory: %v", err)
		}
	}

	log.Infof("Cloning repository to %s", clonePath)
	cloneCmd := exec.Command("git", "clone", "--branch", repoInfo.DefaultBranch, repoInfo.RepoURL, clonePath)
	output, err := cloneCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error cloning repo: %v, output: %s", err, output)
	}

	createBranchCmd := exec.Command("git", "-C", clonePath, "checkout", "-b", targetBranch)
	output, err = createBranchCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error creating target branch from default branch: %v, output: %s", err, output)
	}

	return nil
}

func (s *Server) processMergeRequests(clonePath string, mergeRequests []gitlab.MergeRequest, targetBranch string, mergeRequestID int) bool {
	hasError := false

	for _, mr := range mergeRequests {
		if err := s.processSingleMergeRequest(clonePath, mr, targetBranch, mergeRequestID); err != nil {
			hasError = true
		}
	}

	return hasError
}

func (s *Server) processSingleMergeRequest(clonePath string, mr gitlab.MergeRequest, targetBranch string, mergeRequestID int) error {
	mrBranchName := fmt.Sprintf("mr-%d", mr.IID)

	fetchCmd := exec.Command("git", "-C", clonePath, "fetch", "origin", fmt.Sprintf("merge-requests/%d/head:%s", mr.IID, mrBranchName))
	output, err := fetchCmd.CombinedOutput()
	if err != nil {
		errMsg := fmt.Sprintf("Error fetching MR #%d: %v, output: %s", mr.IID, err, output)
		s.addCommentToBuffer(mergeRequestID, errMsg)
		return fmt.Errorf(errMsg)
	}

	checkoutCmd := exec.Command("git", "-C", clonePath, "checkout", targetBranch)
	output, err = checkoutCmd.CombinedOutput()
	if err != nil {
		errMsg := fmt.Sprintf("Error checking out branch: %v, output: %s", err, output)
		log.Printf(errMsg)
		s.addCommentToBuffer(mergeRequestID, errMsg)
		return fmt.Errorf(errMsg)
	}

	mergeCmd := exec.Command("git", "-C", clonePath, "merge", "--no-ff", mrBranchName)
	output, err = mergeCmd.CombinedOutput()
	if err != nil {
		errMsg := fmt.Sprintf("Error merging MR #%d: %v, output: %s", mr.IID, err, output)
		log.Printf(errMsg)
		s.addCommentToBuffer(mergeRequestID, errMsg)
		return fmt.Errorf(errMsg)
	}

	s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Merged MR #%d: %s", mr.IID, mr.Title))
	return nil
}

func (s *Server) pushChanges(clonePath, targetBranch string) error {
	pushCmd := exec.Command("git", "-C", clonePath, "push", "origin", targetBranch, "--force")
	output, err := pushCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error pushing to remote: %v, output: %s", err, output)
	}
	return nil
}

func (s *Server) handleErrorAndNotify(projectID, mergeRequestID int, errorMessage string, r *http.Request) {
	s.addCommentToBuffer(mergeRequestID, errorMessage)
	s.sendComments(projectID, mergeRequestID, true, r)
}

func (s *Server) fetchMergeRequests(projectID int) ([]gitlab.MergeRequest, error) {
	data, err := s.apiClient.Send("GET", fmt.Sprintf("/projects/%d/merge_requests?state=opened&labels=%s", projectID, config.TriggerTag), nil)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("empty response from GitLab API")
	}

	var mergeRequests []gitlab.MergeRequest
	if err := json.Unmarshal(data, &mergeRequests); err != nil {
		return nil, err
	}

	return mergeRequests, nil
}
