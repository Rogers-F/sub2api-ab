//go:build integration

package tlsfingerprint

import (
	"crypto/x509"
	"net/url"
	"testing"
)

func TestIsDefaultCaptureServerUnavailableDetectsExpiredCertificate(t *testing.T) {
	err := &url.Error{
		Op:  "Post",
		URL: defaultCaptureURL,
		Err: x509.CertificateInvalidError{Reason: x509.Expired},
	}

	if !isDefaultCaptureServerUnavailable("", err) {
		t.Fatalf("expired certificate error for default capture server should be treated as unavailable")
	}
}

func TestIsDefaultCaptureServerUnavailableDoesNotSkipCustomURL(t *testing.T) {
	err := &url.Error{
		Op:  "Post",
		URL: "https://capture.example.test",
		Err: x509.CertificateInvalidError{Reason: x509.Expired},
	}

	if isDefaultCaptureServerUnavailable("https://capture.example.test", err) {
		t.Fatalf("custom capture URL failures should not be treated as default capture server unavailability")
	}
}
