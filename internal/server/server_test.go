package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab-mr-combiner/internal/config"
)

func TestValidateEvent(t *testing.T) {
	s := NewServer()

	testCases := []struct {
		name           string
		event          WebhookEvent
		expectedResult bool
		expectedProjID int
		expectedMRIID  int
	}{
		{
			name: "Valid Note Event",
			event: WebhookEvent{
				EventType:  "note",
				ObjectAttr: json.RawMessage(`{"action": "create", "note": "` + config.TriggerMessage + `", "noteable_type": "MergeRequest", "project_id": 123, "noteable_id": 456}`),
			},
			expectedResult: true,
			expectedProjID: 123,
			expectedMRIID:  456,
		},
		{
			name: "Valid MR Event with Trigger Tag",
			event: WebhookEvent{
				EventType:  "merge_request",
				ObjectAttr: json.RawMessage(`{"action": "open", "iid": 789, "labels": [{"title": "` + config.TriggerTag + `", "project_id": 321}]}`),
			},
			expectedResult: true,
			expectedProjID: 321,
			expectedMRIID:  789,
		},
		{
			name: "Invalid Note Event",
			event: WebhookEvent{
				EventType:  "note",
				ObjectAttr: json.RawMessage(`{"action": "create", "note": "Wrong message", "noteable_type": "MergeRequest"}`),
			},
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			projID, mrIID, result := s.validateEvent(tc.event)
			if result != tc.expectedResult {
				t.Errorf("Expected result %v, got %v", tc.expectedResult, result)
			}
			if result {
				if projID != tc.expectedProjID {
					t.Errorf("Expected project ID %d, got %d", tc.expectedProjID, projID)
				}
				if mrIID != tc.expectedMRIID {
					t.Errorf("Expected merge request IID %d, got %d", tc.expectedMRIID, mrIID)
				}
			}
		})
	}
}

func TestValidateSecretToken(t *testing.T) {
	s := NewServer()
	config.SecretToken = "test-secret"

	testCases := []struct {
		name          string
		token         string
		expectedError bool
		projectID     int
	}{
		{
			name:          "Valid Secret Token",
			token:         "test-secret",
			expectedError: false,
			projectID:     123,
		},
		{
			name:          "Invalid Secret Token",
			token:         "wrong-token",
			expectedError: true,
			projectID:     456,
		},
		{
			name:          "Empty SecretToken Configuration",
			token:         "",
			expectedError: false,
			projectID:     789,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/webhook", nil)
			req.Header.Set("X-Gitlab-Token", tc.token)

			err := s.validateSecretToken(req, tc.projectID)
			if tc.expectedError && err == nil {
				t.Errorf("Expected an error, got nil")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	}
}

func TestHandleWebhook(t *testing.T) {
	s := NewServer()
	config.TriggerMessage = "combine mr"
	config.TriggerTag = "mr-combine"

	testCases := []struct {
		name           string
		eventJSON      string
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "Valid Note Event",
			eventJSON: `{
				"object_kind": "note",
				"event_type": "note",
				"project_id": 123,
				"object_attributes": {
					"action": "create",
					"note": "combine mr",
					"noteable_type": "MergeRequest",
					"noteable_id": 456,
					"project_id": 123
				}
			}`,
			expectedStatus: http.StatusOK,
			expectedBody:   `{"message":"OK"}`,
		},
		{
			name: "Invalid JSON",
			eventJSON: `{
				"invalid": "json"
			}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Invalid request body"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/webhook", bytes.NewBufferString(tc.eventJSON))
			w := httptest.NewRecorder()

			s.handleWebhook(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, w.Code)
			}

			var response map[string]string
			json.Unmarshal(w.Body.Bytes(), &response)
			actualBody, _ := json.Marshal(response)

			if string(actualBody) != tc.expectedBody {
				t.Errorf("Expected body %s, got %s", tc.expectedBody, string(actualBody))
			}
		})
	}
}

func TestIsProjectActive(t *testing.T) {
	s := NewServer()

	testCases := []struct {
		name       string
		projectID  int
		beforeTest func(s *Server)
		expected   bool
	}{
		{
			name:      "Project Not Active",
			projectID: 123,
			expected:  false,
		},
		{
			name:      "Project Active",
			projectID: 456,
			beforeTest: func(s *Server) {
				s.activeProjects.Store(456, struct{}{})
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.beforeTest != nil {
				tc.beforeTest(s)
			}

			result := s.isProjectActive(tc.projectID)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}
