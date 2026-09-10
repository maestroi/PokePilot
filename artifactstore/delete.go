package artifactstore

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// DeleteObject removes key from the configured bucket. Deleting an already
// absent key is treated as success so cleanup remains idempotent across retries
// and S3-compatible implementations that answer missing DELETEs with 404.
func (s *S3) DeleteObject(ctx context.Context, key string) error {
	if s == nil {
		return fmt.Errorf("artifactstore: nil S3 client")
	}
	key, err := cleanObjectKey(key)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.objectURL(key), nil)
	if err != nil {
		return fmt.Errorf("artifactstore: build DeleteObject request: %w", err)
	}
	req.Header.Set("x-amz-content-sha256", emptyPayloadHash)
	s.sign(req, emptyPayloadHash, s.now().UTC())
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("artifactstore: DeleteObject %s: %w", key, err)
	}
	defer resp.Body.Close()
	if (resp.StatusCode >= 200 && resp.StatusCode < 300) || resp.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return statusError(resp, "DeleteObject", key)
}

// DeletePrefix removes every object below prefix and returns the number of
// objects deleted. Listing is paginated with ListObjectsV2 so deleting a run
// also covers artifacts and derived replay caches from older attempts.
func (s *S3) DeletePrefix(ctx context.Context, prefix string) (int, error) {
	if s == nil {
		return 0, fmt.Errorf("artifactstore: nil S3 client")
	}
	prefix, err := cleanObjectKey(prefix)
	if err != nil {
		return 0, err
	}

	deleted := 0
	continuation := ""
	for {
		page, err := s.listPrefix(ctx, prefix, continuation)
		if err != nil {
			return deleted, err
		}
		for _, item := range page.Contents {
			if err := s.DeleteObject(ctx, item.Key); err != nil {
				return deleted, err
			}
			deleted++
		}
		if !page.IsTruncated {
			return deleted, nil
		}
		if strings.TrimSpace(page.NextContinuationToken) == "" {
			return deleted, fmt.Errorf("artifactstore: ListObjectsV2 %s: truncated response without continuation token", prefix)
		}
		continuation = page.NextContinuationToken
	}
}

type listBucketResult struct {
	IsTruncated           bool                `xml:"IsTruncated"`
	NextContinuationToken string              `xml:"NextContinuationToken"`
	Contents              []listBucketContent `xml:"Contents"`
}

type listBucketContent struct {
	Key string `xml:"Key"`
}

func (s *S3) listPrefix(ctx context.Context, prefix, continuation string) (listBucketResult, error) {
	var out listBucketResult
	u := s.bucketURL()
	query := u.Query()
	query.Set("list-type", "2")
	query.Set("prefix", prefix)
	if continuation != "" {
		query.Set("continuation-token", continuation)
	}
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return out, fmt.Errorf("artifactstore: build ListObjectsV2 request: %w", err)
	}
	req.Header.Set("x-amz-content-sha256", emptyPayloadHash)
	s.sign(req, emptyPayloadHash, s.now().UTC())
	resp, err := s.http.Do(req)
	if err != nil {
		return out, fmt.Errorf("artifactstore: ListObjectsV2 %s: %w", prefix, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, statusError(resp, "ListObjectsV2", prefix)
	}
	if err := xml.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("artifactstore: decode ListObjectsV2 %s: %w", prefix, err)
	}
	return out, nil
}

func (s *S3) bucketURL() *url.URL {
	u := *s.endpoint
	base := strings.TrimSuffix(u.Path, "/")
	u.Path = base + "/" + s.bucket
	u.RawPath = ""
	return &u
}
