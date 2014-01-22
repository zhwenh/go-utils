package tls_test

import (
	"crypto/tls"
	"fmt"
	"github.com/fastly/go-utils/server"
	ttls "github.com/fastly/go-utils/tls"
	"io"
	"io/ioutil"
	"log"
	"strings"
	"testing"
	"time"
)

func NewMockServer() *server.Server {
	addr := ":0" // let kernel assign an unused port
	s, err := server.NewSingleServer(&addr)
	if err != nil {
		log.Panic(err)
		return nil
	}
	return s
}

func TestTLSCacheToProxy(t *testing.T) {
	check(t, "test-cache-client", "test-proxy-server", true)
}

func TestTLSProxyToSyslogd(t *testing.T) {
	check(t, "test-proxy-client", "test-syslogd-server", true)
}

// reject connections from unknown certs
func TestTLSUnknownToProxy(t *testing.T) {
	check(t, "test-unknown-client", "test-proxy-server", false)
}

func TestTLSProxyToUnknown(t *testing.T) {
	check(t, "test-proxy-client", "test-unknown-server", false)
}

func check(t *testing.T, clientName, serverName string, shouldPass bool) {
	clientConfig, err := ttls.ConfigureClient(clientName, "test-tls-ca")
	if err != nil {
		t.Errorf("Bad client key '%s': %s", clientName, err)
		return
	}
	serverConfig, err := ttls.ConfigureServer(serverName, "test-tls-ca")
	if err != nil {
		t.Errorf("Bad server key '%s': %s", serverName, err)
		return
	}

	server := NewMockServer()
	server.SetListener(tls.NewListener(server.Listener(), serverConfig))
	listener := server.Listener()

	testData := strings.Repeat("x", 1<<16)

	go func() {
		server.SignalReady()

		conn, err := listener.Accept()
		if err == nil {
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(time.Second))
			data, err := ioutil.ReadAll(conn)
			if err == nil && string(data) != testData {
				err = fmt.Errorf("Server read incorrect data; got '%s', expected '%s'", string(data), testData)
			}

			if err != nil && shouldPass {
				t.Errorf("Server read error: %v", err)
			} else if err == nil && !shouldPass {
				t.Errorf("Expected server read error: %v", err)
			}
		} else {
			t.Errorf("Listener error: %v", err)
		}

		server.WaitForShutdown()
		server.SignalFinish()
	}()

	server.WaitForReady()

	addr := listener.Addr().String()
	conn, err := tls.Dial("tcp", addr, clientConfig)
	if err != nil && shouldPass {
		t.Errorf("Client connection error: %v", err)
		return
	} else if err == nil && !shouldPass {
		t.Errorf("Expected client connection error: %v", err)
		return
	} else if err != nil {
		return
	} // else err == nil && shouldPass
	conn.SetDeadline(time.Now().Add(time.Second))

	n, err := io.WriteString(conn, testData)
	if err == nil && n < len(testData) {
		err = fmt.Errorf("Client incomplete write: expected %d bytes, got %d", len(testData), n)
	}

	if err != nil && shouldPass {
		t.Errorf("Client write error: %v", err)
	} else if err == nil && !shouldPass {
		t.Errorf("Expected client write error: %v", err)
	}

	conn.Close()
	server.Shutdown()
}
