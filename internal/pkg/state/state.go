package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/name-cho/newhole/internal/pkg/cache"
)

const (
	historyFileName       = "hostnames.json"
	stateFileName         = "state.json"
	maxHistorySize        = 200
	activeTunnelsFileName = "active-tunnels.json"
)

type HostnameRecord struct {
	Hostname   string    `json:"hostname"`
	Domain     string    `json:"domain"`
	FirstUsed  time.Time `json:"firstUsed"`
	LastUsed   time.Time `json:"lastUsed"`
	Uses       int       `json:"uses"`
	ExposeMode string    `json:"exposeMode"`
}

type hostnameHistory struct {
	Hostnames []HostnameRecord `json:"hostnames"`
}

type LastTunnel struct {
	Mode     string    `json:"mode"`
	Target   string    `json:"target"`
	Hostname string    `json:"hostname"`
	UsedAt   time.Time `json:"usedAt"`
}

type storedState struct {
	LastTunnel *LastTunnel `json:"lastTunnel"`
}

var mutex sync.Mutex

func historyLocation() string {
	return cache.GetLocalStorageFile(historyFileName, "")
}

func stateLocation() string {
	return cache.GetLocalStorageFile(stateFileName, "")
}

func readHistory() hostnameHistory {
	history := hostnameHistory{}
	data, err := os.ReadFile(historyLocation())
	if err != nil {
		return history
	}
	if err := json.Unmarshal(data, &history); err != nil {
		return hostnameHistory{}
	}
	return history
}

func writeHistory(history hostnameHistory) error {
	sort.Slice(history.Hostnames, func(i, j int) bool {
		return history.Hostnames[i].LastUsed.After(history.Hostnames[j].LastUsed)
	})
	if len(history.Hostnames) > maxHistorySize {
		history.Hostnames = history.Hostnames[:maxHistorySize]
	}
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}
	location := historyLocation()
	if err := os.MkdirAll(filepath.Dir(location), 0o700); err != nil {
		return err
	}
	return os.WriteFile(location, data, 0o600)
}

func RecordHostname(hostname string, domain string, exposeMode string) error {
	if hostname == "" {
		return nil
	}
	mutex.Lock()
	defer mutex.Unlock()

	history := readHistory()
	now := time.Now()
	for index := range history.Hostnames {
		record := &history.Hostnames[index]
		if record.Hostname == hostname && record.Domain == domain {
			record.LastUsed = now
			record.Uses++
			if exposeMode != "" {
				record.ExposeMode = exposeMode
			}
			return writeHistory(history)
		}
	}
	history.Hostnames = append(history.Hostnames, HostnameRecord{
		Hostname:   hostname,
		Domain:     domain,
		FirstUsed:  now,
		LastUsed:   now,
		Uses:       1,
		ExposeMode: exposeMode,
	})
	return writeHistory(history)
}

func ForgetHostname(hostname string) error {
	mutex.Lock()
	defer mutex.Unlock()

	history := readHistory()
	kept := history.Hostnames[:0]
	for _, record := range history.Hostnames {
		if record.Hostname != hostname {
			kept = append(kept, record)
		}
	}
	history.Hostnames = kept
	return writeHistory(history)
}

func Hostnames() []HostnameRecord {
	mutex.Lock()
	defer mutex.Unlock()

	history := readHistory()
	sort.Slice(history.Hostnames, func(i, j int) bool {
		return history.Hostnames[i].LastUsed.After(history.Hostnames[j].LastUsed)
	})
	return history.Hostnames
}

func HostnameNames() []string {
	records := Hostnames()
	names := make([]string, 0, len(records))
	for _, record := range records {
		names = append(names, record.Hostname)
	}
	return names
}

func SaveLastTunnel(last LastTunnel) error {
	mutex.Lock()
	defer mutex.Unlock()

	state := storedState{LastTunnel: &last}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	location := stateLocation()
	if err := os.MkdirAll(filepath.Dir(location), 0o700); err != nil {
		return err
	}
	return os.WriteFile(location, data, 0o600)
}

func HostnamesJSON() ([]byte, error) {
	return json.MarshalIndent(hostnameHistory{Hostnames: Hostnames()}, "", "  ")
}

func LastTunnelSettings() *LastTunnel {
	mutex.Lock()
	defer mutex.Unlock()

	state := storedState{}
	data, err := os.ReadFile(stateLocation())
	if err != nil {
		return nil
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil
	}
	return state.LastTunnel
}

type ActiveTunnel struct {
	TunnelID string          `json:"tunnelId"`
	Mode     string          `json:"mode"`
	Payload  json.RawMessage `json:"payload"`
}

type activeTunnels struct {
	Tunnels []ActiveTunnel `json:"tunnels"`
}

func readActive() activeTunnels {
	stored := activeTunnels{}
	data, err := os.ReadFile(filepath.Join(mustStorageRoot(), activeTunnelsFileName))
	if err != nil {
		return stored
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return activeTunnels{}
	}
	return stored
}

func writeActive(stored activeTunnels) error {
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	location := filepath.Join(mustStorageRoot(), activeTunnelsFileName)
	if err := os.MkdirAll(filepath.Dir(location), 0o700); err != nil {
		return err
	}
	return os.WriteFile(location, data, 0o600)
}

func mustStorageRoot() string {
	root, err := cache.StorageRoot()
	if err != nil {
		return "."
	}
	return root
}

func SaveActiveTunnel(tunnel ActiveTunnel) error {
	if tunnel.TunnelID == "" {
		return nil
	}
	mutex.Lock()
	defer mutex.Unlock()

	stored := readActive()
	for index := range stored.Tunnels {
		if stored.Tunnels[index].TunnelID == tunnel.TunnelID {
			stored.Tunnels[index] = tunnel
			return writeActive(stored)
		}
	}
	stored.Tunnels = append(stored.Tunnels, tunnel)
	return writeActive(stored)
}

func RemoveActiveTunnel(tunnelID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	stored := readActive()
	kept := stored.Tunnels[:0]
	for _, tunnel := range stored.Tunnels {
		if tunnel.TunnelID != tunnelID {
			kept = append(kept, tunnel)
		}
	}
	stored.Tunnels = kept
	return writeActive(stored)
}

func ActiveTunnels() []ActiveTunnel {
	mutex.Lock()
	defer mutex.Unlock()
	return readActive().Tunnels
}
