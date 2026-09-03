package s3

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	s3Namespace = "http://s3.amazonaws.com/doc/2006-03-01/"
)

type S3Object struct {
	Key          string
	Content      []byte
	ContentType  string
	ETag         string
	LastModified time.Time
}

type S3Bucket struct {
	Name               string
	LocationConstraint string
	CreationDate       time.Time
	Objects            map[string]*S3Object
}

type MultipartUpload struct {
	Bucket string
	Key    string
	Parts  map[int][]byte
}

type S3Handler struct {
	mu        sync.RWMutex
	buckets   map[string]*S3Bucket
	uploads   map[string]*MultipartUpload
	AccountID string
}

func (h *S3Handler) Name() string {
	return "s3"
}

func (h *S3Handler) Matches(r *http.Request) bool {
	host := strings.ToLower(r.Host)
	if strings.Contains(host, "s3.") || strings.HasPrefix(host, "s3-") {
		return true
	}
	// Fallback to true since S3 is the fallback/default service for path-style requests on port 4566.
	// This plugin should be registered last.
	return true
}

func NewHandler() *S3Handler {
	return &S3Handler{
		buckets:   make(map[string]*S3Bucket),
		uploads:   make(map[string]*MultipartUpload),
		AccountID: "000000000000",
	}
}

// XML structures
type ListAllMyBucketsBucket struct {
	Name         string `xml:"Name"`
	CreationDate string `xml:"CreationDate"`
}

type ListAllMyBucketsResult struct {
	XMLName xml.Name                 `xml:"ListAllMyBucketsResult"`
	Xmlns   string                   `xml:"xmlns,attr"`
	Buckets []ListAllMyBucketsBucket `xml:"Buckets>Bucket"`
	OwnerID string                   `xml:"Owner>ID"`
	OwnerName string                 `xml:"Owner>DisplayName"`
}

type ListBucketResultContents struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

type ListBucketResult struct {
	XMLName     xml.Name                   `xml:"ListBucketResult"`
	Xmlns       string                     `xml:"xmlns,attr"`
	Name        string                     `xml:"Name"`
	Prefix      string                     `xml:"Prefix"`
	KeyCount    int                        `xml:"KeyCount"`
	MaxKeys     int                        `xml:"MaxKeys"`
	IsTruncated bool                       `xml:"IsTruncated"`
	Contents    []ListBucketResultContents `xml:"Contents"`
}

type LocationConstraintResponse struct {
	XMLName xml.Name `xml:"LocationConstraint"`
	Xmlns   string   `xml:"xmlns,attr"`
	Region  string   `xml:",chardata"`
}

type InitiateMultipartUploadResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	Xmlns    string   `xml:"xmlns,attr"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadId string   `xml:"UploadId"`
}

type CopyPartResult struct {
	XMLName      xml.Name `xml:"CopyPartResult"`
	Xmlns        string   `xml:"xmlns,attr"`
	LastModified string   `xml:"LastModified"`
	ETag         string   `xml:"ETag"`
}

type CompleteMultipartUploadResult struct {
	XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
	Xmlns    string   `xml:"xmlns,attr"`
	Location string   `xml:"Location"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	ETag     string   `xml:"ETag"`
}

func (h *S3Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bucket, key := extractBucketAndKey(r)

	// Route based on request properties
	switch {
	case bucket == "" && key == "" && r.Method == http.MethodGet:
		h.handleListBuckets(w, r)

	case bucket != "" && key == "" && r.Method == http.MethodPut:
		h.handleCreateBucket(w, r, bucket)

	case bucket != "" && key == "" && r.Method == http.MethodGet:
		if r.URL.Query().Has("location") {
			h.handleGetBucketLocation(w, r, bucket)
		} else {
			h.handleListObjects(w, r, bucket)
		}

	case bucket != "" && key == "" && r.Method == http.MethodDelete:
		h.handleDeleteBucket(w, r, bucket)

	case bucket != "" && key != "" && r.Method == http.MethodPut:
		if r.URL.Query().Has("uploadId") {
			h.handleUploadPart(w, r, bucket, key)
		} else {
			h.handlePutObject(w, r, bucket, key)
		}

	case bucket != "" && key != "" && r.Method == http.MethodGet:
		h.handleGetObject(w, r, bucket, key)

	case bucket != "" && key != "" && r.Method == http.MethodHead:
		h.handleHeadObject(w, r, bucket, key)

	case bucket != "" && key != "" && r.Method == http.MethodDelete:
		h.handleDeleteObject(w, r, bucket, key)

	case bucket != "" && key != "" && r.Method == http.MethodPost:
		if r.URL.Query().Has("uploads") {
			h.handleCreateMultipartUpload(w, r, bucket, key)
		} else if r.URL.Query().Has("uploadId") {
			h.handleCompleteOrAbortMultipartUpload(w, r, bucket, key)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *S3Handler) handleListBuckets(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	bucketsList := make([]ListAllMyBucketsBucket, 0, len(h.buckets))
	for _, b := range h.buckets {
		bucketsList = append(bucketsList, ListAllMyBucketsBucket{
			Name:         b.Name,
			CreationDate: b.CreationDate.UTC().Format("2006-01-02T15:04:05.000Z"),
		})
	}

	writeXML(w, ListAllMyBucketsResult{
		Xmlns:     s3Namespace,
		Buckets:   bucketsList,
		OwnerID:   h.AccountID,
		OwnerName: "floci-go-owner",
	})
}

func (h *S3Handler) handleCreateBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Parse location constraint if set in body
	var location string
	bodyBytes, _ := io.ReadAll(r.Body)
	if len(bodyBytes) > 0 {
		type CreateBucketConfiguration struct {
			LocationConstraint string `xml:"LocationConstraint"`
		}
		var config CreateBucketConfiguration
		_ = xml.Unmarshal(bodyBytes, &config)
		location = config.LocationConstraint
	}

	h.buckets[bucket] = &S3Bucket{
		Name:               bucket,
		LocationConstraint: location,
		CreationDate:       time.Now(),
		Objects:            make(map[string]*S3Object),
	}

	w.WriteHeader(http.StatusOK)
}

func (h *S3Handler) handleGetBucketLocation(w http.ResponseWriter, r *http.Request, bucket string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	b, exists := h.buckets[bucket]
	if !exists {
		h.writeError(w, 404, "NoSuchBucket", "The specified bucket does not exist.")
		return
	}

	writeXML(w, LocationConstraintResponse{
		Xmlns:  s3Namespace,
		Region: b.LocationConstraint,
	})
}

func (h *S3Handler) handleListObjects(w http.ResponseWriter, r *http.Request, bucket string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	b, exists := h.buckets[bucket]
	if !exists {
		h.writeError(w, 404, "NoSuchBucket", "The specified bucket does not exist.")
		return
	}

	prefix := r.URL.Query().Get("prefix")

	contents := make([]ListBucketResultContents, 0)
	for _, obj := range b.Objects {
		if prefix == "" || strings.HasPrefix(obj.Key, prefix) {
			contents = append(contents, ListBucketResultContents{
				Key:          obj.Key,
				LastModified: obj.LastModified.UTC().Format("2006-01-02T15:04:05.000Z"),
				ETag:         obj.ETag,
				Size:         int64(len(obj.Content)),
				StorageClass: "STANDARD",
			})
		}
	}

	writeXML(w, ListBucketResult{
		Xmlns:       s3Namespace,
		Name:        bucket,
		Prefix:      prefix,
		KeyCount:    len(contents),
		MaxKeys:     1000,
		IsTruncated: false,
		Contents:    contents,
	})
}

func (h *S3Handler) handleDeleteBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	b, exists := h.buckets[bucket]
	if !exists {
		h.writeError(w, 404, "NoSuchBucket", "The specified bucket does not exist.")
		return
	}

	if len(b.Objects) > 0 {
		h.writeError(w, 409, "BucketNotEmpty", "The bucket you tried to delete is not empty.")
		return
	}

	delete(h.buckets, bucket)
	w.WriteHeader(http.StatusNoContent)
}

func (h *S3Handler) handlePutObject(w http.ResponseWriter, r *http.Request, bucket string, key string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	b, exists := h.buckets[bucket]
	if !exists {
		h.writeError(w, 404, "NoSuchBucket", "The specified bucket does not exist.")
		return
	}

	// 1. Check if CopyObject request
	copySource := r.Header.Get("X-Amz-Copy-Source")
	var content []byte
	var contentType string

	if copySource != "" {
		// x-amz-copy-source format: /bucket/key or bucket/key
		copySource = strings.TrimPrefix(copySource, "/")
		// Handle URL path escaping in copy source (tested by TestS3NonASCIIKey)
		unescapedSource, err := url.PathUnescape(copySource)
		if err == nil {
			copySource = unescapedSource
		}

		parts := strings.SplitN(copySource, "/", 2)
		if len(parts) < 2 {
			h.writeError(w, 400, "InvalidArgument", "Invalid copy source header.")
			return
		}
		srcBucket, srcKey := parts[0], parts[1]

		srcB, exists := h.buckets[srcBucket]
		if !exists {
			h.writeError(w, 404, "NoSuchBucket", "The source bucket does not exist.")
			return
		}
		srcObj, exists := srcB.Objects[srcKey]
		if !exists {
			h.writeError(w, 404, "NoSuchKey", "The source key does not exist.")
			return
		}

		content = srcObj.Content
		contentType = srcObj.ContentType
	} else {
		// Regular PutObject
		var err error
		content, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		contentType = r.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "binary/octet-stream"
		}
	}

	etag := fmt.Sprintf("\"%s\"", md5Hash(content))

	obj := &S3Object{
		Key:          key,
		Content:      content,
		ContentType:  contentType,
		ETag:         etag,
		LastModified: time.Now().Truncate(time.Second), // Second precision as verified by HeadObject test
	}

	b.Objects[key] = obj

	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)

	// If CopyObject, we must return CopyObjectResult body
	if copySource != "" {
		type CopyObjectResult struct {
			XMLName      xml.Name `xml:"CopyObjectResult"`
			LastModified string   `xml:"LastModified"`
			ETag         string   `xml:"ETag"`
		}
		writeXML(w, CopyObjectResult{
			LastModified: obj.LastModified.UTC().Format("2006-01-02T15:04:05.000Z"),
			ETag:         etag,
		})
	}
}

func (h *S3Handler) handleGetObject(w http.ResponseWriter, r *http.Request, bucket string, key string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	b, exists := h.buckets[bucket]
	if !exists {
		h.writeError(w, 404, "NoSuchBucket", "The specified bucket does not exist.")
		return
	}

	obj, exists := b.Objects[key]
	if !exists {
		h.writeError(w, 404, "NoSuchKey", "The specified key does not exist.")
		return
	}

	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(obj.Content)))
	w.Header().Set("ETag", obj.ETag)
	w.Header().Set("Last-Modified", obj.LastModified.UTC().Format(http.TimeFormat))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(obj.Content)
}

func (h *S3Handler) handleHeadObject(w http.ResponseWriter, r *http.Request, bucket string, key string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	b, exists := h.buckets[bucket]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	obj, exists := b.Objects[key]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(obj.Content)))
	w.Header().Set("ETag", obj.ETag)
	w.Header().Set("Last-Modified", obj.LastModified.UTC().Format(http.TimeFormat))
	w.WriteHeader(http.StatusOK)
}

func (h *S3Handler) handleDeleteObject(w http.ResponseWriter, r *http.Request, bucket string, key string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	b, exists := h.buckets[bucket]
	if !exists {
		h.writeError(w, 404, "NoSuchBucket", "The specified bucket does not exist.")
		return
	}

	delete(b.Objects, key)
	w.WriteHeader(http.StatusNoContent)
}

func (h *S3Handler) handleCreateMultipartUpload(w http.ResponseWriter, r *http.Request, bucket string, key string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	uploadID := generateUUID()
	h.uploads[uploadID] = &MultipartUpload{
		Bucket: bucket,
		Key:    key,
		Parts:  make(map[int][]byte),
	}

	writeXML(w, InitiateMultipartUploadResult{
		Xmlns:    s3Namespace,
		Bucket:   bucket,
		Key:      key,
		UploadId: uploadID,
	})
}

func (h *S3Handler) handleUploadPart(w http.ResponseWriter, r *http.Request, bucket string, key string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	uploadID := r.URL.Query().Get("uploadId")
	partStr := r.URL.Query().Get("partNumber")
	partNum, _ := strconv.Atoi(partStr)

	upload, exists := h.uploads[uploadID]
	if !exists {
		h.writeError(w, 404, "NoSuchUpload", "The specified multipart upload does not exist.")
		return
	}

	copySource := r.Header.Get("X-Amz-Copy-Source")
	var content []byte

	if copySource != "" {
		// Handle UploadPartCopy
		copySource = strings.TrimPrefix(copySource, "/")
		unescapedSource, err := url.PathUnescape(copySource)
		if err == nil {
			copySource = unescapedSource
		}

		parts := strings.SplitN(copySource, "/", 2)
		if len(parts) < 2 {
			h.writeError(w, 400, "InvalidArgument", "Invalid copy source header.")
			return
		}
		srcBucket, srcKey := parts[0], parts[1]

		srcB, exists := h.buckets[srcBucket]
		if !exists {
			h.writeError(w, 404, "NoSuchBucket", "The source bucket does not exist.")
			return
		}
		srcObj, exists := srcB.Objects[srcKey]
		if !exists {
			h.writeError(w, 404, "NoSuchKey", "The source key does not exist.")
			return
		}

		content = srcObj.Content
	} else {
		// Regular UploadPart
		var err error
		content, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	upload.Parts[partNum] = content
	etag := fmt.Sprintf("\"%s\"", md5Hash(content))

	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)

	if copySource != "" {
		writeXML(w, CopyPartResult{
			Xmlns:        s3Namespace,
			LastModified: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
			ETag:         etag,
		})
	}
}

func (h *S3Handler) handleCompleteOrAbortMultipartUpload(w http.ResponseWriter, r *http.Request, bucket string, key string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	uploadID := r.URL.Query().Get("uploadId")
	upload, exists := h.uploads[uploadID]
	if !exists {
		h.writeError(w, 404, "NoSuchUpload", "The specified multipart upload does not exist.")
		return
	}

	if r.Method == http.MethodDelete {
		// Abort
		delete(h.uploads, uploadID)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Complete
	type CompletedPart struct {
		PartNumber int    `xml:"PartNumber"`
		ETag       string `xml:"ETag"`
	}
	type CompletedMultipartUpload struct {
		Parts []CompletedPart `xml:"Part"`
	}

	bodyBytes, _ := io.ReadAll(r.Body)
	var completed CompletedMultipartUpload
	_ = xml.Unmarshal(bodyBytes, &completed)

	// Combine parts in order
	var combined []byte
	for _, part := range completed.Parts {
		partContent, exists := upload.Parts[part.PartNumber]
		if exists {
			combined = append(combined, partContent...)
		}
	}

	b, exists := h.buckets[bucket]
	if !exists {
		h.writeError(w, 404, "NoSuchBucket", "The specified bucket does not exist.")
		return
	}

	etag := fmt.Sprintf("\"%s\"", md5Hash(combined))
	obj := &S3Object{
		Key:          key,
		Content:      combined,
		ContentType:  "application/octet-stream",
		ETag:         etag,
		LastModified: time.Now().Truncate(time.Second),
	}

	b.Objects[key] = obj
	delete(h.uploads, uploadID)

	location := fmt.Sprintf("http://%s/%s/%s", r.Host, bucket, key)

	writeXML(w, CompleteMultipartUploadResult{
		Xmlns:    s3Namespace,
		Location: location,
		Bucket:   bucket,
		Key:      key,
		ETag:     etag,
	})
}

func (h *S3Handler) writeError(w http.ResponseWriter, status int, code string, msg string) {
	type ErrorResponse struct {
		XMLName   xml.Name `xml:"Error"`
		Code      string   `xml:"Code"`
		Message   string   `xml:"Message"`
		RequestID string   `xml:"RequestId"`
		HostID    string   `xml:"HostId"`
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(ErrorResponse{
		Code:      code,
		Message:   msg,
		RequestID: "00000000-0000-0000-0000-000000000000",
		HostID:    "floci-go-host-id",
	})
}

func extractBucketAndKey(r *http.Request) (string, string) {
	host := strings.ToLower(r.Host)
	path := r.URL.Path

	// 1. Virtual Host Style: {bucket}.s3.localhost:4566 or {bucket}.s3.us-east-1.amazonaws.com
	if idx := strings.Index(host, ".s3"); idx != -1 {
		bucket := host[:idx]
		key := strings.TrimPrefix(path, "/")
		return bucket, key
	}

	// 2. Path Style: localhost:4566/{bucket}/{key}
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return "", ""
	}

	parts := strings.SplitN(path, "/", 2)
	bucket := parts[0]
	key := ""
	if len(parts) > 1 {
		key = parts[1]
	}
	return bucket, key
}

func writeXML(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(data)
}

func md5Hash(data []byte) string {
	hasher := md5.New()
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))
}

func generateUUID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:])
}

type S3ObjectSnapshot struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	ContentType  string    `json:"contentType"`
	ETag         string    `json:"etag"`
	LastModified time.Time `json:"lastModified"`
}

type S3BucketSnapshot struct {
	Name               string             `json:"name"`
	LocationConstraint string             `json:"locationConstraint"`
	CreationDate       time.Time          `json:"creationDate"`
	Objects            []S3ObjectSnapshot `json:"objects"`
}

func (h *S3Handler) GetBuckets() []S3BucketSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make([]S3BucketSnapshot, 0, len(h.buckets))
	for _, b := range h.buckets {
		objs := make([]S3ObjectSnapshot, 0, len(b.Objects))
		for _, o := range b.Objects {
			objs = append(objs, S3ObjectSnapshot{
				Key:          o.Key,
				Size:         int64(len(o.Content)),
				ContentType:  o.ContentType,
				ETag:         o.ETag,
				LastModified: o.LastModified,
			})
		}
		res = append(res, S3BucketSnapshot{
			Name:               b.Name,
			LocationConstraint: b.LocationConstraint,
			CreationDate:       b.CreationDate,
			Objects:            objs,
		})
	}
	return res
}
