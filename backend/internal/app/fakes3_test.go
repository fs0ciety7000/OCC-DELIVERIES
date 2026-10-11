package app

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeS3 is a minimal path-style S3 server (one bucket) for the backup
// tests: ListObjectsV2, Get/Head/Put/Delete object and multipart uploads.
// Signatures are not checked; no network is involved.
type fakeS3 struct {
	t       *testing.T
	bucket  string
	srv     *httptest.Server
	mu      sync.Mutex
	objects map[string]fakeObject
	uploads map[string]map[int][]byte
	nextID  int
	// requests seen, "METHOD key"
	requests []string
}

type fakeObject struct {
	data     []byte
	modified time.Time
}

func newFakeS3(t *testing.T, bucket string) *fakeS3 {
	f := &fakeS3{t: t, bucket: bucket, objects: map[string]fakeObject{}, uploads: map[string]map[int][]byte{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeS3) put(key string, data []byte, modified time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = fakeObject{data: data, modified: modified.UTC().Truncate(time.Second)}
}

func (f *fakeS3) keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.objects))
	for k := range f.objects {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (f *fakeS3) serve(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	bucket, key, _ := strings.Cut(path, "/")
	if bucket != f.bucket {
		http.Error(w, "<Error><Code>NoSuchBucket</Code></Error>", http.StatusNotFound)
		return
	}
	q := r.URL.Query()
	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+key)
	f.mu.Unlock()

	switch {
	case key == "" && r.Method == http.MethodGet:
		f.list(w, q.Get("prefix"))
	case r.Method == http.MethodPost && q.Has("uploads"):
		f.mu.Lock()
		f.nextID++
		id := fmt.Sprintf("up%d", f.nextID)
		f.uploads[id] = map[int][]byte{}
		f.mu.Unlock()
		writeXML(w, struct {
			XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
			Bucket   string
			Key      string
			UploadId string
		}{Bucket: bucket, Key: key, UploadId: id})
	case r.Method == http.MethodPut && q.Get("uploadId") != "":
		var n int
		fmt.Sscanf(q.Get("partNumber"), "%d", &n)
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.uploads[q.Get("uploadId")][n] = body
		f.mu.Unlock()
		w.Header().Set("ETag", fmt.Sprintf(`"part%d"`, n))
	case r.Method == http.MethodPost && q.Get("uploadId") != "":
		_, _ = io.Copy(io.Discard, r.Body)
		f.mu.Lock()
		parts := f.uploads[q.Get("uploadId")]
		nums := make([]int, 0, len(parts))
		for n := range parts {
			nums = append(nums, n)
		}
		sort.Ints(nums)
		var buf bytes.Buffer
		for _, n := range nums {
			buf.Write(parts[n])
		}
		delete(f.uploads, q.Get("uploadId"))
		f.objects[key] = fakeObject{data: buf.Bytes(), modified: time.Now().UTC().Truncate(time.Second)}
		f.mu.Unlock()
		writeXML(w, struct {
			XMLName xml.Name `xml:"CompleteMultipartUploadResult"`
			Key     string
			ETag    string
		}{Key: key, ETag: `"done"`})
	case r.Method == http.MethodDelete && q.Get("uploadId") != "":
		f.mu.Lock()
		delete(f.uploads, q.Get("uploadId"))
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		f.put(key, body, time.Now())
		w.Header().Set("ETag", `"etag"`)
	case r.Method == http.MethodDelete:
		f.mu.Lock()
		delete(f.objects, key)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		f.mu.Lock()
		obj, found := f.objects[key]
		f.mu.Unlock()
		if !found {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("<Error><Code>NoSuchKey</Code><Message>not found</Message></Error>"))
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("ETag", `"etag"`)
		http.ServeContent(w, r, key, obj.modified, bytes.NewReader(obj.data))
	default:
		http.Error(w, "unsupported", http.StatusNotImplemented)
	}
}

func (f *fakeS3) list(w http.ResponseWriter, prefix string) {
	type content struct {
		Key          string
		LastModified string
		Size         int64
		ETag         string
	}
	f.mu.Lock()
	var items []content
	for k, o := range f.objects {
		if strings.HasPrefix(k, prefix) {
			items = append(items, content{Key: k, LastModified: o.modified.Format(time.RFC3339), Size: int64(len(o.data)), ETag: `"etag"`})
		}
	}
	f.mu.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	writeXML(w, struct {
		XMLName     xml.Name `xml:"ListBucketResult"`
		Name        string
		Prefix      string
		KeyCount    int
		MaxKeys     int
		IsTruncated bool
		Contents    []content
	}{Name: f.bucket, Prefix: prefix, KeyCount: len(items), MaxKeys: 1000, Contents: items})
}

func writeXML(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(v)
}
