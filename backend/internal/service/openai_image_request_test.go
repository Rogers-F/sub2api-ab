//go:build unit

package service

import (
	"bytes"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOpenAIImageRequestMeta_JSON(t *testing.T) {
	meta, err := ParseOpenAIImageRequestMeta([]byte(`{"model":"gpt-image-2","n":2,"size":"1536x1024"}`), "application/json")
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", meta.Model)
	require.Equal(t, 2, meta.ImageCount)
	require.Equal(t, "2K", meta.ImageSize)
	require.False(t, meta.Multipart)
}

func TestParseOpenAIImageRequestMeta_Multipart(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("n", "3"))
	require.NoError(t, writer.WriteField("size", "1024x1024"))
	require.NoError(t, writer.Close())

	meta, err := ParseOpenAIImageRequestMeta(body.Bytes(), writer.FormDataContentType())
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", meta.Model)
	require.Equal(t, 3, meta.ImageCount)
	require.Equal(t, "1K", meta.ImageSize)
	require.True(t, meta.Multipart)
}

func TestRewriteOpenAIImageRequestModel_Multipart(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "cat"))
	require.NoError(t, writer.Close())

	rewrittenBody, rewrittenContentType, err := rewriteOpenAIImageRequestModel(body.Bytes(), writer.FormDataContentType(), "gpt-image-2-upstream")
	require.NoError(t, err)

	meta, err := ParseOpenAIImageRequestMeta(rewrittenBody, rewrittenContentType)
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2-upstream", meta.Model)
}

func TestExtractOpenAIUsageFromJSONBytes_ImageTokens(t *testing.T) {
	usage, ok := extractOpenAIUsageFromJSONBytes([]byte(`{
		"usage":{
			"input_tokens":12,
			"output_tokens":34,
			"input_tokens_details":{"cached_tokens":5},
			"output_tokens_details":{"image_tokens":144}
		}
	}`))
	require.True(t, ok)
	require.Equal(t, 12, usage.InputTokens)
	require.Equal(t, 34, usage.OutputTokens)
	require.Equal(t, 5, usage.CacheReadInputTokens)
	require.Equal(t, 144, usage.ImageOutputTokens)
}
