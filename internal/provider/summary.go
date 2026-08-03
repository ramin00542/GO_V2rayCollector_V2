package provider

import "github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"

type Summary struct {
	Requests  int
	Succeeded int
	Failed    int
	BytesRead int
	Extracted int
	Accepted  int
	Rejected  int
}

func Summarize(results []domain.ProviderResult) Summary {
	var summary Summary
	for _, result := range results {
		summary.Requests++
		summary.BytesRead += result.BytesRead
		summary.Extracted += result.Extracted
		summary.Accepted += result.Accepted
		summary.Rejected += result.Rejected
		if result.Error == "" {
			summary.Succeeded++
		} else {
			summary.Failed++
		}
	}
	return summary
}
