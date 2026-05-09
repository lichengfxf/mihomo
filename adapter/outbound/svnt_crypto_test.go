package outbound

import (
	"io"
	"net"
	"testing"
)

func TestNewSVNTCryptoEnvAuth(t *testing.T) {
	env, err := newSVNTCryptoEnv("9C60A285DD88CDFACA9C9EFA156653EF")
	if err != nil {
		t.Fatalf("newSVNTCryptoEnv failed: %v", err)
	}

	const expected = "/xEcJHoQOuc2FO6iDILNvw=="
	if env.auth != expected {
		t.Fatalf("unexpected auth: got %q want %q", env.auth, expected)
	}
}

func TestSVNTCryptoConnRoundTrip(t *testing.T) {
	env, err := newSVNTCryptoEnv("9C60A285DD88CDFACA9C9EFA156653EF")
	if err != nil {
		t.Fatalf("newSVNTCryptoEnv failed: %v", err)
	}

	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	client := env.wrapConn(left)
	server := env.wrapConn(right)

	writeErrCh := make(chan error, 1)
	go func() {
		_, err := client.Write([]byte("hello svnt crypto"))
		writeErrCh <- err
	}()

	buf := make([]byte, len("hello svnt crypto"))
	if _, err := io.ReadFull(server, buf); err != nil {
		t.Fatalf("server read failed: %v", err)
	}
	if string(buf) != "hello svnt crypto" {
		t.Fatalf("unexpected payload: got %q", string(buf))
	}
	if err := <-writeErrCh; err != nil {
		t.Fatalf("client write failed: %v", err)
	}
}
