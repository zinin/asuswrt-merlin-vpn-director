package wizard

import "sync"

type Step string

const (
	StepNone         Step = ""
	StepSelectServer Step = "select_server"
	StepExclusions   Step = "exclusions"
	StepClients      Step = "clients"
	StepClientIP     Step = "client_ip"
	StepClientRoute  Step = "client_route"
	StepConfirm      Step = "confirm"
)

type ClientRoute struct {
	IP    string
	Route string // "xray", "ovpnc1", ..., "wgc5"
}

type State struct {
	mu          sync.Mutex
	ChatID      int64
	Step        Step
	ServerIndex int
	Exclusions  map[string]bool
	Clients     []ClientRoute
	PendingIP   string
}

// Thread-safe setters
func (s *State) SetStep(step Step) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Step = step
}

func (s *State) SetServerIndex(idx int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ServerIndex = idx
}

func (s *State) SetExclusion(key string, value bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Exclusions[key] = value
}

func (s *State) ToggleExclusion(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Exclusions[key] = !s.Exclusions[key]
}

func (s *State) SetPendingIP(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PendingIP = ip
}

func (s *State) AddClient(client ClientRoute) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Clients = append(s.Clients, client)
}

func (s *State) RemoveLastClient() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Clients) > 0 {
		s.Clients = s.Clients[:len(s.Clients)-1]
	}
}

// Thread-safe getters
func (s *State) GetStep() Step {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Step
}

func (s *State) GetServerIndex() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ServerIndex
}

func (s *State) GetPendingIP() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.PendingIP
}

func (s *State) GetExclusions() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := make(map[string]bool)
	for k, v := range s.Exclusions {
		copy[k] = v
	}
	return copy
}

func (s *State) GetClients() []ClientRoute {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := make([]ClientRoute, len(s.Clients))
	for i, c := range s.Clients {
		copy[i] = c
	}
	return copy
}

type Manager struct {
	mu     sync.RWMutex
	states map[int64]*State
}

func NewManager() *Manager {
	return &Manager{
		states: make(map[int64]*State),
	}
}

func (m *Manager) Get(chatID int64) *State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.states[chatID]
}

func (m *Manager) Start(chatID int64) *State {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := &State{
		ChatID:     chatID,
		Step:       StepSelectServer,
		Exclusions: make(map[string]bool),
		Clients:    []ClientRoute{},
	}
	m.states[chatID] = state
	return state
}

func (m *Manager) Clear(chatID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.states, chatID)
}
