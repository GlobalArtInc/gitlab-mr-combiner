package config

import (
	"log"
	"os"
)

var (
	TriggerMessage = getEnv("TRIGGER_MESSAGE", "")
	TriggerTag     = getEnv("TRIGGER_TAG", "")
	TargetBranch   = getEnv("TARGET_BRANCH", "develop")
	GitlabToken    = getEnv("GITLAB_TOKEN", "")
	GitlabURL      = getEnv("GITLAB_URL", "https://gitlab.com/")
	GitEmail       = getEnv("GIT_EMAIL", "vcs@example.com")
	GitUser        = getEnv("GIT_USER", "vcs")
	SecretToken    = getEnv("SECRET_TOKEN", "")
)

func ValidateEnvVars() {
	required := map[string]string{
		"TRIGGER_MESSAGE": TriggerMessage,
		"TRIGGER_TAG":     TriggerTag,
		"TARGET_BRANCH":   TargetBranch,
		"GITLAB_TOKEN":    GitlabToken,
		"GITLAB_URL":      GitlabURL,
	}

	for key, value := range required {
		if value == "" {
			log.Fatalf("Missing required env variable: %s", key)
		}
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
