package models

type ExposeDirectoryConfig struct {
	Local  LocalDirectorySpecs `json:"local"`
	Remote RemoteEndpointSpecs `json:"remote"`
}
