package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/senseylabs/kaizen-cli/internal/cache"
	"github.com/senseylabs/kaizen-cli/internal/client"
)

const optionsCacheTTL = 15 * time.Minute

// fetchMembers fetches board members for selection, cached for 15 minutes.
func fetchMembers(boardID string, c *client.KaizenClient) ([]client.BoardMember, error) {
	cacheKey := fmt.Sprintf("members:%s", boardID)

	if cached, ok := cache.Get(cacheKey, optionsCacheTTL); ok {
		var members []client.BoardMember
		if json.Unmarshal(cached, &members) == nil {
			return members, nil
		}
	}

	path := fmt.Sprintf("/kaizen/boards/%s/members", boardID)
	body, err := c.Get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch members: %w", err)
	}

	var resp client.APIResponse[[]client.BoardMember]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse members response: %w", err)
	}

	_ = cache.Set(cacheKey, resp.Data)

	return resp.Data, nil
}

// fetchLabels fetches board labels for selection, cached for 15 minutes.
func fetchLabels(boardID string, c *client.KaizenClient) ([]client.Label, error) {
	cacheKey := fmt.Sprintf("labels:%s", boardID)

	if cached, ok := cache.Get(cacheKey, optionsCacheTTL); ok {
		var labels []client.Label
		if json.Unmarshal(cached, &labels) == nil {
			return labels, nil
		}
	}

	path := fmt.Sprintf("/kaizen/boards/%s/labels", boardID)
	body, err := c.Get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch labels: %w", err)
	}

	var resp client.APIResponse[[]client.Label]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse labels response: %w", err)
	}

	_ = cache.Set(cacheKey, resp.Data)

	return resp.Data, nil
}

// fetchProjects fetches board projects for selection, cached for 15 minutes.
func fetchProjects(boardID string, c *client.KaizenClient) ([]client.Project, error) {
	cacheKey := fmt.Sprintf("projects:%s", boardID)

	if cached, ok := cache.Get(cacheKey, optionsCacheTTL); ok {
		var projects []client.Project
		if json.Unmarshal(cached, &projects) == nil {
			return projects, nil
		}
	}

	path := fmt.Sprintf("/kaizen/boards/%s/projects", boardID)
	body, err := c.Get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch projects: %w", err)
	}

	var resp client.APIResponse[[]client.Project]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse projects response: %w", err)
	}

	_ = cache.Set(cacheKey, resp.Data)

	return resp.Data, nil
}
