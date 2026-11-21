package models

type ExposeWebdavConfig struct {
	Local  LocalDirectorySpecs `json:"local"`
	Remote RemoteEndpointSpecs `json:"remote"`
}
