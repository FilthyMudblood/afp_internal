package topology

import "sync"

// InMemoryNeighborStore is a lock-protected local implementation for topology neighbors.
type InMemoryNeighborStore struct {
	mu          sync.RWMutex
	peerCVP     map[string]float64
	endpoints   map[string]string
	coreMembers map[string]struct{}
}

func NewInMemoryNeighborStore() *InMemoryNeighborStore {
	return &InMemoryNeighborStore{
		peerCVP:     make(map[string]float64),
		endpoints:   make(map[string]string),
		coreMembers: make(map[string]struct{}),
	}
}

func (s *InMemoryNeighborStore) UpsertNeighbor(peerID string, cvp float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.peerCVP[peerID] = cvp
	s.coreMembers[peerID] = struct{}{}
}

func (s *InMemoryNeighborStore) UpsertEndpoint(peerID string, endpoint string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.endpoints[peerID] = endpoint
}

func (s *InMemoryNeighborStore) ResolveEndpoint(peerID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	endpoint, ok := s.endpoints[peerID]
	return endpoint, ok
}

func (s *InMemoryNeighborStore) GetTrustedNeighbors(minCVPSThreshold float64) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	neighbors := make([]string, 0, len(s.coreMembers))
	for peerID := range s.coreMembers {
		cvp, ok := s.peerCVP[peerID]
		if !ok || cvp < minCVPSThreshold {
			continue
		}
		neighbors = append(neighbors, peerID)
	}
	return neighbors
}

func (s *InMemoryNeighborStore) ApplyPreemptiveDecay(peerID string, decayFactor float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cvp, ok := s.peerCVP[peerID]
	if !ok {
		return
	}
	newCVP := cvp * decayFactor
	if newCVP < 0 {
		newCVP = 0
	}
	s.peerCVP[peerID] = newCVP
}

func (s *InMemoryNeighborStore) GetCVP(peerID string) (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cvp, ok := s.peerCVP[peerID]
	return cvp, ok
}
