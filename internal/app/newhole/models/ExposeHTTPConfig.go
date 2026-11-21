package models

type ExposeHTTPConfig struct {
	Local  LocalHTTPEndpointSpecs `json:"local"`
	Remote RemoteEndpointSpecs    `json:"remote"`
}
