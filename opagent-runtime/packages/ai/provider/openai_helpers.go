package provider

import (
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func OpenAIRequestOptions(httpClient *http.Client, headers map[string]string) []option.RequestOption {
	opts := make([]option.RequestOption, 0, len(headers)+1)
	if httpClient != nil {
		opts = append(opts, option.WithHTTPClient(httpClient))
	}
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		opts = append(opts, option.WithHeader(key, value))
	}
	return opts
}

func normalizeSchema(schema any) openai.FunctionParameters {
	return normalizeProviderSchemaMap(schema)
}
