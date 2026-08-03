package provider

import (
	"errors"

	"github.com/RaminTabriz/V2rayCollector/internal/domain"
	"github.com/RaminTabriz/V2rayCollector/internal/fetch"
)

func recordError(result *domain.ProviderResult, err error) {
	result.Error = err.Error()
	var httpError *fetch.HTTPError
	if errors.As(err, &httpError) {
		result.HTTPStatus = httpError.StatusCode
	}
}
