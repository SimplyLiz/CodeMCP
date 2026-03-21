package query

import (
	"context"
)

// ClusterReviewerAssignment maps cluster-level reviewer suggestions.
type ClusterReviewerAssignment struct {
	ClusterName string            `json:"clusterName"`
	ClusterIdx  int               `json:"clusterIdx"`
	Reviewers   []SuggestedReview `json:"reviewers"`
}

// assignClusterReviewers assigns reviewers to each cluster based on ownership.
// Builds on the existing getSuggestedReviewers logic but scoped per cluster.
func (e *Engine) assignClusterReviewers(ctx context.Context, clusters []PRCluster) []ClusterReviewerAssignment {
	assignments := make([]ClusterReviewerAssignment, 0, len(clusters))

	for i, cluster := range clusters {
		files := make([]PRFileChange, 0, len(cluster.Files))
		for _, f := range cluster.Files {
			files = append(files, PRFileChange{Path: f})
		}

		reviewers := e.getSuggestedReviewers(ctx, files)

		// Limit to top 3 reviewers per cluster
		if len(reviewers) > 3 {
			reviewers = reviewers[:3]
		}

		assignments = append(assignments, ClusterReviewerAssignment{
			ClusterName: cluster.Name,
			ClusterIdx:  i,
			Reviewers:   reviewers,
		})
	}

	return assignments
}
