package utils

import (
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
