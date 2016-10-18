package main

import (
	"io/ioutil"
	"log"
	"strconv"

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

			if prj.BuildsEnabled {
				prjInode.NewChild("builds", true,
					&projectBuildsNode{
						Node:  nodefs.NewDefaultNode(),
						fs:    fs,
						prjID: prj.ID,
					})
			}
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
	if n.fs.debug {
		log.Printf("projectDescNode.Open(%d)\n", n.prjID)
	}
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
	if file != nil {
		return file.GetAttr(out)
	}
	out.Mode = fuse.S_IFREG | 0444
	return fuse.OK
}

/******************************************************************************/
/* Project builds */

type projectBuildsNode struct {
	nodefs.Node
	fs    *GitlabFs
	prjID int
}

func (n *projectBuildsNode) fetch() bool {
	// Look up this project's info
	prj, _, err := n.fs.client.Projects.GetProject(n.prjID)
	if err != nil {
		log.Printf("GetProject(%d) error: %v\n", n.prjID, err)
		return false
	}

	if !prj.BuildsEnabled {
		// TODO: ENOENT?
		return true
	}

	// Get all of the builds from the API
	blds, _, err := n.fs.client.Builds.ListProjectBuilds(prj.ID, nil)
	if err != nil {
		log.Printf("ListProjectBuilds() error: %v\n", prj.PathWithNamespace, err)
		return false
	}

	// Get a map of all existing build inodes
	existing := n.Inode().Children()

	// Add new ones
	for _, bld := range blds {
		bldName := strconv.Itoa(bld.ID)

		_, exists := existing[bldName]
		if !exists {
			n.addNewBuildDirNode(&bld)
			continue
		}
	}

	// TODO: Remove ones that no longer exist -- Can this even happen with GitLab?

	return true
}

func (n *projectBuildsNode) addNewBuildDirNode(bld *gitlab.Build) {
	if n.fs.debug {
		log.Printf("Adding new build inode (%d) to project (%d)\n", bld.ID, n.prjID)
	}

	fs := n.fs
	prjID := n.prjID
	bldID := bld.ID
	bldName := strconv.Itoa(bld.ID)

	// Add the builds/1234 directory
	bldDirInode := n.Inode().NewChild(bldName, true, &buildDirNode{
		Node:  nodefs.NewDefaultNode(),
		fs:    fs,
		prjID: prjID,
		bldID: bldID,
	})

	// Add the builds/1234/xxx files
	bldDirInode.NewChild("status", false, &buildStatusNode{
		Node:  nodefs.NewDefaultNode(),
		fs:    fs,
		prjID: prjID,
		bldID: bldID,
	})
	bldDirInode.NewChild("trace", false, &buildTraceNode{
		Node:  nodefs.NewDefaultNode(),
		fs:    fs,
		prjID: prjID,
		bldID: bldID,
	})
	bldDirInode.NewChild(bld.ArtifactsFile.Filename, false, &buildArtifactsArchiveNode{
		Node:  nodefs.NewDefaultNode(),
		fs:    fs,
		prjID: prjID,
		bldID: bldID,
		size:  uint64(bld.ArtifactsFile.Size),
	})

}

func (n *projectBuildsNode) OpenDir(context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	if n.fs.debug {
		log.Printf("projectBuildsNode.OpenDir(%d)\n", n.prjID)
	}

	if !n.fetch() {
		return nil, fuse.EIO
	}

	return n.Node.OpenDir(context)
}

func (n *projectBuildsNode) Lookup(out *fuse.Attr, name string, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	if n.fs.debug {
		log.Printf("projectBuildsNode.Lookup(%q)\n", name)
	}

	if !n.fetch() {
		return nil, fuse.EIO
	}
	ch := n.Inode().GetChild(name)
	if ch == nil {
		return nil, fuse.ENOENT
	}

	return ch, ch.Node().GetAttr(out, nil, context)
}

/******************************************************************************/
/* <build_id> directory */

type buildDirNode struct {
	nodefs.Node
	fs    *GitlabFs
	prjID int
	bldID int
}

/******************************************************************************/
/* builds/<id>/status */

type buildStatusNode struct {
	nodefs.Node
	fs    *GitlabFs
	prjID int
	bldID int
}

func (n *buildStatusNode) GetAttr(out *fuse.Attr, file nodefs.File, context *fuse.Context) fuse.Status {
	if file != nil {
		return file.GetAttr(out)
	}
	out.Mode = fuse.S_IFREG | 0444
	return fuse.OK
}

func (n *buildStatusNode) Open(flags uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}
	bld, _, err := n.fs.client.Builds.GetSingleBuild(n.prjID, n.bldID)
	if err != nil {
		log.Printf("GetSingleBuild(%d, %d) error: %v\n", n.prjID, n.bldID, err)
		return nil, fuse.EIO
	}
	return nodefs.NewDataFile([]byte(bld.Status + "\n")), fuse.OK
}

/******************************************************************************/
/* builds/<id>/trace */

type buildTraceNode struct {
	nodefs.Node
	fs    *GitlabFs
	prjID int
	bldID int
}

func (n *buildTraceNode) GetAttr(out *fuse.Attr, file nodefs.File, context *fuse.Context) fuse.Status {
	if file != nil {
		return file.GetAttr(out)
	}
	out.Mode = fuse.S_IFREG | 0444
	return fuse.OK
}

func (n *buildTraceNode) Open(flags uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}

	traceReader, _, err := n.fs.client.Builds.GetTraceFile(n.prjID, n.bldID)
	if err != nil {
		log.Printf("GetTraceFile(%d, %d) error: %v\n", n.prjID, n.bldID, err)
		return nil, fuse.EIO
	}

	traceBuf, err := ioutil.ReadAll(traceReader)
	if err != nil {
		log.Printf("ReadAll error: %v\n", err)
		return nil, fuse.EIO
	}

	return nodefs.NewDataFile(traceBuf), fuse.OK
}

/******************************************************************************/
/* builds/<id>/<artifacts_archive_name> */

type buildArtifactsArchiveNode struct {
	nodefs.Node
	fs    *GitlabFs
	prjID int
	bldID int
	size  uint64
}

func (n *buildArtifactsArchiveNode) GetAttr(out *fuse.Attr, file nodefs.File, context *fuse.Context) fuse.Status {
	if file != nil {
		return file.GetAttr(out)
	}
	out.Mode = fuse.S_IFREG | 0444
	out.Size = n.size
	return fuse.OK
}

func (n *buildArtifactsArchiveNode) Open(flags uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}

	artReader, _, err := n.fs.client.Builds.GetBuildArtifacts(n.prjID, n.bldID)
	if err != nil {
		log.Printf("GetBuildArtifacts(%d, %d) error: %v\n", n.prjID, n.bldID, err)
		return nil, fuse.EIO
	}

	artBuf, err := ioutil.ReadAll(artReader)
	if err != nil {
		log.Printf("ReadAll error: %v\n", err)
		return nil, fuse.EIO
	}

	n.size = uint64(len(artBuf))
	return nodefs.NewDataFile(artBuf), fuse.OK
}
