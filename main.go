package main

import "gitlab-mr-combiner/internal/server"

func main() {
	server := server.NewServer()
	server.Init()
}
