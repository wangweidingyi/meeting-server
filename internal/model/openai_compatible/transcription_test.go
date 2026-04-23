package openai_compatible

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranscriptionClientRecognizeUploadsWaveAndReturnsText(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", request.Method)
		}
		if request.URL.Path != "/v1/audio/transcriptions" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer stt-key" {
			t.Fatalf("unexpected authorization header %q", got)
		}

		reader, err := request.MultipartReader()
		if err != nil {
			t.Fatalf("multipart reader: %v", err)
		}

		fields := map[string]string{}
		fileBody := []byte(nil)
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("next part: %v", err)
			}

			bytes, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("read part: %v", err)
			}

			if part.FileName() != "" {
				fileBody = bytes
				if got := part.FormName(); got != "file" {
					t.Fatalf("unexpected file field %s", got)
				}
				continue
			}

			fields[part.FormName()] = string(bytes)
		}

		if got := fields["model"]; got != "sensevoice-large" {
			t.Fatalf("unexpected model %q", got)
		}
		if len(fileBody) == 0 {
			t.Fatal("expected wave payload to be uploaded")
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"text":"会议测试文本"}`))
	}))
	defer server.Close()

	client := &TranscriptionClient{
		BaseURL: server.URL + "/v1",
		APIKey:  "stt-key",
		Model:   "sensevoice-large",
	}

	text, err := client.Recognize(context.Background(), []byte("RIFFfake-wave"))
	if err != nil {
		t.Fatalf("recognize: %v", err)
	}
	if text != "会议测试文本" {
		t.Fatalf("unexpected text %q", text)
	}
}

func TestTranscriptionClientMultipartBuilderProducesFilePart(t *testing.T) {
	t.Parallel()

	body, contentType, err := buildTranscriptionRequestBody("sensevoice-large", []byte("RIFFwav"))
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	if contentType == "" {
		t.Fatal("expected multipart content type")
	}

	reader := multipart.NewReader(body, multipartBoundary(t, contentType))
	partCount := 0
	for {
		_, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		partCount++
	}

	if partCount != 2 {
		t.Fatalf("expected 2 multipart parts, got %d", partCount)
	}
}

func multipartBoundary(t *testing.T, contentType string) string {
	t.Helper()

	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("parse media type: %v", err)
	}

	return params["boundary"]
}
