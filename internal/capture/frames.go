package capture

import (
	"encoding/binary"
	"time"
)

type Frame struct {
	Direction  string    `json:"direction"`
	Opcode     byte      `json:"opcode"`
	Data       []byte    `json:"data"`
	Size       uint64    `json:"size"`
	Final      bool      `json:"final"`
	Time       time.Time `json:"time"`
	Compressed bool      `json:"compressed"`
}

// frameParser observes the byte stream without changing what the endpoints receive.
type frameParser struct {
	direction string
	onFrame   func(Frame)
	header    [14]byte
	headerLen int
	needed    int
	frame     Frame
	remaining uint64
	mask      []byte
}

func (p *frameParser) Write(data []byte) (int, error) {
	original := len(data)
	for len(data) > 0 {
		if p.needed == 0 {
			p.needed = 2
		}
		if p.headerLen < p.needed {
			n := min(len(data), p.needed-p.headerLen)
			copy(p.header[p.headerLen:], data[:n])
			p.headerLen += n
			data = data[n:]
			if p.headerLen < p.needed {
				continue
			}
			if p.needed == 2 {
				switch p.header[1] & 127 {
				case 126:
					p.needed += 2
				case 127:
					p.needed += 8
				}
				if p.header[1]&128 != 0 {
					p.needed += 4
				}
				if p.headerLen < p.needed {
					continue
				}
			}
			p.readHeader()
		}
		n := uint64(len(data))
		n = min(n, p.remaining)
		take := min(n, uint64(1024-len(p.frame.Data)))
		for i, value := range data[:take] {
			if len(p.mask) > 0 {
				value ^= p.mask[(p.frame.Size-p.remaining+uint64(i))%4]
			}
			p.frame.Data = append(p.frame.Data, value)
		}
		p.remaining -= n
		data = data[n:]
		if p.remaining == 0 {
			p.onFrame(p.frame)
			p.headerLen, p.needed = 0, 0
		}
	}
	return original, nil
}

func (p *frameParser) readHeader() {
	header := p.header
	length := uint64(header[1] & 127)
	offset := 2
	switch length {
	case 126:
		length = uint64(binary.BigEndian.Uint16(header[2:4]))
		offset = 4
	case 127:
		length = binary.BigEndian.Uint64(header[2:10])
		offset = 10
	}
	p.mask = nil
	if header[1]&128 != 0 {
		p.mask = p.header[offset : offset+4]
	}
	p.frame = Frame{
		Direction: p.direction, Opcode: header[0] & 15,
		Final: header[0]&128 != 0, Compressed: header[0]&64 != 0,
		Size: length, Time: time.Now(),
	}
	p.remaining = length
}

func (s *store) addFrame(id uint64, frame Frame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	detail, ok := s.items[id]
	if !ok {
		return
	}
	kept := detail.Frames
	if len(kept) >= 100 {
		kept = kept[1:]
	}
	detail.Frames = append(append([]Frame(nil), kept...), frame)
	detail.FrameCount++
	s.items[id] = detail
	s.revision++
}
