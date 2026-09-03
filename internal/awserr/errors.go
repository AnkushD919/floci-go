package awserr

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
)

// AwsError represents a generic AWS error that can be formatted for any protocol.
type AwsError struct {
	StatusCode int    `json:"-" xml:"-"`
	Code       string `json:"code" xml:"Code"`
	Message    string `json:"message" xml:"Message"`
	Type       string `json:"type,omitempty" xml:"Type,omitempty"` // Sender or Receiver
}

func (e *AwsError) Error() string {
	return fmt.Sprintf("AWS Error %s: %s", e.Code, e.Message)
}

// New creates a new AwsError with a default Sender type and 400 Bad Request status.
func New(statusCode int, code, message string) *AwsError {
	return &AwsError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
		Type:       "Sender",
	}
}

// WriteXMLResponse writes the error in Query / REST-XML style.
// xmlns is optional (used by SQS, SNS etc.)
func (e *AwsError) WriteXMLResponse(w http.ResponseWriter, xmlns string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(e.StatusCode)

	type ErrorInner struct {
		Type    string `xml:"Type"`
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}

	type QueryErrorResponse struct {
		XMLName   xml.Name   `xml:"ErrorResponse"`
		Xmlns     string     `xml:"xmlns,attr,omitempty"`
		Error     ErrorInner `xml:"Error"`
		RequestID string     `xml:"RequestId"`
	}

	resp := QueryErrorResponse{
		Xmlns: xmlns,
		Error: ErrorInner{
			Type:    e.Type,
			Code:    e.Code,
			Message: e.Message,
		},
		RequestID: "00000000-0000-0000-0000-000000000000",
	}

	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(resp)
}

// WriteS3ErrorResponse writes errors in S3 REST-XML style (no wrapper, root element <Error>).
func (e *AwsError) WriteS3ErrorResponse(w http.ResponseWriter, bucket, key string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(e.StatusCode)

	type S3ErrorResponse struct {
		XMLName    xml.Name `xml:"Error"`
		Code       string   `xml:"Code"`
		Message    string   `xml:"Message"`
		BucketName string   `xml:"BucketName,omitempty"`
		Key        string   `xml:"Key,omitempty"`
		RequestID  string   `xml:"RequestId"`
		HostID     string   `xml:"HostId"`
	}

	resp := S3ErrorResponse{
		Code:       e.Code,
		Message:    e.Message,
		BucketName: bucket,
		Key:        key,
		RequestID:  "00000000-0000-0000-0000-000000000000",
		HostID:     "floci-s3-mock-host-id",
	}

	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(resp)
}

// WriteJSONResponse writes the error in JSON 1.1 or JSON 1.0 style.
// targetService is used to prefix the __type field (e.g. "com.amazonaws.dynamodb.v20120810").
func (e *AwsError) WriteJSONResponse(w http.ResponseWriter, targetService string) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.Header().Set("x-amzn-ErrorType", e.Code)
	w.WriteHeader(e.StatusCode)

	var typeField string
	if targetService != "" {
		typeField = fmt.Sprintf("%s#%s", targetService, e.Code)
	} else {
		typeField = e.Code
	}

	resp := map[string]interface{}{
		"__type":  typeField,
		"message": e.Message,
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// WriteRESTJSONResponse writes the error in REST-JSON style.
func (e *AwsError) WriteRESTJSONResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-amzn-ErrorType", e.Code)
	w.WriteHeader(e.StatusCode)

	resp := map[string]interface{}{
		"message": e.Message,
		"code":    e.Code,
		"type":    e.Type,
	}

	_ = json.NewEncoder(w).Encode(resp)
}
