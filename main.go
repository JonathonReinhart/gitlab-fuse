package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/JonathonReinhart/gitlab-fuse/gitlabfs"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/xanzy/go-gitlab"
)

/******************************************************************************/

// Wait for SIGINT in the background and unmount ourselves if we get it.
// This prevents a dangling "Transport endpoint is not connected"
// mountpoint if the user hits CTRL-C.
// https://github.com/rfjakob/gocryptfs/blob/master/mount.go
func handleSigint(srv *fuse.Server, mountpoint string) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	signal.Notify(ch, syscall.SIGTERM)
	go func() {
		<-ch
		log.Print("Unmounting...")
		err := srv.Unmount()
		if err != nil {
			log.Print(err)
			log.Print("Trying lazy unmount")
			cmd := exec.Command("fusermount", "-u", "-z", mountpoint)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()
		}
		os.Exit(1)
	}()
}

func main() {
	// Parse arguments
	url := flag.String("url", os.Getenv("GITLAB_URL"), "GitLab URL")
	token := flag.String("token", os.Getenv("GITLAB_PRIVATE_TOKEN"), "GitLab private token")
	debug := flag.Bool("debug", false, "Enable debug logging")
	fusedebug := flag.Bool("fusedebug", false, "Enable FUSE debug logging")
	flag.Parse()

	if len(flag.Args()) < 1 {
		log.Fatal("Usage: gitlab-fuse mountpoint")
	}
	if *url == "" {
		log.Fatal("GitLab URL not set (via GITLAB_URL or -url)")
	}
	if *token == "" {
		log.Fatal("GitLab token not set (via GITLAB_PRIVATE_TOKEN or -token)")
	}
	mountpoint := flag.Arg(0)

	// Create GitLab client
	git := gitlab.NewClient(nil, *token)
	git.SetBaseURL(*url)

	// Create GitlabFs
	fs := gitlabfs.NewGitlabFs(git)
	fs.SetDebug(*debug)

	// Create FS connector
	opts := nodefs.NewOptions()
	opts.Debug = *fusedebug
	conn := nodefs.NewFileSystemConnector(fs.Root(), opts)

	// Create the FUSE server
	mntOpts := &fuse.MountOptions{
		Debug:  *fusedebug,
		FsName: *url,
		Name:   "gitlab",
	}
	server, err := fuse.NewServer(conn.RawFS(), mountpoint, mntOpts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}

	// Run!
	handleSigint(server, mountpoint)
	server.Serve()
}
