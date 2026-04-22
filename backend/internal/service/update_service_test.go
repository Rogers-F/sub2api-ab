//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateServiceCacheStub struct{}

func (updateServiceCacheStub) GetUpdateInfo(ctx context.Context) (string, error) {
	return "", errors.New("cache miss")
}

func (updateServiceCacheStub) SetUpdateInfo(ctx context.Context, data string, ttl time.Duration) error {
	return nil
}

type updateServiceGitHubClientStub struct {
	lastRepo string
	release  *GitHubRelease
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(ctx context.Context, repo string) (*GitHubRelease, error) {
	s.lastRepo = repo
	return s.release, nil
}

func (s *updateServiceGitHubClientStub) DownloadFile(ctx context.Context, url, dest string, maxSize int64) error {
	return nil
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(ctx context.Context, url string) ([]byte, error) {
	return nil, nil
}

func TestUpdateServiceCheckUpdate_UsesForkReleaseRepo(t *testing.T) {
	t.Parallel()

	client := &updateServiceGitHubClientStub{
		release: &GitHubRelease{
			TagName: "v0.1.119",
			Name:    "Sub2API 0.1.119",
			HTMLURL: "https://github.com/Rogers-F/sub2api-ab/releases/tag/v0.1.119",
		},
	}

	svc := NewUpdateService(updateServiceCacheStub{}, client, "0.1.118", "release")
	info, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.Equal(t, "Rogers-F/sub2api-ab", client.lastRepo)
	require.Equal(t, "0.1.118", info.CurrentVersion)
	require.Equal(t, "0.1.119", info.LatestVersion)
	require.True(t, info.HasUpdate)
	require.NotNil(t, info.ReleaseInfo)
	require.Equal(t, "https://github.com/Rogers-F/sub2api-ab/releases/tag/v0.1.119", info.ReleaseInfo.HTMLURL)
}
