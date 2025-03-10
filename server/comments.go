package server

import (
	"fmt"
	"gitlab-mr-combiner/config"
	"gitlab-mr-combiner/utils"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
)

func (s *Server) addCommentToBuffer(mergeRequestID int, comment string) {
	comments, _ := s.commentsBuffer.LoadOrStore(mergeRequestID, []string{})
	comments = append(comments.([]string), comment)
	s.commentsBuffer.Store(mergeRequestID, comments)
	log.Info(comment)
}

func (s *Server) sendComments(projectID, mergeRequestID int, hasError bool, r *http.Request) {
	comments, ok := s.commentsBuffer.Load(mergeRequestID)
	targetBranch := utils.GetQueryParam("branch", config.TargetBranch, r)
	if !hasError {
		hasError = true
	}
	if !ok {
		log.Warnf("No comments found for MR #%d", mergeRequestID)
		return
	}

	commentList := comments.([]string)
	combinedComment := strings.Join(commentList, "\n")
	var message string
	if hasError {
		message = "An error occurred during rebase into " + targetBranch
	} else {
		message = "Merge Requests were merged into " + targetBranch
	}

	if err := s.createCommentOnMR(projectID, mergeRequestID, combinedComment, message); err != nil {
		log.Errorf("Failed to add comment: %v", err)
	}

	s.commentsBuffer.Delete(mergeRequestID)
}

func (s *Server) createCommentOnMR(projectID, mergeRequestID int, comment string, beforeCommentMessage string) error {
	formattedComment := fmt.Sprintf("%s\n```\n%s\n```", beforeCommentMessage, comment)

	_, err := s.apiClient.Send("POST", fmt.Sprintf("/projects/%d/merge_requests/%d/notes", projectID, mergeRequestID), map[string]string{"body": formattedComment})
	if err != nil {
		log.Errorf("Failed to add comment: %v", err)
		return err
	}
	log.Infof("Comment added to MR #%d", mergeRequestID)
	return nil
}
