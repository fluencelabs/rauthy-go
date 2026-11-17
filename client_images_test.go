package rauthy_test

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fluencelabs/rauthy-go"
)

func TestSniffImageMime(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		data []byte
		want string
		bad  bool
	}{
		{name: "png", data: []byte("\x89PNG\r\n\x1a\n\x00\x00"), want: rauthy.MimePNG},
		{name: "jpeg", data: []byte{0xFF, 0xD8, 0xFF, 0xE0}, want: rauthy.MimeJPEG},
		{name: "bare svg", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), want: rauthy.MimeSVG},
		{
			name: "svg behind an xml declaration",
			data: []byte("<?xml version=\"1.0\"?>\n<svg/>"),
			want: rauthy.MimeSVG,
		},
		{
			// A byte-order mark ahead of the declaration is common from
			// Windows editors and must not defeat the sniff.
			name: "svg behind a bom",
			data: []byte("\xef\xbb\xbf<svg/>"),
			want: rauthy.MimeSVG,
		},
		{name: "webp is not an accepted upload type", data: []byte("RIFF\x00\x00\x00\x00WEBPVP8L"), bad: true},
		{name: "gif is not an accepted upload type", data: []byte("GIF89a"), bad: true},
		{name: "empty", data: nil, bad: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := rauthy.SniffImageMime(tc.data)
			if tc.bad {
				if err == nil {
					t.Fatalf("SniffImageMime = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("SniffImageMime: %v", err)
			}
			if got != tc.want {
				t.Fatalf("SniffImageMime = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPutClientImageWireFormat pins the request shape, which the OpenAPI
// document does not describe at all: a multipart body whose single part
// carries a filename and the declared image type.
func TestPutClientImageWireFormat(t *testing.T) {
	t.Parallel()

	var (
		gotPath        string
		gotPartName    string
		gotPartType    string
		gotPartFile    string
		gotPartPayload []byte
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		gotPath = r.URL.Path
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/form-data" {
			t.Errorf("content type = %q (%v), want multipart/form-data", r.Header.Get("Content-Type"), err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mr := multipart.NewReader(r.Body, params["boundary"])
		part, err := mr.NextPart()
		if err != nil {
			t.Errorf("read part: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotPartName = part.FormName()
		gotPartFile = part.FileName()
		gotPartType = part.Header.Get("Content-Type")
		gotPartPayload, _ = io.ReadAll(part)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := rauthy.New(srv.URL, "name$secret")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	payload := []byte("\x89PNG\r\n\x1a\npretend this is an image")
	err = c.PutClientImage(context.Background(), "my client", rauthy.ImageLogo, rauthy.Image{
		Data:        payload,
		ContentType: rauthy.MimePNG,
	})
	if err != nil {
		t.Fatalf("PutClientImage: %v", err)
	}

	// The client id is path-escaped on the wire; net/http hands the handler the
	// decoded form back.
	if want := "/auth/v1/clients/my client/logo"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotPartName != "logo" {
		t.Errorf("part name = %q, want logo", gotPartName)
	}
	if gotPartFile == "" {
		t.Error("part has no filename; Rauthy needs one to treat it as a file upload")
	}
	if gotPartType != rauthy.MimePNG {
		t.Errorf("part content type = %q, want %q", gotPartType, rauthy.MimePNG)
	}
	if string(gotPartPayload) != string(payload) {
		t.Errorf("part payload = %q, want %q", gotPartPayload, payload)
	}
}

// TestGetClientImageReturnsRawBytes covers the half of the round trip that is
// not JSON: the body is the image and the type comes from the header.
func TestGetClientImageReturnsRawBytes(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/favicon") {
			w.Header().Set("Content-Type", "image/webp")
			_, _ = w.Write([]byte("RIFFwebp-bytes"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := rauthy.New(srv.URL, "name$secret")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	got, err := c.GetClientImage(context.Background(), "app", rauthy.ImageFavicon)
	if err != nil {
		t.Fatalf("GetClientImage: %v", err)
	}
	if got.ContentType != "image/webp" {
		t.Errorf("content type = %q, want image/webp", got.ContentType)
	}
	if string(got.Data) != "RIFFwebp-bytes" {
		t.Errorf("data = %q", got.Data)
	}

	// An absent image is a bare 404 with no error envelope, so IsNotFound has
	// to work off the status alone.
	_, missingErr := c.GetClientImage(context.Background(), "app", rauthy.ImageLogo)
	if !rauthy.IsNotFound(missingErr) {
		t.Fatalf("missing logo: got %v, want a not-found error", missingErr)
	}
}

// The kind is the last path segment, so only the kinds Rauthy defines may go
// there.
func TestClientImage_RejectsUnknownKind(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	})

	img := rauthy.Image{Data: []byte("<svg/>"), ContentType: rauthy.MimeSVG}
	for _, kind := range []rauthy.ImageKind{"", "../../users/u1", "banner", "logo?x"} {
		if _, err := c.GetClientImage(t.Context(), "cl1", kind); err == nil {
			t.Errorf("GetClientImage(%q) succeeded, want error", kind)
		}
		if err := c.PutClientImage(t.Context(), "cl1", kind, img); err == nil {
			t.Errorf("PutClientImage(%q) succeeded, want error", kind)
		}
		if err := c.DeleteClientImage(t.Context(), "cl1", kind); err == nil {
			t.Errorf("DeleteClientImage(%q) succeeded, want error", kind)
		}
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("server received %d request(s), want none", got)
	}
}

// The content type is written into the multipart part header as given, so a
// CR or LF in it would inject headers into the part.
func TestPutClientImage_RejectsBadContentType(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	})

	for _, ct := range []string{
		"image/png\r\nX-Injected: 1",
		"image/png\nX-Injected: 1",
		"image/png\r",
		"not a media type",
		"",
	} {
		img := rauthy.Image{Data: []byte("\x89PNG\r\n\x1a\n"), ContentType: ct}
		if err := c.PutClientImage(t.Context(), "cl1", rauthy.ImageLogo, img); err == nil {
			t.Errorf("PutClientImage with content type %q succeeded, want error", ct)
		}
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("server received %d request(s), want none", got)
	}
}
