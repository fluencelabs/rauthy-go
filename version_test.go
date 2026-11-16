package rauthy_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/fluencelabs/rauthy-go"
)

// The body a live 0.36.2 answers GET /auth/v1/version with.
const liveVersionBody = `{"current":"0.36.2","last_check":1788076923,"latest":"0.37.2",` +
	`"latest_url":"https://github.com/sebadob/rauthy/releases/tag/v0.37.2","update_available":true}`

// The spec lists no permission for GET /version, so the call goes out without
// the API key.
func TestGetVersion_ReadsVersionWithoutTheAPIKey(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	var gotAuth []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotAuth = r.Header.Values("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(liveVersionBody))
	})

	got, err := c.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/auth/v1/version" {
		t.Errorf("got %s %s, want GET /auth/v1/version", gotMethod, gotPath)
	}
	if len(gotAuth) != 0 {
		t.Errorf("Authorization = %q, want no header", gotAuth)
	}
	if got.Current != "0.36.2" || !got.UpdateAvailable {
		t.Errorf("got %+v", got)
	}
	if got.LastCheck == nil || *got.LastCheck != 1788076923 {
		t.Errorf("LastCheck = %v, want 1788076923", got.LastCheck)
	}
	if got.Latest == nil || *got.Latest != "0.37.2" {
		t.Errorf("Latest = %v, want 0.37.2", got.Latest)
	}
	if got.LatestURL == nil || *got.LatestURL != "https://github.com/sebadob/rauthy/releases/tag/v0.37.2" {
		t.Errorf("LatestURL = %v", got.LatestURL)
	}
}

// The other calls still carry the key: leaving it out of one request must not
// leak into the next.
func TestGetVersion_OtherCallsKeepTheAPIKey(t *testing.T) {
	t.Parallel()

	var gotAuth string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/v1/version" {
			_, _ = w.Write([]byte(liveVersionBody))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	})

	if _, err := c.GetVersion(context.Background()); err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if _, err := c.ListRoles(context.Background()); err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if gotAuth != "API-Key tf$secret" {
		t.Errorf("Authorization = %q after GetVersion, want the API key", gotAuth)
	}
}

func TestVersionResponse_AtLeast(t *testing.T) {
	t.Parallel()

	cases := []struct {
		current      string
		major, minor int
		want         bool
	}{
		{"0.36.2", 0, 36, true},
		{"0.36.2", 0, 35, true},
		{"0.36.2", 0, 37, false},
		{"0.36.2", 1, 0, false},
		{"0.36.0", 0, 36, true},
		{"1.0.0", 0, 99, true},
		{"2.1.0", 2, 1, true},
		{"2.1.0", 2, 2, false},
		{"10.0.0", 9, 0, true},
		// A pre-release of X.Y.0 comes before X.Y.0, so it is not yet X.Y.
		{"0.37.0-rc.1", 0, 37, false},
		{"0.37.0-rc.1", 0, 36, true},
		// A pre-release of a later patch is past X.Y.0.
		{"0.37.1-beta1", 0, 37, true},
		// Build metadata does not take part in precedence.
		{"0.37.0+20241016", 0, 37, true},
		{"0.37.0-rc.1+build.5", 0, 36, true},
	}
	for _, tc := range cases {
		v := rauthy.VersionResponse{Current: tc.current}
		got, err := v.AtLeast(tc.major, tc.minor)
		if err != nil {
			t.Errorf("%q.AtLeast(%d, %d): %v", tc.current, tc.major, tc.minor, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q.AtLeast(%d, %d) = %v, want %v", tc.current, tc.major, tc.minor, got, tc.want)
		}
	}
}

// A value that is not a semantic version is an error, never a guess in either
// direction.
func TestVersionResponse_AtLeastRejectsAnUnparsableVersion(t *testing.T) {
	t.Parallel()

	for _, current := range []string{
		"",
		"0.36",
		"0",
		"v0.36.2",
		" 0.36.2",
		"0.36.2 ",
		"0.036.2",
		"00.36.2",
		"0.36.02",
		"0.36.2.1",
		"0.36.x",
		"-1.36.2",
		"+1.36.2",
		"0.36.2-",
		"0.36.2+",
		"0.36.2-rc..1",
		"0.36.2-01",
		"0.36.2-rc_1",
		"0.36.2+build..1",
		"99999999999999999999.0.0",
		"unknown",
	} {
		v := rauthy.VersionResponse{Current: current}
		if got, err := v.AtLeast(0, 1); err == nil {
			t.Errorf("%q.AtLeast(0, 1) = %v, want an error", current, got)
		}
	}
}

func TestVersionResponse_AtLeastRejectsANegativeFloor(t *testing.T) {
	t.Parallel()

	v := rauthy.VersionResponse{Current: "0.36.2"}
	for _, floor := range [][2]int{{-1, 0}, {0, -1}} {
		if _, err := v.AtLeast(floor[0], floor[1]); err == nil {
			t.Errorf("AtLeast(%d, %d) succeeded, want an error", floor[0], floor[1])
		}
	}
}
