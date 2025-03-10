package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"gitlab-mr-combiner/config"
	"gitlab-mr-combiner/gitlab"
	"gitlab-mr-combiner/utils"

	log "github.com/sirupsen/logrus"
)

type Server struct {
	apiClient      *gitlab.ApiClient
	activeProjects sync.Map
	commentsBuffer sync.Map
}

func NewServer() *Server {
	return &Server{
		apiClient: gitlab.NewApiClient(),
	}
}

func (s *Server) Init() {
	config.ValidateEnvVars()
	utils.InitGitConfig()
	utils.InitLogger()

	http.HandleFunc("/", s.handleWebhook)
	log.Println("Server is running on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	var event struct {
		ProjectID  int    `json:"project_id"`
		EventType  string `json:"event_type"`
		ObjectAttr struct {
			Action string `json:"action"`
			Note   string `json:"note"`
		} `json:"object_attributes"`
		MergeRequest struct {
			IID int `json:"iid"`
		} `json:"merge_request"`
	}

	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		utils.RespondWithJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if event.EventType != "note" || event.ObjectAttr.Action != "create" || event.ObjectAttr.Note != config.TriggerMessage {
		utils.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Event ignored"})
		return
	}

	if s.isProjectActive(event.ProjectID) {
		utils.RespondWithJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Project is already being processed"})
		return
	}

	if config.SecretToken != "" && r.Header.Get("X-Gitlab-Token") != config.SecretToken {
		s.activeProjects.Delete(event.ProjectID)
		utils.RespondWithJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid token"})
		return
	}
	s.activeProjects.Store(event.ProjectID, struct{}{})

	s.commentsBuffer = sync.Map{}

	go func() {
		defer func() {
			s.activeProjects.Delete(event.ProjectID)
		}()
		s.combineAllMRs(event.ProjectID, event.MergeRequest.IID, r)
	}()

	utils.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "OK"})
}

func (s *Server) isProjectActive(projectID int) bool {
	_, ok := s.activeProjects.Load(projectID)
	return ok
}

func (s *Server) getRepoInfo(projectID int) (*gitlab.RepoInfo, error) {
	data, err := s.apiClient.Send("GET", fmt.Sprintf("/projects/%d", projectID), nil)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("empty response from GitLab API")
	}

	var repo gitlab.RepoInfo
	if err := json.Unmarshal(data, &repo); err != nil {
		return nil, err
	}

	return &repo, nil
}
