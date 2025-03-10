package utils

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"

	"gitlab-mr-combiner/config"

	log "github.com/sirupsen/logrus"
)

func InitLogger() {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
		FullTimestamp:   true,
		DisableColors:   true,
	})
	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)
}

func InitGitConfig() {
	commands := [][]string{
		{"git", "config", "--global", "user.email", config.GitEmail},
		{"git", "config", "--global", "user.name", config.GitUser},
	}

	for _, cmd := range commands {
		if err := exec.Command(cmd[0], cmd[1:]...).Run(); err != nil {
			log.Fatalf("Failed to run command: %s", cmd)
		}
	}
}

func RespondWithJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Errorf("Failed to encode JSON response: %v", err)
	}
}

func GetQueryParam(key string, defaultValue string, r *http.Request) string {
	query := r.URL.Query()
	if val, ok := query[key]; ok {
		return val[0]
	}
	return defaultValue
}
