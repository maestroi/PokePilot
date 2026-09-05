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
	"errors"
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

var emptyPayloadHash = sha256Hex(nil)

type S3Config struct {
	Endpoint  string
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	Timeout   time.Duration
}

type Object struct {
	Bucket string
	Key    string
	Size   int64
	SHA256 string
}

// ReadObject is a streaming S3 response. Callers must close Body. StatusCode
// is normally 200 or 206 so HTTP relays can preserve byte-range semantics.
type ReadObject struct {
	Body          io.ReadCloser
	StatusCode    int
	ContentLength int64
	ContentType   string
	ContentRange  string
	AcceptRanges  string
	ETag          string
}

type StatusError struct {
	Operation  string
	Key        string
	StatusCode int
	Detail     string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("artifactstore: %s %s: status %d: %s", e.Operation, e.Key, e.StatusCode, e.Detail)
}

func IsNotFound(err error) bool {
	var status *StatusError
	return errors.As(err, &status) && status.StatusCode == http.StatusNotFound
}

type S3 struct {
	endpoint  *url.URL
	bucket    string
	region    string
	accessKey string
	secretKey string
	http      *http.Client
	now       func() time.Time
}

func S3FromEnv() (*S3, bool, error) {
	cfg := S3Config{
		Endpoint: strings.TrimSpace(os.Getenv(EnvS3Endpoint)), Bucket: strings.TrimSpace(os.Getenv(EnvS3Bucket)),
		Region: strings.TrimSpace(os.Getenv(EnvS3Region)), AccessKey: strings.TrimSpace(os.Getenv(EnvS3AccessKey)),
		SecretKey: strings.TrimSpace(os.Getenv(EnvS3SecretKey)),
	}
	if cfg.Endpoint == "" && cfg.Bucket == "" && cfg.AccessKey == "" && cfg.SecretKey == "" { return nil, false, nil }
	if cfg.Region == "" { cfg.Region = defaultRegion }
	if raw := strings.TrimSpace(os.Getenv(EnvS3Timeout)); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 { return nil, true, fmt.Errorf("artifactstore: %s must be a positive duration", EnvS3Timeout) }
		cfg.Timeout = d
	}
	client, err := NewS3(cfg)
	if err != nil { return nil, true, err }
	return client, true, nil
}

func NewS3(cfg S3Config) (*S3, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" { return nil, fmt.Errorf("artifactstore: %s is required", EnvS3Endpoint) }
	if strings.TrimSpace(cfg.Bucket) == "" { return nil, fmt.Errorf("artifactstore: %s is required", EnvS3Bucket) }
	if strings.Contains(cfg.Bucket, "/") { return nil, fmt.Errorf("artifactstore: bucket %q must not contain '/'", cfg.Bucket) }
	if strings.TrimSpace(cfg.AccessKey) == "" { return nil, fmt.Errorf("artifactstore: %s is required", EnvS3AccessKey) }
	if strings.TrimSpace(cfg.SecretKey) == "" { return nil, fmt.Errorf("artifactstore: %s is required", EnvS3SecretKey) }
	endpoint, err := url.Parse(strings.TrimRight(cfg.Endpoint, "/"))
	if err != nil { return nil, fmt.Errorf("artifactstore: parse S3 endpoint: %w", err) }
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" { return nil, fmt.Errorf("artifactstore: S3 endpoint scheme must be http or https") }
	if endpoint.Host == "" { return nil, fmt.Errorf("artifactstore: S3 endpoint has no host") }
	region := strings.TrimSpace(cfg.Region); if region == "" { region = defaultRegion }
	timeout := cfg.Timeout; if timeout <= 0 { timeout = defaultTimeout }
	return &S3{endpoint: endpoint, bucket: cfg.Bucket, region: region, accessKey: cfg.AccessKey, secretKey: cfg.SecretKey, http: &http.Client{Timeout: timeout}, now: time.Now}, nil
}

func (s *S3) Bucket() string { if s == nil { return "" }; return s.bucket }

func (s *S3) PutObject(ctx context.Context, key, mediaType string, data []byte) (Object, error) {
	return s.PutObjectReader(ctx, key, mediaType, bytes.NewReader(data))
}

// PutObjectReader stores a seekable stream without buffering the whole object.
func (s *S3) PutObjectReader(ctx context.Context, key, mediaType string, r io.ReadSeeker) (Object, error) {
	if s == nil { return Object{}, fmt.Errorf("artifactstore: nil S3 client") }
	key, err := cleanObjectKey(key); if err != nil { return Object{}, err }
	if r == nil { return Object{}, fmt.Errorf("artifactstore: nil object reader") }
	if _, err := r.Seek(0, io.SeekStart); err != nil { return Object{}, fmt.Errorf("artifactstore: seek %s before hash: %w", key, err) }
	h := sha256.New(); size, err := io.Copy(h, r)
	if err != nil { return Object{}, fmt.Errorf("artifactstore: hash %s: %w", key, err) }
	payloadHex := hex.EncodeToString(h.Sum(nil))
	if _, err := r.Seek(0, io.SeekStart); err != nil { return Object{}, fmt.Errorf("artifactstore: seek %s before upload: %w", key, err) }
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.objectURL(key), r)
	if err != nil { return Object{}, fmt.Errorf("artifactstore: build PutObject request: %w", err) }
	req.ContentLength = size
	if mediaType != "" { req.Header.Set("Content-Type", mediaType) }
	req.Header.Set("x-amz-content-sha256", payloadHex); s.sign(req, payloadHex, s.now().UTC())
	resp, err := s.http.Do(req)
	if err != nil { return Object{}, fmt.Errorf("artifactstore: PutObject %s: %w", key, err) }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return Object{}, statusError(resp, "PutObject", key) }
	return Object{Bucket: s.bucket, Key: key, Size: size, SHA256: payloadHex}, nil
}

func (s *S3) HeadObject(ctx context.Context, key string) (Object, error) {
	if s == nil { return Object{}, fmt.Errorf("artifactstore: nil S3 client") }
	key, err := cleanObjectKey(key); if err != nil { return Object{}, err }
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, s.objectURL(key), nil)
	if err != nil { return Object{}, fmt.Errorf("artifactstore: build HeadObject request: %w", err) }
	req.Header.Set("x-amz-content-sha256", emptyPayloadHash); s.sign(req, emptyPayloadHash, s.now().UTC())
	resp, err := s.http.Do(req)
	if err != nil { return Object{}, fmt.Errorf("artifactstore: HeadObject %s: %w", key, err) }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return Object{}, statusError(resp, "HeadObject", key) }
	return Object{Bucket: s.bucket, Key: key, Size: resp.ContentLength}, nil
}

func (s *S3) GetObject(ctx context.Context, key, rangeHeader string) (*ReadObject, error) {
	if s == nil { return nil, fmt.Errorf("artifactstore: nil S3 client") }
	key, err := cleanObjectKey(key); if err != nil { return nil, err }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.objectURL(key), nil)
	if err != nil { return nil, fmt.Errorf("artifactstore: build GetObject request: %w", err) }
	if rangeHeader = strings.TrimSpace(rangeHeader); rangeHeader != "" { req.Header.Set("Range", rangeHeader) }
	req.Header.Set("x-amz-content-sha256", emptyPayloadHash); s.sign(req, emptyPayloadHash, s.now().UTC())
	resp, err := s.http.Do(req)
	if err != nil { return nil, fmt.Errorf("artifactstore: GetObject %s: %w", key, err) }
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		err := statusError(resp, "GetObject", key); resp.Body.Close(); return nil, err
	}
	return &ReadObject{Body: resp.Body, StatusCode: resp.StatusCode, ContentLength: resp.ContentLength, ContentType: resp.Header.Get("Content-Type"), ContentRange: resp.Header.Get("Content-Range"), AcceptRanges: resp.Header.Get("Accept-Ranges"), ETag: resp.Header.Get("ETag")}, nil
}

func cleanObjectKey(key string) (string, error) {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" { return "", fmt.Errorf("artifactstore: empty object key") }
	return key, nil
}

func (s *S3) objectURL(key string) string {
	u := *s.endpoint; base := strings.TrimSuffix(u.Path, "/"); u.Path = base + "/" + s.bucket + "/" + key; u.RawPath = ""; return u.String()
}

func statusError(resp *http.Response, operation, key string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody)); detail := strings.TrimSpace(string(body)); if detail == "" { detail = resp.Status }
	return &StatusError{Operation: operation, Key: key, StatusCode: resp.StatusCode, Detail: detail}
}

func (s *S3) sign(req *http.Request, payloadHash string, now time.Time) {
	amzDate := now.Format("20060102T150405Z"); date := now.Format("20060102"); req.Header.Set("x-amz-date", amzDate)
	canonicalHeaders := "host:" + strings.ToLower(req.URL.Host) + "\n" + "x-amz-content-sha256:" + payloadHash + "\n" + "x-amz-date:" + amzDate + "\n"
	const signedHeaders = "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{req.Method, req.URL.EscapedPath(), req.URL.Query().Encode(), canonicalHeaders, signedHeaders, payloadHash}, "\n")
	canonicalHash := sha256.Sum256([]byte(canonicalRequest)); scope := date + "/" + s.region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(canonicalHash[:])
	kDate := hmacSHA256([]byte("AWS4"+s.secretKey), date); kRegion := hmacSHA256(kDate, s.region); kService := hmacSHA256(kRegion, "s3"); kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.accessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
}

func sha256Hex(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func hmacSHA256(key []byte, value string) []byte { h := hmac.New(sha256.New, key); _, _ = h.Write([]byte(value)); return h.Sum(nil) }
