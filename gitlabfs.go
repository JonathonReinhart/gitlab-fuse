package main

import (
	//"io/ioutil"
	"log"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"

	"github.com/xanzy/go-gitlab"
)

/**
 * Paths are composed like this:
 * <namespace>/
 *    <project>/
 *        builds/
 *            <build_id>/
 *                status
 *                trace
 *                artifacts/
 *                    individual.bin
 *                    artifacts.bin
 *                <artifacts_filename>
 */

/******************************************************************************/
/* GitlabFs */

type GitlabFs struct {
	client *gitlab.Client
	root   *rootInode //nodefs.Node
	debug  bool
}

func NewGitlabFs(client *gitlab.Client) *GitlabFs {
	// See go-ufse/zipfs/memtree.go NewMemTreeFs()
	fs := &GitlabFs{
		root:   &rootInode{Node: nodefs.NewDefaultNode()},
		client: client,
	}
	fs.root.fs = fs
	return fs
}

func (fs *GitlabFs) Root() nodefs.Node {
	return fs.root
}

func (fs *GitlabFs) SetDebug(debug bool) {
	fs.debug = debug
	if debug {
		log.Print("Debugging enabled")
	}
}

/******************************************************************************/
/* rootInode */

type rootInode struct {
	// http://www.hydrogen18.com/blog/golang-embedding.html
	nodefs.Node

	// Back-reference to our overall fs object
	fs *GitlabFs
}

func (r *rootInode) Lookup(out *fuse.Attr, name string, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	if r.fs.debug {
		log.Printf("Lookup(%q)\n", name)
	}
	return nil, fuse.ENOSYS
}

/*
func (r *rootInode) OpenDir(context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	return []fuse.DirEntry{
		{Name: "aaa"},
		{Name: "bbb"},
	}, fuse.OK

	return nil, fuse.ENOENT
}
*/
