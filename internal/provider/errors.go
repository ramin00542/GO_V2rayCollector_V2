package provider

import (
	"errors"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/fetch"
)

func recordError(result *domain.ProviderResult, err error) {
	result.Error = err.Error()
	var httpError *fetch.HTTPError
	if errors.As(err, &httpError) {
		result.HTTPStatus = httpError.StatusCode
	} else {
		// Set default status code for non-HTTP errors
		// Use 0 for unknown errors, but for classification purposes we'll use 500
		if result.HTTPStatus == 0 {
			result.HTTPStatus = 500
		}
	}
}
