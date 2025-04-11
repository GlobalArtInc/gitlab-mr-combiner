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
	if _, err := os.Stat(clonePath); os.IsNotExist(err) {
		log.Infof("Repository directory not found, cloning repository to %s", clonePath)
		if err := exec.Command("git", "clone", "--branch", repoInfo.DefaultBranch, repoInfo.RepoURL, clonePath).Run(); err != nil {
			return fmt.Errorf("error cloning repo: %v", err)
		}
	} else {
		log.Infof("Repository directory exists, updating local repository")
		if err := s.updateLocalRepository(clonePath, repoInfo.DefaultBranch); err != nil {
			return err
		}
	}

	if err := s.prepareTargetBranch(clonePath, targetBranch); err != nil {
		return err
	}

	return nil
}

func (s *Server) updateLocalRepository(clonePath, defaultBranch string) error {
	if err := exec.Command("git", "-C", clonePath, "reset", "--hard").Run(); err != nil {
		return fmt.Errorf("error resetting repository: %v", err)
	}

	if err := exec.Command("git", "-C", clonePath, "fetch", "--all").Run(); err != nil {
		return fmt.Errorf("error fetching updates: %v", err)
	}

	if err := exec.Command("git", "-C", clonePath, "checkout", defaultBranch).Run(); err != nil {
		return fmt.Errorf("error checking out default branch: %v", err)
	}

	if err := exec.Command("git", "-C", clonePath, "pull", "origin", defaultBranch).Run(); err != nil {
		return fmt.Errorf("error pulling latest changes: %v", err)
	}

	statusCmd := exec.Command("git", "-C", clonePath, "status", "--porcelain")
	statusOutput, err := statusCmd.Output()
	if err != nil {
		return fmt.Errorf("error checking git status: %v", err)
	}

	if len(statusOutput) > 0 {
		log.Warn("Local changes detected, attempting to pull changes with force...")
		if err := exec.Command("git", "-C", clonePath, "fetch", "origin").Run(); err != nil {
			return fmt.Errorf("error fetching from remote: %v", err)
		}

		if err := exec.Command("git", "-C", clonePath, "reset", "--hard", "origin/"+defaultBranch).Run(); err != nil {
			return fmt.Errorf("error resetting to remote branch: %v", err)
		}
	}

	return nil
}

func (s *Server) prepareTargetBranch(clonePath, targetBranch string) error {
	checkBranchCmd := exec.Command("git", "-C", clonePath, "branch", "--list", targetBranch)
	branchOutput, err := checkBranchCmd.Output()
	if err != nil {
		return fmt.Errorf("error checking branch existence: %v", err)
	}

	if len(branchOutput) > 0 {
		if err := exec.Command("git", "-C", clonePath, "checkout", targetBranch).Run(); err != nil {
			return fmt.Errorf("error checking out existing target branch: %v", err)
		}
	} else {
		if err := exec.Command("git", "-C", clonePath, "checkout", "-b", targetBranch).Run(); err != nil {
			return fmt.Errorf("error creating target branch: %v", err)
		}
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

	if err := exec.Command("git", "-C", clonePath, "fetch", "origin", fmt.Sprintf("merge-requests/%d/head:%s", mr.IID, mrBranchName)).Run(); err != nil {
		s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Error fetching MR #%d: %v", mr.IID, err))
		return err
	}

	if err := exec.Command("git", "-C", clonePath, "checkout", targetBranch).Run(); err != nil {
		log.Printf("Error checking out branch: %v", err)
		s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Error checking out branch: %v", err))
		return err
	}

	if err := exec.Command("git", "-C", clonePath, "merge", "--no-ff", mrBranchName).Run(); err != nil {
		log.Printf("Error merging MR #%d: %v", mr.IID, err)
		s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Error merging MR #%d: %v", mr.IID, err))
		return err
	}

	s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Merged MR #%d: %s", mr.IID, mr.Title))
	return nil
}

func (s *Server) pushChanges(clonePath, targetBranch string) error {
	return exec.Command("git", "-C", clonePath, "push", "origin", targetBranch, "--force").Run()
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
