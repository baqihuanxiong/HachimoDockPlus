package audio

import (
	"encoding/binary"
	"io"
)

// pcmVolume applies live volume to digital outputs without a hardware mixer.
// A small buffer keeps incomplete S16_LE samples across arbitrarily short reads.
type pcmVolume struct {
	source  io.Reader
	volume  *Volume
	pending []byte
	err     error
}

func (p *pcmVolume) Read(out []byte) (int, error) {
	if len(out) == 0 {
		return 0, nil
	}
	if len(p.pending) == 0 && p.err == nil {
		block := make([]byte, 4096)
		n, err := io.ReadFull(p.source, block)
		p.err = err
		if err == io.ErrUnexpectedEOF {
			p.err = io.EOF
		}
		block = block[:n]
		volume := p.volume.Snapshot().Volume
		for i := 0; i+1 < n; i += 2 {
			sample := int(int16(binary.LittleEndian.Uint16(block[i:])))
			binary.LittleEndian.PutUint16(block[i:], uint16(int16(sample*volume/100)))
		}
		p.pending = block
	}
	if len(p.pending) > 0 {
		n := copy(out, p.pending)
		p.pending = p.pending[n:]
		return n, nil
	}
	return 0, p.err
}
func outputPCM(source io.Reader, volume *Volume) io.Reader {
	if volume == nil {
		return source
	}
	s := volume.Snapshot()
	if s.Output != "spdif" && s.Output != "bluetooth" {
		return source
	}
	return &pcmVolume{source: source, volume: volume}
}
