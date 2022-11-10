package search

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepositoryExportData(t *testing.T) {
	var createdAt = time.Date(2021, 2, 28, 12, 30, 0, 0, time.UTC)
	tests := []struct {
		name   string
		fields []string
		repo   Repository
		output string
	}{
		{
			name:   "exports requested fields",
			fields: []string{"createdAt", "description", "fullName", "isArchived", "isFork", "isPrivate", "pushedAt"},
			repo: Repository{
				CreatedAt:   createdAt,
				Description: "description",
				FullName:    "ungtb10d/cli",
				IsArchived:  true,
				IsFork:      false,
				IsPrivate:   false,
				PushedAt:    createdAt,
			},
			output: `{"createdAt":"2021-02-28T12:30:00Z","description":"description","fullName":"ungtb10d/cli","isArchived":true,"isFork":false,"isPrivate":false,"pushedAt":"2021-02-28T12:30:00Z"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exported := tt.repo.ExportData(tt.fields)
			buf := bytes.Buffer{}
			enc := json.NewEncoder(&buf)
			require.NoError(t, enc.Encode(exported))
			assert.Equal(t, tt.output, strings.TrimSpace(buf.String()))
		})
	}
}

func TestIssueExportData(t *testing.T) {
	var updatedAt = time.Date(2021, 2, 28, 12, 30, 0, 0, time.UTC)
	tests := []struct {
		name   string
		fields []string
		issue  Issue
		output string
	}{
		{
			name:   "exports requested fields",
			fields: []string{"assignees", "body", "commentsCount", "labels", "isLocked", "repository", "title", "updatedAt"},
			issue: Issue{
				Assignees:     []User{{Login: "test"}},
				Body:          "body",
				CommentsCount: 1,
				Labels:        []Label{{Name: "label1"}, {Name: "label2"}},
				IsLocked:      true,
				RepositoryURL: "https://github.com/owner/repo",
				Title:         "title",
				UpdatedAt:     updatedAt,
			},
			output: `{"assignees":[{"id":"","login":"test","type":""}],"body":"body","commentsCount":1,"isLocked":true,"labels":[{"color":"","description":"","id":"","name":"label1"},{"color":"","description":"","id":"","name":"label2"}],"repository":{"name":"repo","nameWithOwner":"owner/repo"},"title":"title","updatedAt":"2021-02-28T12:30:00Z"}`,
		},
		{
			name:   "state when issue",
			fields: []string{"isPullRequest", "state"},
			issue: Issue{
				StateInternal: "closed",
			},
			output: `{"isPullRequest":false,"state":"closed"}`,
		},
		{
			name:   "state when pull request",
			fields: []string{"isPullRequest", "state"},
			issue: Issue{
				PullRequest: PullRequest{
					MergedAt: time.Now(),
					URL:      "a-url",
				},
				StateInternal: "closed",
			},
			output: `{"isPullRequest":true,"state":"merged"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exported := tt.issue.ExportData(tt.fields)
			buf := bytes.Buffer{}
			enc := json.NewEncoder(&buf)
			require.NoError(t, enc.Encode(exported))
			assert.Equal(t, tt.output, strings.TrimSpace(buf.String()))
		})
	}
}
