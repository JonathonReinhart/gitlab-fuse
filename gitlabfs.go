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

/**
 * http://www.hydrogen18.com/blog/golang-embedding.html
 *
 * By embedding the result of NewDefaultNode() in our Node types, we get a lot
 * of stuff for free. See fuse/nodefs/api.go
 */

/******************************************************************************/
/* GitlabFs */

type GitlabFs struct {
	client *gitlab.Client
	root   *rootNode
	debug  bool
}

func NewGitlabFs(client *gitlab.Client) *GitlabFs {
	fs := &GitlabFs{
		client: client,
	}
	fs.root = NewRootNode(fs)
	return fs
}

func (fs *GitlabFs) Root() nodefs.Node {
	return fs.root
}

func (fs *GitlabFs) SetDebug(debug bool) {
	fs.debug = debug
}

func (fs *GitlabFs) onMount() {
	if fs.debug {
		log.Print("onMount()\n")
	}

	prjmap, err := GetAllVisibleProjects(fs.client)
	if err != nil {
		// TODO: Not sure how to make this error visible
		// Maybe this fetch should actually go somewhere
		// earlier, and then we just populate the inodes here.
		panic(err)
	}

	// Add namespaces to root
	for ns, projects := range prjmap {
		nsNode := &namespaceNode{
			Node: nodefs.NewDefaultNode(),
			fs:   fs,
			path: ns,
		}
		nsInode := fs.root.Inode().NewChild(ns, true, nsNode)

		// Add projects to namespace
		for _, prj := range projects {
			prjNode := &projectNode{
				Node: nodefs.NewDefaultNode(),
				fs:   fs,
				path: prj.Path,
			}
			prjInode := nsInode.NewChild(prj.Path, true, prjNode)

			// Add project contents to project
			prjInode.NewChild("description", false,
				&projectDescNode{
					Node:  nodefs.NewDefaultNode(),
					fs:    fs,
					prjID: prj.ID,
				})
		}
	}

}

/******************************************************************************/
/* rootNode */

type rootNode struct {
	nodefs.Node

	// Back-reference to our overall fs object
	fs *GitlabFs
}

func NewRootNode(fs *GitlabFs) *rootNode {
	root := &rootNode{
		Node: nodefs.NewDefaultNode(),
		fs:   fs,
	}

	return root
}

func (r *rootNode) OnMount(c *nodefs.FileSystemConnector) {
	r.fs.onMount()
}

func (r *rootNode) Lookup(out *fuse.Attr, name string, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	if r.fs.debug {
		log.Printf("rootNode.Lookup(%q)\n", name)
	}
	return nil, fuse.ENOENT
}

/******************************************************************************/
/* Namespace */

type namespaceNode struct {
	nodefs.Node
	fs   *GitlabFs
	path string
}

/******************************************************************************/
/* Project */

type projectNode struct {
	nodefs.Node
	fs   *GitlabFs
	path string
}

func (n *projectNode) Lookup(out *fuse.Attr, name string, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	if n.fs.debug {
		log.Printf("projectNode.Lookup(%q)\n", name)
	}
	return nil, fuse.ENOENT
}

/******************************************************************************/
/* Project description */

type projectDescNode struct {
	nodefs.Node
	fs    *GitlabFs
	prjID int
}

func (n *projectDescNode) Open(flags uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}
	prj, _, err := n.fs.client.Projects.GetProject(n.prjID)
	if err != nil {
		log.Printf("GetProject(%d) error: %v\n", n.prjID, err)
		return nil, fuse.EIO
	}
	return nodefs.NewDataFile([]byte(prj.Description + "\n")), fuse.OK
}

func (n *projectDescNode) GetAttr(out *fuse.Attr, file nodefs.File, context *fuse.Context) fuse.Status {
	prj, _, err := n.fs.client.Projects.GetProject(n.prjID)
	if err != nil {
		log.Printf("GetProject(%d) error: %v\n", n.prjID, err)
		return fuse.EIO
	}
	out.Mode = fuse.S_IFREG | 0444
	out.Size = uint64(len(prj.Description) + 1)
	return fuse.OK
}
