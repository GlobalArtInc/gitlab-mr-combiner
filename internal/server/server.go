package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"gitlab-mr-combiner/internal/config"
	"gitlab-mr-combiner/internal/gitlab"
	"gitlab-mr-combiner/internal/utils"

	log "github.com/sirupsen/logrus"
)

type Server struct {
	apiClient      *gitlab.ApiClient
	activeProjects sync.Map
	commentsBuffer sync.Map
}

type WebhookEvent struct {
	ObjectKind   string          `json:"object_kind"`
	EventType    string          `json:"event_type"`
	ProjectID    int             `json:"project_id"`
	ObjectAttr   json.RawMessage `json:"object_attributes"`
	MergeRequest json.RawMessage `json:"merge_request"`
	Labels       []struct {
		Title string `json:"title"`
	} `json:"labels"`
}

type NoteEventAttr struct {
	Action      string `json:"action"`
	Note        string `json:"note"`
	NoteableID  int    `json:"noteable_id"`
	NotableType string `json:"noteable_type"`
	ProjectID   int    `json:"project_id"`
}

type MREventAttr struct {
	Action string `json:"action"`
	IID    int    `json:"iid"`
	Labels []struct {
		Title     string `json:"title"`
		ProjectID int    `json:"project_id"`
	} `json:"labels"`
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
	log.Printf("Server is running on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	var event WebhookEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	projectID, mergeRequestIID, isValidEvent := s.validateEvent(event)
	if !isValidEvent {
		s.respondWithMessage(w, "Event ignored")
		return
	}

	if err := s.processWebhookEvent(w, r, projectID, mergeRequestIID); err != nil {
		log.Errorf("Error processing webhook: %v", err)
		s.respondWithError(w, http.StatusInternalServerError, "Failed to process event")
	}
}

func (s *Server) validateEvent(event WebhookEvent) (int, int, bool) {
	switch event.EventType {
	case "note":
		var noteAttr NoteEventAttr
		if err := json.Unmarshal(event.ObjectAttr, &noteAttr); err != nil {
			return 0, 0, false
		}
		if noteAttr.Action == "create" && noteAttr.Note == config.TriggerMessage &&
			noteAttr.NotableType == "MergeRequest" {
			var mergeRequest struct {
				IID int `json:"iid"`
			}
			if err := json.Unmarshal(event.MergeRequest, &mergeRequest); err != nil {
				return 0, 0, false
			}
			fmt.Println("noteAttr.ProjectID", mergeRequest.IID)
			return noteAttr.ProjectID, mergeRequest.IID, true
		}

	case "merge_request":
		var mrAttr MREventAttr
		if err := json.Unmarshal(event.ObjectAttr, &mrAttr); err != nil {
			return 0, 0, false
		}
		for _, label := range mrAttr.Labels {
			if label.Title == config.TriggerTag {
				return label.ProjectID, mrAttr.IID, true
			}
		}
	}

	return 0, 0, false
}

func (s *Server) processWebhookEvent(w http.ResponseWriter, r *http.Request, projectID, mergeRequestIID int) error {
	if s.isProjectActive(projectID) {
		s.respondWithError(w, http.StatusTooManyRequests, "Project is already being processed")
		return fmt.Errorf("project %d is already active", projectID)
	}

	if err := s.validateSecretToken(r, projectID); err != nil {
		return err
	}

	s.activeProjects.Store(projectID, struct{}{})
	s.commentsBuffer = sync.Map{}

	go func() {
		defer s.activeProjects.Delete(projectID)
		s.combineAllMRs(projectID, mergeRequestIID, r)
	}()

	s.respondWithMessage(w, "OK")
	return nil
}

func (s *Server) validateSecretToken(r *http.Request, projectID int) error {
	if config.SecretToken != "" && r.Header.Get("X-Gitlab-Token") != config.SecretToken {
		s.activeProjects.Delete(projectID)
		return fmt.Errorf("invalid secret token")
	}
	return nil
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

func (s *Server) GetQueryParam(key, defaultValue string, r *http.Request) string {
	if val := r.URL.Query().Get(key); val != "" {
		return val
	}
	return defaultValue
}

func (s *Server) respondWithError(w http.ResponseWriter, statusCode int, message string) {
	s.RespondWithJSON(w, statusCode, map[string]string{"error": message})
}

func (s *Server) respondWithMessage(w http.ResponseWriter, message string) {
	s.RespondWithJSON(w, http.StatusOK, map[string]string{"message": message})
}

func (s *Server) RespondWithJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Errorf("Failed to encode JSON response: %v", err)
	}
}
