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
	}
}
