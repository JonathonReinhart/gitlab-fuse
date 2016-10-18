package gitlabfs

import (
	"github.com/xanzy/go-gitlab"
)

// GetAllVisibleProjects returns a map of namespace to a list of Projects in that namespace.
func GetAllVisibleProjects(git *gitlab.Client) (map[string][]*gitlab.Project, error) {
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
