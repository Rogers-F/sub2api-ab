package server

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

func TestProvideHTTPServerEnablesH2CWithNetHTTPProtocols(t *testing.T) {
	router := gin.New()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:              "127.0.0.1",
			Port:              8080,
			ReadHeaderTimeout: 30,
			IdleTimeout:       120,
			H2C: config.H2CConfig{
				Enabled:                      true,
				MaxConcurrentStreams:         50,
				MaxReadFrameSize:             1 << 20,
				MaxUploadBufferPerConnection: 2 << 20,
				MaxUploadBufferPerStream:     512 << 10,
			},
		},
	}

	server := ProvideHTTPServer(cfg, router)

	if server.Protocols == nil {
		t.Fatal("expected H2C protocols to be configured")
	}
	if !server.Protocols.HTTP1() {
		t.Fatal("expected HTTP/1 to remain enabled")
	}
	if !server.Protocols.UnencryptedHTTP2() {
		t.Fatal("expected unencrypted HTTP/2 to be enabled")
	}
	if server.HTTP2 == nil {
		t.Fatal("expected HTTP/2 config to be configured")
	}
	if server.HTTP2.MaxConcurrentStreams != 50 {
		t.Fatalf("MaxConcurrentStreams = %d, want 50", server.HTTP2.MaxConcurrentStreams)
	}
	if server.HTTP2.MaxReadFrameSize != 1<<20 {
		t.Fatalf("MaxReadFrameSize = %d, want %d", server.HTTP2.MaxReadFrameSize, 1<<20)
	}
	if server.HTTP2.MaxReceiveBufferPerConnection != 2<<20 {
		t.Fatalf("MaxReceiveBufferPerConnection = %d, want %d", server.HTTP2.MaxReceiveBufferPerConnection, 2<<20)
	}
	if server.HTTP2.MaxReceiveBufferPerStream != 512<<10 {
		t.Fatalf("MaxReceiveBufferPerStream = %d, want %d", server.HTTP2.MaxReceiveBufferPerStream, 512<<10)
	}
}

func TestProvideHTTPServerLeavesProtocolsDefaultWhenH2CDisabled(t *testing.T) {
	router := gin.New()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:              "127.0.0.1",
			Port:              8080,
			ReadHeaderTimeout: 30,
			IdleTimeout:       120,
		},
	}

	server := ProvideHTTPServer(cfg, router)

	if server.Protocols != nil {
		t.Fatal("expected default protocol handling when H2C is disabled")
	}
	if server.HTTP2 != nil {
		t.Fatal("expected default HTTP/2 config when H2C is disabled")
	}
}
