package httpserver

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

const (
	sniffTimeout     = 10 * time.Second
	handshakeTimeout = 10 * time.Second
	tlsRecordType    = 0x16
)

type sniffConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *sniffConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

type connListener struct {
	conns chan net.Conn
	done  chan struct{}
}

func (l *connListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *connListener) Close() error {
	select {
	case <-l.done:
	default:
		close(l.done)
	}
	return nil
}

func (l *connListener) Addr() net.Addr {
	return tunnelAddr{}
}

type tunnelAddr struct{}

func (tunnelAddr) Network() string { return "tcp" }
func (tunnelAddr) String() string  { return "127.0.0.1" }

// Serve accepts TLS and plain HTTP connections on the same listener
func Serve(server *http.Server, listener net.Listener) error {
	httpListener := &connListener{conns: make(chan net.Conn), done: make(chan struct{})}
	go server.Serve(httpListener)
	h2Server := &http2.Server{}
	for {
		conn, err := listener.Accept()
		if err != nil {
			httpListener.Close()
			return err
		}
		go serveConn(server, h2Server, httpListener, conn)
	}
}

func serveConn(server *http.Server, h2Server *http2.Server, httpListener *connListener, conn net.Conn) {
	conn.SetReadDeadline(time.Now().Add(sniffTimeout))
	reader := bufio.NewReader(conn)
	firstByte, err := reader.Peek(1)
	conn.SetReadDeadline(time.Time{})
	if err != nil {
		conn.Close()
		return
	}

	buffered := &sniffConn{Conn: conn, reader: reader}
	if firstByte[0] != tlsRecordType {
		redirectToHTTPS(buffered)
		return
	}

	tlsConn := tls.Server(buffered, server.TLSConfig)
	tlsConn.SetDeadline(time.Now().Add(handshakeTimeout))
	if err := tlsConn.Handshake(); err != nil {
		tlsConn.Close()
		return
	}
	tlsConn.SetDeadline(time.Time{})

	if tlsConn.ConnectionState().NegotiatedProtocol == "h2" {
		h2Server.ServeConn(tlsConn, &http2.ServeConnOpts{Handler: server.Handler})
		return
	}

	select {
	case httpListener.conns <- tlsConn:
	case <-httpListener.done:
		tlsConn.Close()
	}
}

func redirectToHTTPS(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(sniffTimeout))
	request, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil || request.Host == "" {
		return
	}
	target := "https://" + request.Host + request.URL.RequestURI()
	fmt.Fprintf(conn, "HTTP/1.1 301 Moved Permanently\r\nLocation: %s\r\nConnection: close\r\nContent-Length: 0\r\n\r\n", target)
}
