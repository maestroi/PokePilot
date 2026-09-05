// Package artifactstore contains durable blob storage used by farm runners.
// It intentionally depends only on the standard library so self-hosted S3
// storage does not pull a cloud SDK into every PokePilot binary.
package artifactstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	EnvS3Endpoint  = "POKEPILOT_S3_ENDPOINT"
	EnvS3Bucket    = "POKEPILOT_S3_BUCKET"
	EnvS3Region    = "POKEPILOT_S3_REGION"
	EnvS3AccessKey = "POKEPILOT_S3_ACCESS_KEY"
	EnvS3SecretKey = "POKEPILOT_S3_SECRET_KEY"
	EnvS3Timeout   = "POKEPILOT_S3_TIMEOUT"

	defaultRegion  = "us-east-1"
	defaultTimeout = 60 * time.Second
	maxErrorBody   = 4 << 10
)

// S3Config configures a path-style S3-compatible endpoint. Path-style URLs
// work with RustFS and MinIO and keep buckets independent of local DNS.
type S3Config struct {
	Endpoint  string
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	Timeout   time.Duration
}

// Object describes one successfully persisted object without exposing storage
// credentials or the endpoint URL.
type Object struct {
	Bucket string
	Key    string
	Size   int64
	SHA256 string
}

// S3 is the minimal PutObject client PokePilot needs for durable artifacts.
// It signs requests with AWS Signature Version 4 and is compatible with S3
// implementations such as RustFS and MinIO.
type S3 struct {
	endpoint  *url.URL
	bucket    string
	region    string
	accessKey string
	secretKey string
	http      *http.Client
	now       func() time.Time
}

// S3FromEnv returns (nil, false, nil) when S3 storage is completely
// unconfigured. If any S3 variable is present the required tuple is validated
// so a typo cannot silently make a runner fall back to inline artifacts.
func S3FromEnv() (*S3, bool, error) {
	cfg := S3Config{
		Endpoint:  strings.TrimSpace(os.Getenv(EnvS3Endpoint)),
		Bucket:    strings.TrimSpace(os.Getenv(EnvS3Bucket)),
		Region:    strings.TrimSpace(os.Getenv(EnvS3Region)),
		AccessKey: strings.TrimSpace(os.Getenv(EnvS3AccessKey)),
		SecretKey: strings.TrimSpace(os.Getenv(EnvS3SecretKey)),
	}
	if cfg.Endpoint == "" && cfg.Bucket == "" && cfg.Region == "" && cfg.AccessKey == "" && cfg.SecretKey == "" && strings.TrimSpace(os.Getenv(EnvS3Timeout)) == "" {
		return nil, false, nil
	}
	if cfg.Region == "" {
		cfg.Region = defaultRegion
	}
	if raw := strings.TrimSpace(os.Getenv(EnvS3Timeout)); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			return nil, true, fmt.Errorf("artifactstore: %s must be a positive duration", EnvS3Timeout)
		}
		cfg.Timeout = d
	}
	client, err := NewS3(cfg)
	if err != nil {
		return nil, true, err
	}
	return client, true, nil
}

// NewS3 validates cfg and builds a path-style S3 client.
func NewS3(cfg S3Config) (*S3, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("artifactstore: %s is required", EnvS3Endpoint)
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("artifactstore: %s is required", EnvS3Bucket)
	}
	if strings.Contains(cfg.Bucket, "/") {
		return nil, fmt.Errorf("artifactstore: bucket %q must not contain '/'", cfg.Bucket)
	}
	if strings.TrimSpace(cfg.AccessKey) == "" {
		return nil, fmt.Errorf("artifactstore: %s is required", EnvS3AccessKey)
	}
	if strings.TrimSpace(cfg.SecretKey) == "" {
		return nil, fmt.Errorf("artifactstore: %s is required", EnvS3SecretKey)
	}
	endpoint, err := url.Parse(strings.TrimRight(cfg.Endpoint, "/"))
	if err != nil {
		return nil, fmt.Errorf("artifactstore: parse S3 endpoint: %w", err)
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, fmt.Errorf("artifactstore: S3 endpoint scheme must be http or https")
	}
	if endpoint.Host == "" {
		return nil, fmt.Errorf("artifactstore: S3 endpoint has no host")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = defaultRegion
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &S3{
		endpoint:  endpoint,
		bucket:    cfg.Bucket,
		region:    region,
		accessKey: cfg.AccessKey,
		secretKey: cfg.SecretKey,
		http:      &http.Client{Timeout: timeout},
		now:       time.Now,
	}, nil
}

// PutObject stores data under key and returns its stable object metadata.
func (s *S3) PutObject(ctx context.Context, key, mediaType string, data []byte) (Object, error) {
	if s == nil {
		return Object{}, fmt.Errorf("artifactstore: nil S3 client")
	}
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		return Object{}, fmt.Errorf("artifactstore: empty object key")
	}
	payload := sha256.Sum256(data)
	payloadHex := hex.EncodeToString(payload[:])

	u := *s.endpoint
	base := strings.TrimSuffix(u.Path, "/")
	u.Path = base + "/" + s.bucket + "/" + key
	u.RawPath = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), bytes.NewReader(data))
	if err != nil {
		return Object{}, fmt.Errorf("artifactstore: build PutObject request: %w", err)
	}
	if mediaType != "" {
		req.Header.Set("Content-Type", mediaType)
	}
	req.Header.Set("x-amz-content-sha256", payloadHex)
	s.sign(req, payloadHex, s.now().UTC())

	resp, err := s.http.Do(req)
	if err != nil {
		return Object{}, fmt.Errorf("artifactstore: PutObject %s: %w", key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			detail = resp.Status
		}
		return Object{}, fmt.Errorf("artifactstore: PutObject %s: status %d: %s", key, resp.StatusCode, detail)
	}
	return Object{Bucket: s.bucket, Key: key, Size: int64(len(data)), SHA256: payloadHex}, nil
}

func (s *S3) sign(req *http.Request, payloadHash string, now time.Time) {
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	req.Header.Set("x-amz-date", amzDate)

	canonicalHeaders := "host:" + strings.ToLower(req.URL.Host) + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"
	const signedHeaders = "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		req.URL.Query().Encode(),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	canonicalHash := sha256.Sum256([]byte(canonicalRequest))
	scope := date + "/" + s.region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(canonicalHash[:])

	kDate := hmacSHA256([]byte("AWS4"+s.secretKey), date)
	kRegion := hmacSHA256(kDate, s.region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.accessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
}

func hmacSHA256(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}
