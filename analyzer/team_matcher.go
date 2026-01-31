package analyzer

import (
	"regexp"
	"sync"
)

type TeamMatcher struct {
	patterns map[string][]*regexp.Regexp
	mu       sync.RWMutex
}

func NewTeamMatcher(teamPatterns map[string][]string) (*TeamMatcher, error) {
	m := &TeamMatcher{
		patterns: make(map[string][]*regexp.Regexp),
	}

	for team, patterns := range teamPatterns {
		compiled := make([]*regexp.Regexp, 0, len(patterns))
		for _, p := range patterns {
			re, err := regexp.Compile(p)
			if err != nil {
				return nil, err
			}
			compiled = append(compiled, re)
		}
		m.patterns[team] = compiled
	}

	return m, nil
}

func (m *TeamMatcher) GetTeam(metricName string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for team, patterns := range m.patterns {
		for _, pattern := range patterns {
			if pattern.MatchString(metricName) {
				return team
			}
		}
	}

	return "unassigned"
}

func (m *TeamMatcher) GetAllTeams() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	teams := make([]string, 0, len(m.patterns))
	for team := range m.patterns {
		teams = append(teams, team)
	}
	return teams
}
