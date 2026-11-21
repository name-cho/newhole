package models

import "fmt"

type LocalHTTPEndpointSpecs struct {
	Port       int32  `json:"port"`
	Host       string `json:"host"`
	HTTPS      bool   `json:"https"`
	Path       string `json:"path"`
	UnixSocket string `json:"unixSocket"`
}

func Validate(options *LocalHTTPEndpointSpecs) error {
	if options.UnixSocket != "" {
		if options.Host != "" {
			return fmt.Errorf("host cannot be used together with unix socket")
		}
		return nil
	}
	if options.Port <= 0 {
		return fmt.Errorf("Port not set")
	}
	return nil
}
