package main

import "gitlab-mr-combiner/server"

func main() {
	server := server.NewServer()
	server.Init()
}
