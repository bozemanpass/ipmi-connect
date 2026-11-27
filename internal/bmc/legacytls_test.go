package bmc

import (
	"crypto/tls"
	"slices"
	"testing"
)

func TestLegacyTLSConfig(t *testing.T) {
	cfg := LegacyTLSConfig()
	if cfg == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if !cfg.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify=true for legacy BMCs")
	}
	if cfg.MinVersion != tls.VersionTLS10 {
		t.Errorf("expected MinVersion=TLS1.0, got %x", cfg.MinVersion)
	}
	if len(cfg.CipherSuites) == 0 {
		t.Error("expected non-empty cipher suites")
	}
	// Later ATEN firmware (TLS 1.2) accepts only GCM suites; the X8DTL-6F only CBC ones.
	for _, want := range []uint16{tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384, tls.TLS_RSA_WITH_AES_256_CBC_SHA} {
		if !slices.Contains(cfg.CipherSuites, want) {
			t.Errorf("cipher suite %s missing", tls.CipherSuiteName(want))
		}
	}
}
