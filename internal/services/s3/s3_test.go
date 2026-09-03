package s3

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestS3Lifecycle(t *testing.T) {
	handler := NewHandler()

	bucket := "my-test-bucket"
	key := "my-file.txt"
	content := "hello S3 world"

	// 1. Create Bucket
	req := httptest.NewRequest(http.MethodPut, "/"+bucket, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected CreateBucket status 200, got %d", w.Result().StatusCode)
	}

	// 2. List Buckets
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var listBuckets ListAllMyBucketsResult
	_ = xml.NewDecoder(w.Body).Decode(&listBuckets)
	if len(listBuckets.Buckets) != 1 || listBuckets.Buckets[0].Name != bucket {
		t.Errorf("expected 1 bucket named %s, got %v", bucket, listBuckets.Buckets)
	}

	// 3. Put Object
	req = httptest.NewRequest(http.MethodPut, "/"+bucket+"/"+key, strings.NewReader(content))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected PutObject status 200, got %d", w.Result().StatusCode)
	}

	// 4. Get Object
	req = httptest.NewRequest(http.MethodGet, "/"+bucket+"/"+key, nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected GetObject status 200, got %d", w.Result().StatusCode)
	}
	body, _ := io.ReadAll(w.Body)
	if string(body) != content {
		t.Errorf("expected body %s, got %s", content, string(body))
	}

	// 5. Head Object
	req = httptest.NewRequest(http.MethodHead, "/"+bucket+"/"+key, nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected HeadObject status 200, got %d", w.Result().StatusCode)
	}

	// 6. List Objects
	req = httptest.NewRequest(http.MethodGet, "/"+bucket, nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var listObjects ListBucketResult
	_ = xml.NewDecoder(w.Body).Decode(&listObjects)
	if len(listObjects.Contents) != 1 || listObjects.Contents[0].Key != key {
		t.Errorf("expected 1 object with key %s, got %v", key, listObjects.Contents)
	}

	// 7. Copy Object
	req = httptest.NewRequest(http.MethodPut, "/"+bucket+"/copied-"+key, nil)
	req.Header.Set("X-Amz-Copy-Source", "/"+bucket+"/"+key)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected CopyObject status 200, got %d", w.Result().StatusCode)
	}

	// Verify copied object exists
	req = httptest.NewRequest(http.MethodGet, "/"+bucket+"/copied-"+key, nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	body, _ = io.ReadAll(w.Body)
	if string(body) != content {
		t.Errorf("expected copied content %s, got %s", content, string(body))
	}

	// 8. Multipart Upload Lifecycle
	req = httptest.NewRequest(http.MethodPost, "/"+bucket+"/multipart.bin?uploads", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var initResp InitiateMultipartUploadResult
	_ = xml.NewDecoder(w.Body).Decode(&initResp)
	uploadId := initResp.UploadId
	if uploadId == "" {
		t.Fatalf("expected non-empty upload ID")
	}

	// Upload Part 1
	req = httptest.NewRequest(http.MethodPut, "/"+bucket+"/multipart.bin?uploadId="+uploadId+"&partNumber=1", strings.NewReader("part1-"))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	etag1 := w.Result().Header.Get("ETag")

	// Upload Part 2
	req = httptest.NewRequest(http.MethodPut, "/"+bucket+"/multipart.bin?uploadId="+uploadId+"&partNumber=2", strings.NewReader("part2"))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	etag2 := w.Result().Header.Get("ETag")

	// Complete Multipart Upload
	completeBody := fmt.Sprintf(`<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>%s</ETag></Part><Part><PartNumber>2</PartNumber><ETag>%s</ETag></Part></CompleteMultipartUpload>`, etag1, etag2)
	req = httptest.NewRequest(http.MethodPost, "/"+bucket+"/multipart.bin?uploadId="+uploadId, strings.NewReader(completeBody))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected CompleteMultipartUpload status 200, got %d", w.Result().StatusCode)
	}

	// Verify Multipart Object
	req = httptest.NewRequest(http.MethodGet, "/"+bucket+"/multipart.bin", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	body, _ = io.ReadAll(w.Body)
	if string(body) != "part1-part2" {
		t.Errorf("expected combined body 'part1-part2', got '%s'", string(body))
	}
}
