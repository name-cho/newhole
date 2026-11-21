package newhole

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/name-cho/newhole/config"
	lm "github.com/name-cho/newhole/internal/app/newhole/models"
	"golang.org/x/crypto/ssh"
)

type sshGateway struct {
	listener net.Listener
	config   *ssh.ServerConfig
	forwards sync.Map
	closed   chan struct{}
	t        *testing.T
}

func newSSHGateway(t *testing.T) *sshGateway {
	t.Helper()
	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatal(err)
	}

	serverConfig := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	serverConfig.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gateway := &sshGateway{listener: listener, config: serverConfig, closed: make(chan struct{}), t: t}
	go gateway.serve()
	t.Cleanup(func() {
		close(gateway.closed)
		listener.Close()
		gateway.forwards.Range(func(key, value interface{}) bool {
			value.(net.Listener).Close()
			return true
		})
	})
	return gateway
}

func (g *sshGateway) serve() {
	for {
		conn, err := g.listener.Accept()
		if err != nil {
			return
		}
		go g.handle(conn)
	}
}

func (g *sshGateway) handle(conn net.Conn) {
	serverConn, channels, requests, err := ssh.NewServerConn(conn, g.config)
	if err != nil {
		return
	}
	defer serverConn.Close()

	go func() {
		for request := range requests {
			if request.Type != "tcpip-forward" {
				request.Reply(false, nil)
				continue
			}
			address, port, ok := parseForwardPayload(request.Payload)
			if !ok {
				request.Reply(false, nil)
				continue
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				request.Reply(false, []byte(err.Error()))
				continue
			}
			var reply []byte
			assignedPort := listener.Addr().(*net.TCPAddr).Port
			if port == 0 {
				reply = ssh.Marshal(struct{ Port uint32 }{uint32(assignedPort)})
			}
			g.forwards.Store(port, listener)
			g.forwards.Store(assignedPort, listener)
			go g.forward(listener, serverConn, port)
			if port == 0 {
				go g.forward(listener, serverConn, assignedPort)
			}
			g.t.Logf("forward request for %q port %d -> %d", address, port, assignedPort)
			request.Reply(true, reply)
		}
	}()

	for newChannel := range channels {
		newChannel.Reject(ssh.UnknownChannelType, "unsupported")
	}
}

func parseForwardPayload(payload []byte) (string, int, bool) {
	if len(payload) < 4 {
		return "", 0, false
	}
	length := int(payload[0])<<24 | int(payload[1])<<16 | int(payload[2])<<8 | int(payload[3])
	if len(payload) < 4+length+4 {
		return "", 0, false
	}
	port := int(payload[4+length])<<24 | int(payload[5+length])<<16 | int(payload[6+length])<<8 | int(payload[7+length])
	return string(payload[4 : 4+length]), port, true
}

func (g *sshGateway) forward(listener net.Listener, serverConn *ssh.ServerConn, port int) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-g.closed:
			default:
			}
			return
		}
		go func(conn net.Conn) {
			payload := ssh.Marshal(struct {
				ConnectedAddress  string
				ConnectedPort     uint32
				OriginatorAddress string
				OriginatorPort    uint32
			}{"127.0.0.1", uint32(port), "127.0.0.1", 4321})
			channel, requests, err := serverConn.OpenChannel("forwarded-tcpip", payload)
			if err != nil {
				conn.Close()
				return
			}
			go ssh.DiscardRequests(requests)
			go func() {
				io.Copy(channel, conn)
				channel.Close()
			}()
			io.Copy(conn, channel)
			conn.Close()
		}(conn)
	}
}

func TestTunnelForwardsTrafficToLocalService(t *testing.T) {
	var receivedHost string
	var receivedPath string

	backend := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
		receivedPath = r.URL.Path
		fmt.Fprint(w, "hello from backend")
	})}
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go backend.Serve(backendListener)
	defer backend.Close()

	gateway := newSSHGateway(t)

	oldGateway := config.Config.GatewayEndpoint
	oldPolicy := config.Config.HostKeyPolicy
	oldDial := config.Config.Tunnels.DialTimeout
	config.Config.GatewayEndpoint = lm.Endpoint{Host: "127.0.0.1", Port: int32(gateway.listener.Addr().(*net.TCPAddr).Port)}
	config.Config.HostKeyPolicy = "insecure"
	config.Config.Tunnels.DialTimeout = "5s"
	t.Cleanup(func() {
		config.Config.GatewayEndpoint = oldGateway
		config.Config.HostKeyPolicy = oldPolicy
		config.Config.Tunnels.DialTimeout = oldDial
	})

	_, clientKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(clientKey)
	if err != nil {
		t.Fatal(err)
	}

	remoteConfig := lm.RemoteEndpointSpecs{
		TunnelID: "test-tunnel",
		SiteID:   "mysite",
		Domain:   "loophole.site",
	}
	fwd := newForwarder(remoteConfig, ssh.PublicKeys(signer), func() (net.Conn, error) {
		return net.Dial("tcp", backendListener.Addr().String())
	}, "backend")

	config.Config.Tunnels.MaxReconnects = 1
	if err := fwd.connect(); err != nil {
		t.Fatalf("unable to connect the tunnel: %v", err)
	}
	go fwd.acceptLoop()
	defer fwd.teardown()

	listener, ok := gateway.forwards.Load(80)
	if !ok {
		t.Fatal("the gateway never received a port forwarding request")
	}
	forwardedListener := listener.(net.Listener)

	conn, err := net.Dial("tcp", forwardedListener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET /hello HTTP/1.1\r\nHost: mysite.loophole.site\r\nConnection: close\r\n\r\n")

	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("unable to read the proxied response: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d differs from expected", response.StatusCode)
	}
	if string(body) != "hello from backend" {
		t.Fatalf("body %q differs from expected", string(body))
	}
	if receivedPath != "/hello" {
		t.Fatalf("backend received path %q", receivedPath)
	}
	if receivedHost != "mysite.loophole.site" {
		t.Fatalf("backend received host %q", receivedHost)
	}
}

func TestForwarderStopsOnQuit(t *testing.T) {
	gateway := newSSHGateway(t)

	oldGateway := config.Config.GatewayEndpoint
	oldPolicy := config.Config.HostKeyPolicy
	config.Config.GatewayEndpoint = lm.Endpoint{Host: "127.0.0.1", Port: int32(gateway.listener.Addr().(*net.TCPAddr).Port)}
	config.Config.HostKeyPolicy = "insecure"
	t.Cleanup(func() {
		config.Config.GatewayEndpoint = oldGateway
		config.Config.HostKeyPolicy = oldPolicy
	})

	_, clientKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(clientKey)
	if err != nil {
		t.Fatal(err)
	}

	remoteConfig := lm.RemoteEndpointSpecs{TunnelID: "quit-test", SiteID: "mysite", Domain: "loophole.site"}
	fwd := newForwarder(remoteConfig, ssh.PublicKeys(signer), func() (net.Conn, error) {
		return net.Dial("tcp", "127.0.0.1:9")
	}, "nowhere")

	quitChannel := make(chan bool, 1)
	done := make(chan error, 1)
	go func() {
		done <- fwd.run(quitChannel)
	}()

	time.Sleep(500 * time.Millisecond)
	quitChannel <- true

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the tunnel did not stop after the quit signal")
	}
	if !fwd.closed.Load() {
		t.Fatal("the forwarder was not marked as closed")
	}
}
