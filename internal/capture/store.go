package capture

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxEntries = 500
	maxBody    = 64 * 1024
)

type Row struct {
	Client      *Client   `json:"client,omitempty"`
	FrameCount  int       `json:"frameCount,omitempty"`
	ID          uint64    `json:"id"`
	Started     time.Time `json:"started"`
	Method      string    `json:"method"`
	URL         string    `json:"url"`
	Host        string    `json:"host"`
	Path        string    `json:"path"`
	Kind        string    `json:"kind"`
	Status      int       `json:"status"`
	Duration    int64     `json:"duration"`
	Size        int64     `json:"size"`
	ContentType string    `json:"contentType"`
	Pending     bool      `json:"pending"`
	Error       string    `json:"error,omitempty"`
	Rule        string    `json:"rule,omitempty"`
	Source      string    `json:"source,omitempty"`
	Breakpoint  string    `json:"breakpoint,omitempty"`
}

type Detail struct {
	ScriptError      string      `json:"scriptError,omitempty"`
	ResponseTrailers http.Header `json:"responseTrailers,omitempty"`
	Timing           *Timing     `json:"timing,omitempty"`
	Frames           []Frame     `json:"frames,omitempty"`
	Row
	RequestHeaders    http.Header `json:"requestHeaders"`
	ResponseHeaders   http.Header `json:"responseHeaders"`
	RequestBody       []byte      `json:"requestBody"`
	ResponseBody      []byte      `json:"responseBody"`
	RequestBytes      int64       `json:"requestBytes"`
	ResponseTruncated bool        `json:"responseTruncated"`
	RequestTruncated  bool        `json:"requestTruncated"`
	ResponseProtocol  string      `json:"responseProtocol,omitempty"`
	Protocol          string      `json:"protocol"`
}

type store struct {
	mu       sync.RWMutex
	revision uint64
	nextID   uint64
	order    []uint64
	items    map[uint64]Detail
}

func newStore() *store {
	return &store{revision: 1, items: make(map[uint64]Detail)}
}

func (s *store) insert(row Row) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	row.ID = s.nextID
	if len(s.order) == maxEntries {
		delete(s.items, s.order[0])
		copy(s.order, s.order[1:])
		s.order = s.order[:maxEntries-1]
	}
	s.items[row.ID] = Detail{Row: row}
	s.order = append(s.order, row.ID)
	s.revision++
	return row.ID
}

func (s *store) update(id uint64, detail Detail) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous, ok := s.items[id]; !ok {
		return
	} else if previous.FrameCount > detail.FrameCount {
		detail.FrameCount, detail.Frames = previous.FrameCount, previous.Frames
	}
	detail.ID = id
	s.items[id] = detail
	s.revision++
}

func (s *store) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make(map[uint64]Detail)
	s.order = nil
	s.revision++
}

func (s *store) snapshot(since uint64) (uint64, []Row, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if since == s.revision {
		return s.revision, nil, false
	}
	rows := make([]Row, 0, len(s.order))
	for i := len(s.order) - 1; i >= 0; i-- {
		rows = append(rows, s.items[s.order[i]].Row)
	}
	return s.revision, rows, true
}

func (s *store) get(id uint64) (Detail, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	detail, ok := s.items[id]
	return detail, ok
}

func (s *store) entries() []Detail {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]Detail, 0, len(s.order))
	for _, id := range s.order {
		entries = append(entries, s.items[id])
	}
	return entries
}

type preview struct {
	mu    sync.Mutex
	data  []byte
	total int64
}

func (p *preview) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.total += int64(len(b))
	remaining := min(len(b), maxBody-len(p.data))
	p.data = append(p.data, b[:remaining]...)
	return len(b), nil
}

func (p *preview) result() ([]byte, int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.data...), p.total
}

func contentType(h http.Header) string {
	kind, _, _ := strings.Cut(h.Get("Content-Type"), ";")
	return strings.TrimSpace(kind)
}
