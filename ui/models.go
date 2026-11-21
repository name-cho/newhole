package ui

import (
	"encoding/json"
)

type MessageType string

const (
	MessageTypeStartTunnelHTTP      MessageType = "MT_RequestTunnelStart_HTTP"
	MessageTypeStartTunnelDirectory MessageType = "MT_RequestTunnelStart_Directory"
	MessageTypeStartTunnelWebDav    MessageType = "MT_RequestTunnelStart_WebDav"
	MessageTypeStopTunnel           MessageType = "MT_RequestTunnelStop"
	MessageTypeAuthorization        MessageType = "MT_RequestLogin"
	MessageTypeLogout               MessageType = "MT_RequestLogout"
	MessageTypeOpenBrowser          MessageType = "MT_OpenInBrowser"
	MessageTypePing                 MessageType = "MT_Ping"
	MessageTypeRefreshHistory       MessageType = "MT_RequestHostnameHistory"
	MessageTypeForgetHostname       MessageType = "MT_RequestHostnameForget"
	MessageTypeActiveTunnels        MessageType = "MT_RequestActiveTunnels"
)

type Message struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type StopTunnelMessage struct {
	TunnelID string `json:"tunnelId"`
}

type OpenInBrowserMessage struct {
	URL string `json:"url"`
}

type ForgetHostnameMessage struct {
	Hostname string `json:"hostname"`
}

type pingMessage struct {
	Type MessageType `json:"type"`
}
