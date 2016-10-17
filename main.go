package main

import (
	"flag"
	"log"
	"os"

	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/xanzy/go-gitlab"
)

/******************************************************************************/

func main() {
	url := flag.String("url", os.Getenv("GITLAB_URL"), "GitLab URL")
	token := flag.String("token", os.Getenv("GITLAB_PRIVATE_TOKEN"), "GitLab private token")
	flag.Parse()

	if len(flag.Args()) < 1 {
		log.Fatal("Usage: gitlab-artifacts-fuse mountpoint")
	}
	if *url == "" {
		log.Fatal("GitLab URL not set (via GITLAB_URL or -url)")
	}
	if *token == "" {
		log.Fatal("GitLab token not set (via GITLAB_PRIVATE_TOKEN or -token)")
	}

	// Create GitLab client
	git := gitlab.NewClient(nil, *token)
	git.SetBaseURL(*url)

	fs := NewGitlabFs(git)

	server, _, err := nodefs.MountRoot(flag.Arg(0), fs.Root(), nil)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}
	server.Serve()
}
