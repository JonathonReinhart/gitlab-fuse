package gitlabfs

import (
	"io"
	"io/ioutil"
	"log"
	"time"

	"github.com/xanzy/go-gitlab"
)

type GitlabClient struct {
	*gitlab.Client
	debug *log.Logger
}

func NewGitlabClient(client *gitlab.Client) *GitlabClient {
	return &GitlabClient{
		Client: client,
		debug:  log.New(ioutil.Discard, "GITLAB: ", log.Lshortfile|log.LstdFlags),
	}
}

func (git *GitlabClient) SetDebugLogOutput(w io.Writer) {
	git.debug.SetOutput(w)
}

// GetAllVisibleProjects returns a map of namespace to a list of Projects in that namespace.
func (git *GitlabClient) getAllVisibleProjects() (map[string][]*gitlab.Project, error) {
	result := make(map[string][]*gitlab.Project)

	opt := gitlab.ListProjectsOptions{
		ListOptions: gitlab.ListOptions{
			Page:    1,
			PerPage: 100,
		},
	}

	for {
		prj, resp, err := git.Projects.ListProjects(&opt)
		if err != nil {
			return nil, err
		}

		// Store these projects in the map
		for _, p := range prj {
			namespace := p.Namespace.Path
			result[namespace] = append(result[namespace], p)
		}

		// Go to the next page
		if resp.NextPage == 0 {
			break
		}
		opt.ListOptions.Page = resp.NextPage
	}

	return result, nil
}

func (git *GitlabClient) GetAllVisibleProjects() (map[string][]*gitlab.Project, error) {
	t0 := time.Now()
	result, err := git.getAllVisibleProjects()
	dt := time.Now().Sub(t0)

	git.debug.Printf("GetAllVisibleProjects() => %d records in %v\n", len(result), dt)
	return result, err
}

func (git *GitlabClient) getAllProjectBuilds(pid interface{}) ([]gitlab.Build, error) {
	result := make([]gitlab.Build, 0)

	opt := gitlab.ListBuildsOptions{
		ListOptions: gitlab.ListOptions{
			Page:    1,
			PerPage: 100,
		},
	}

	for {
		builds, resp, err := git.Builds.ListProjectBuilds(pid, &opt)
		if err != nil {
			return nil, err
		}

		result = append(result, builds...)

		// Go to the next page
		if resp.NextPage == 0 {
			break
		}
		opt.ListOptions.Page = resp.NextPage
	}

	return result, nil
}

func (git *GitlabClient) GetAllProjectBuilds(pid interface{}) ([]gitlab.Build, error) {
	t0 := time.Now()
	result, err := git.getAllProjectBuilds(pid)
	dt := time.Now().Sub(t0)

	git.debug.Printf("GetAllProjectsBuilds() => %d records in %v\n", len(result), dt)
	return result, err
}
