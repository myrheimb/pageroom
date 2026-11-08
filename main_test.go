package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOwnOriginOnly(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		origin        string
		secFetchSite  string
		wantForbidden bool
	}{
		{name: "app's own page", path: "/wails/runtime", origin: appOrigin()},
		{name: "no origin, same-origin fetch", path: "/wails/runtime", secFetchSite: "same-origin"},
		{name: "preview iframe", path: "/wails/runtime", origin: "http://127.0.0.1:1313", wantForbidden: true},
		{name: "sandboxed frame", path: "/wails/runtime", origin: "null", wantForbidden: true},
		{name: "no origin, cross-site", path: "/wails/runtime", secFetchSite: "cross-site", wantForbidden: true},
		{name: "event payloads", path: "/wails/eventpayload/1", origin: "http://127.0.0.1:1313", wantForbidden: true},
		{name: "site images", path: "/site-file/static/cat.jpg", origin: "http://127.0.0.1:1313"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, nil)
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.secFetchSite != "" {
				request.Header.Set("Sec-Fetch-Site", test.secFetchSite)
			}
			recorder := httptest.NewRecorder()
			ownOriginOnly(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(recorder, request)

			if forbidden := recorder.Code == http.StatusForbidden; forbidden != test.wantForbidden {
				t.Errorf("forbidden = %v, want %v", forbidden, test.wantForbidden)
			}
		})
	}
}
