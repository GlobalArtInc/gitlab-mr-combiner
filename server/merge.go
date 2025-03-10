package server

import (
	"encoding/json"
	"fmt"
	"gitlab-mr-combiner/config"
	"gitlab-mr-combiner/gitlab"
	"gitlab-mr-combiner/utils"
	"net/http"
	"os/exec"

	log "github.com/sirupsen/logrus"
)

func (s *Server) combineAllMRs(projectID, mergeRequestID int, r *http.Request) {
	log.Println("Processing MRs for project:", projectID)
	targetBranch := utils.GetQueryParam("branch", config.TargetBranch, r)
	repoInfo, err := s.getRepoInfo(projectID)
	if err != nil {
		s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Error fetching repo info: %v", err))
		s.sendComments(projectID, mergeRequestID, true, r)
		return
	}

	s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Repo Info: Branch=%s, URL=%s", repoInfo.DefaultBranch, repoInfo.RepoURL))

	clonePath := fmt.Sprintf("/gitlab-combiner/project-%d", projectID)
	hasError := false

	if err := exec.Command("rm", "-rf", clonePath).Run(); err != nil {
		s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Failed to remove directory: %v", err))
	}

	if err := exec.Command("git", "clone", "--branch", repoInfo.DefaultBranch, repoInfo.RepoURL, clonePath).Run(); err != nil {
		s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Error cloning repo: %v", err))
		s.sendComments(projectID, mergeRequestID, true, r)
		return
	}
	defer exec.Command("rm", "-rf", clonePath).Run()

	if targetBranch == repoInfo.DefaultBranch {
		s.addCommentToBuffer(mergeRequestID, "Target branch is the same as the default branch")
		s.sendComments(projectID, mergeRequestID, true, r)
		return
	}

	if err := exec.Command("git", "-C", clonePath, "checkout", "-b", targetBranch).Run(); err != nil {
		s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Error creating branch: %v", err))
		s.sendComments(projectID, mergeRequestID, true, r)
		return
	}

	mergeRequests, err := s.fetchMergeRequests(projectID)
	if err != nil {
		s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Error fetching MRs: %v", err))
		s.sendComments(projectID, mergeRequestID, true, r)
		return
	}

	s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Found %d MRs", len(mergeRequests)))

	for _, mr := range mergeRequests {
		if err := exec.Command("git", "-C", clonePath, "fetch", "origin", fmt.Sprintf("merge-requests/%d/head:mr-%d", mr.IID, mr.IID)).Run(); err != nil {
			s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Error fetching MR #%d: %v", mr.IID, err))
			hasError = true
			continue
		}

		if err := exec.Command("git", "-C", clonePath, "checkout", targetBranch).Run(); err != nil {
			log.Printf("Error checking out branch: %v", err)
			s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Error checking out branch: %v", err))
			hasError = true
			continue
		}

		if err := exec.Command("git", "-C", clonePath, "merge", "--no-ff", fmt.Sprintf("mr-%d", mr.IID)).Run(); err != nil {
			log.Printf("Error merging MR #%d: %v", mr.IID, err)
			s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Error merging MR #%d: %v", mr.IID, err))
			hasError = true
			continue
		}

		s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Merged MR #%d: %s", mr.IID, mr.Title))
	}

	if err := exec.Command("git", "-C", clonePath, "push", "origin", targetBranch, "--force").Run(); err != nil {
		s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Error pushing to remote: %v", err))
		s.sendComments(projectID, mergeRequestID, true, r)
		return
	}
	s.addCommentToBuffer(mergeRequestID, fmt.Sprintf("Merged MRs into %s", targetBranch))
	s.sendComments(projectID, mergeRequestID, hasError, r)
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
