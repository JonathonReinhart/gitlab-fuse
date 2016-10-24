package gitlabfs

import (
	"github.com/xanzy/go-gitlab"
)

type GitlabClient struct {
	*gitlab.Client
}

func NewGitlabClient(client *gitlab.Client) *GitlabClient {
	return &GitlabClient{
		Client: client,
	}
}

// GetAllVisibleProjects returns a map of namespace to a list of Projects in that namespace.
func (git *GitlabClient) GetAllVisibleProjects() (map[string][]*gitlab.Project, error) {
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

func (git *GitlabClient) GetAllProjectBuilds(pid interface{}) ([]gitlab.Build, error) {
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
