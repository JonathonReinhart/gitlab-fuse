package gitlabfs

import (
	"archive/zip"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"

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

func (fs *GitlabFs) DbgPrintf(format string, v ...interface{}) {
	if fs.debug {
		log.Printf(format, v...)
	}
}

func (fs *GitlabFs) onMount() {
	fs.DbgPrintf("onMount()\n")

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
	r.fs.DbgPrintf("rootNode.Lookup(%q)\n", name)
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
	n.fs.DbgPrintf("projectNode.Lookup(%q)\n", name)
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
	n.fs.DbgPrintf("projectDescNode.Open(%d)\n", n.prjID)
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
	blds, err := GetAllProjectBuilds(n.fs.client, prj.ID)
	if err != nil {
		log.Printf("GetAllProjectBuilds() error: %v\n", prj.PathWithNamespace, err)
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
	n.fs.DbgPrintf("Adding new build inode (%d) to project (%d)\n", bld.ID, n.prjID)

	fs := n.fs
	prjID := n.prjID
	bldID := bld.ID
	bldName := strconv.Itoa(bld.ID)

	// Add the builds/1234 directory
	bldDirInode := n.Inode().NewChild(bldName, true, nodefs.NewDefaultNode())

	// Add the builds/1234/xxx files
	bldDirInode.NewChild("status", false, &buildStatusNode{
		buildNode: NewBuildNode(fs, prjID, bldID),
	})
	bldDirInode.NewChild("trace", false, &buildTraceNode{
		buildNode: NewBuildNode(fs, prjID, bldID),
	})
	bldDirInode.NewChild(bld.ArtifactsFile.Filename, false, &buildArtifactsArchiveNode{
		buildNode: NewBuildNode(fs, prjID, bldID),
		size:      uint64(bld.ArtifactsFile.Size),
	})
	if bld.ArtifactsFile.Size > 0 {
		bldDirInode.NewChild("artifacts", true, NewBuildArtifactsDirNode(fs, prjID, bldID))
	}

}

func (n *projectBuildsNode) OpenDir(context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	n.fs.DbgPrintf("projectBuildsNode.OpenDir(%d)\n", n.prjID)

	if !n.fetch() {
		return nil, fuse.EIO
	}

	return n.Node.OpenDir(context)
}

func (n *projectBuildsNode) Lookup(out *fuse.Attr, name string, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	n.fs.DbgPrintf("projectBuildsNode.Lookup(%q)\n", name)

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

type buildNode struct {
	nodefs.Node
	fs    *GitlabFs
	prjID int
	bldID int
}

func NewBuildNode(fs *GitlabFs, prjID, bldID int) buildNode {
	return buildNode{
		Node:  nodefs.NewDefaultNode(),
		fs:    fs,
		prjID: prjID,
		bldID: bldID,
	}
}

func (n *buildNode) GetAttr(out *fuse.Attr, file nodefs.File, context *fuse.Context) fuse.Status {
	if file != nil {
		return file.GetAttr(out)
	}
	out.Mode = fuse.S_IFREG | 0444
	return fuse.OK
}

/******************************************************************************/
/* builds/<id>/status */

type buildStatusNode struct {
	buildNode
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
	buildNode
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
	buildNode
	size uint64
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

/******************************************************************************/
/* builds/<id>/artifacts/ */

type buildArtifactsDirNode struct {
	nodefs.Node
	fs    *GitlabFs
	prjID int
	bldID int

	zipr *ZipFileReader
}

func NewBuildArtifactsDirNode(fs *GitlabFs, prjID, bldID int) *buildArtifactsDirNode {
	return &buildArtifactsDirNode{
		Node:  nodefs.NewDefaultNode(),
		prjID: prjID,
		bldID: bldID,
		fs:    fs,
	}
}

func (n *buildArtifactsDirNode) getArchive() (*os.File, error) {
	n.fs.DbgPrintf("Getting artifact archive for prjID=%d, bldID=%d\n", n.prjID, n.bldID)

	// Get its name
	bld, _, err := n.fs.client.Builds.GetSingleBuild(n.prjID, n.bldID)
	if err != nil {
		log.Printf("GetSingleBuild(prjID=%d bldID=%d) failed: %v\n", n.prjID, n.bldID, err)
		return nil, err
	}
	filename := bld.ArtifactsFile.Filename

	if !strings.HasSuffix(filename, ".zip") {
		return nil, errors.New("Only zip files are supported")
	}

	// Download the artifact
	artReader, _, err := n.fs.client.Builds.GetBuildArtifacts(n.prjID, n.bldID)
	if err != nil {
		log.Printf("GetBuildArtifacts(prjID=%d bldID=%d) failed: %v\n", n.prjID, n.bldID, err)
		return nil, err
	}

	f, err := UnlinkedTempFile("", "gitlab-fuse-artifact")
	if err != nil {
		log.Printf("UnlinkedTempFile() failed: %v\n", err)
		return nil, err
	}

	_, err = io.Copy(f, artReader)
	if err != nil {
		log.Printf("Copying artifact archive failed: %v\n", err)
		return nil, err
	}

	f.Seek(0, os.SEEK_SET)
	return f, nil
}

func (n *buildArtifactsDirNode) fetch() bool {
	if n.zipr != nil {
		return true
	}

	archf, err := n.getArchive()
	if err != nil {
		return false
	}

	n.zipr, err = ZipReaderFromFile(archf)
	if err != nil {
		log.Printf("zip.NewReader() failed: %v\n", err)
		archf.Close()
		return false
	}

	for _, f := range n.zipr.File {
		n.fs.DbgPrintf("   %q\n", f.Name)

		node := &buildArtifactNode{
			Node: nodefs.NewDefaultNode(),
			f:    f,
		}

		n.Inode().NewChild(f.Name, false, node)
	}

	return true
}

func (n *buildArtifactsDirNode) OpenDir(context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	n.fs.DbgPrintf("buildArtifactsDirNode.OpenDir() (prjID=%d bldID=%d)\n", n.prjID, n.bldID)

	if !n.fetch() {
		return nil, fuse.EIO
	}

	return n.Node.OpenDir(context)
}

func (n *buildArtifactsDirNode) Lookup(out *fuse.Attr, name string, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	n.fs.DbgPrintf("buildArtifactsDirNode.Lookup(%q) (prjID=%d bldID=%d)\n", name, n.prjID, n.bldID)

	if !n.fetch() {
		return nil, fuse.EIO
	}
	ch := n.Inode().GetChild(name)
	if ch == nil {
		return nil, fuse.ENOENT
	}

	return ch, ch.Node().GetAttr(out, nil, context)
}

/*****/

type buildArtifactNode struct {
	nodefs.Node
	f *zip.File
}

func (n *buildArtifactNode) GetAttr(out *fuse.Attr, file nodefs.File, context *fuse.Context) fuse.Status {
	if file != nil {
		return file.GetAttr(out)
	}
	out.Mode = fuse.S_IFREG | 0444
	out.Size = n.f.UncompressedSize64
	return fuse.OK
}

func (n *buildArtifactNode) Open(flags uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}

	// Open the file from the zip archive
	rc, err := n.f.Open()
	if err != nil {
		log.Printf("zip.File.Open() failed: %v\n", err)
		return nil, fuse.EIO
	}
	defer rc.Close()

	buf, err := ioutil.ReadAll(rc)
	if err != nil {
		log.Printf("ReadAll error: %v\n", err)
		return nil, fuse.EIO
	}

	return nodefs.NewDataFile(buf), fuse.OK
}
