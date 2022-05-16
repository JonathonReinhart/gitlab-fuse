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

func (git *GitlabClient) getAllProjectJobs(pid interface{}) ([]*gitlab.Job, error) {
	result := make([]*gitlab.Job, 0)

	opt := gitlab.ListJobsOptions{
		ListOptions: gitlab.ListOptions{
			Page:    1,
			PerPage: 100,
		},
	}

	for {
		jobs, resp, err := git.Jobs.ListProjectJobs(pid, &opt)
		if err != nil {
			return nil, err
		}

		result = append(result, jobs...)

		// Go to the next page
		if resp.NextPage == 0 {
			break
		}
		opt.ListOptions.Page = resp.NextPage
	}

	return result, nil
}

func (git *GitlabClient) GetAllProjectJobs(pid interface{}) ([]*gitlab.Job, error) {
	t0 := time.Now()
	result, err := git.getAllProjectJobs(pid)
	dt := time.Now().Sub(t0)

	git.debug.Printf("GetAllProjectsJobs() => %d records in %v\n", len(result), dt)
	return result, err
}

/******************************************************************************/

func (git *GitlabClient) getAllProjectPipelines(pid interface{}) ([]*gitlab.PipelineInfo, error) {
	result := make([]*gitlab.PipelineInfo, 0)

	opt := gitlab.ListProjectPipelinesOptions{
		ListOptions: gitlab.ListOptions{
			Page:    1,
			PerPage: 100,
		},
	}

	for {
		jobs, resp, err := git.Pipelines.ListProjectPipelines(pid, &opt)
		if err != nil {
			return nil, err
		}

		result = append(result, jobs...)

		// Go to the next page
		if resp.NextPage == 0 {
			break
		}
		opt.ListOptions.Page = resp.NextPage
	}

	return result, nil
}

func (git *GitlabClient) GetAllProjectPipelines(pid interface{}) ([]*gitlab.PipelineInfo, error) {
	t0 := time.Now()
	result, err := git.getAllProjectPipelines(pid)
	dt := time.Now().Sub(t0)

	git.debug.Printf("GetAllProjectPipelines() => %d records in %v\n", len(result), dt)
	return result, err
}
