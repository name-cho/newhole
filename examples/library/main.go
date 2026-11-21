package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/name-cho/newhole/internal/app/newhole"
	lm "github.com/name-cho/newhole/internal/app/newhole/models"
)

func main() {
	hostname := flag.String("hostname", "", "custom hostname to request")
	port := flag.Int("port", 8080, "local port to expose")
	flag.Parse()

	server := &http.Server{Addr: "127.0.0.1:8080", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello from the embedded tunnel\n"))
	})}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	remote := lm.RemoteEndpointSpecs{TunnelID: "embedded-tunnel", SiteID: *hostname, ExposeMode: "http"}
	authMethod, err := newhole.RegisterTunnel(&remote)
	if err != nil {
		log.Fatal(err)
	}

	quit := make(chan bool)
	if err := newhole.ForwardPort(lm.ExposeHTTPConfig{
		Local:  lm.LocalHTTPEndpointSpecs{Host: "127.0.0.1", Port: int32(*port)},
		Remote: remote,
	}, authMethod, quit); err != nil {
		log.Fatal(err)
	}
}
