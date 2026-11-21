# Using Newhole as a library

The tunnel is a plain Go value: `newhole.ForwardPort`, `newhole.ForwardDirectory`
and `newhole.ForwardDirectoryViaWebdav` expose a local service and block until the
quit channel receives a value. That makes it possible to start a tunnel from your
own program, for example from a server that should publish itself.

```go
package main

import (
	"log"
	"time"

	"github.com/name-cho/newhole/internal/app/newhole"
	lm "github.com/name-cho/newhole/internal/app/newhole/models"
)

func main() {
	remote := lm.RemoteEndpointSpecs{
		TunnelID:     "my-service",
		SiteID:       "my-service", // custom hostname, empty for a random one
		IdentityFile: "/home/me/.newhole/.ssh/id_rsa",
	}

	authMethod, err := newhole.RegisterTunnel(&remote)
	if err != nil {
		log.Fatal(err)
	}

	quit := make(chan bool, 1)
	go func() {
		time.Sleep(10 * time.Minute)
		quit <- true
	}()

	expose := lm.ExposeHTTPConfig{
		Local:  lm.LocalHTTPEndpointSpecs{Host: "127.0.0.1", Port: 8080},
		Remote: remote,
	}
	if err := newhole.ForwardPort(expose, authMethod, quit); err != nil {
		log.Fatal(err)
	}
}
```

A runnable version lives in `examples/library`:

```bash
go run ./examples/library -port 8080
```

## WebSockets

The proxy speaks HTTP/1.1, so WebSocket upgrades (`Connection: Upgrade` plus
`Upgrade: websocket`) pass straight through to the local service. The public URL is
`https://<hostname>.<domain>`, so a browser connects to `wss://<hostname>.<domain>`
and the tunnel keeps the connection open as long as either side does.

Two caveats:

* The local service must answer the upgrade itself; Newhole does not translate
  between WebSockets and anything else.
* Keepalives only cover the SSH transport, so a network device that drops idle TCP
  connections drops an idle WebSocket as well.

## Compression

Newhole does not compress payloads: the tunnel forwards bytes as they are. Enable
compression in the local service (`gzip` middleware, `Content-Encoding`) and it is
preserved end to end, which also keeps WebSocket frames intact.
