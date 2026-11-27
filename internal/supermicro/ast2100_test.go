package supermicro

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
)

// ast2100Update is one AST2100 (0x57) rectangle: luma and chroma quantisation selectors, the
// subsampling mode (422 or 444) and the Aspeed bitstream.
func ast2100Update(w, h uint16, luma, chroma byte, subsampling uint16, bitstream []byte) []byte {
	return fbu(rect(0, 0, w, h, encodingAST2100, ast2100Payload(luma, chroma, subsampling, bitstream)))
}

func ast2100Payload(luma, chroma byte, subsampling uint16, bitstream []byte) []byte {
	return append([]byte{luma, chroma, byte(subsampling >> 8), byte(subsampling)}, bitstream...)
}

// endOfFrame is an Aspeed bitstream holding only the frame-end code: a valid frame that paints
// no block, so the picture stays at the decoder's initial white.
var endOfFrame = []byte{0, 0, 0, 0x90, 0, 0, 0, 0}

func TestSessionDecodesAST2100UpdatesAndRequestsTheNext(t *testing.T) {
	_, bmc, sink, _ := startSession(t, ServerInfo{Video: true, KeyboardMouse: true})
	go func() { _, _ = bmc.conn.Write(ast2100Update(16, 16, 4, 7, 422, endOfFrame)) }()
	waitFor(t, "a decoded frame", func() bool { n, _, _, _, _ := sink.snapshot(); return n > 0 })
	_, w, h, pix, _ := sink.snapshot()
	if w != 16 || h != 16 || len(pix) != 16*16*4 {
		t.Fatalf("screen %dx%d with %d bytes, want 16x16 RGBA", w, h, len(pix))
	}
	if got := rgbaAt(pix, 16, 0, 0); got != [4]byte{0xff, 0xff, 0xff, 0xff} {
		t.Errorf("pixel (0,0) = %v, want white", got)
	}
	waitFor(t, "a full update request after the mode change", func() bool {
		return bytes.Contains(bmc.sent(), framebufferUpdateRequest(false, 16, 16))
	})

	// A second frame, in the 4:4:4 mode, is applied to the same picture.
	go func() { _, _ = bmc.conn.Write(ast2100Update(16, 16, 0, 0, 444, endOfFrame)) }()
	waitFor(t, "a second frame", func() bool { n, _, _, _, _ := sink.snapshot(); return n > 1 })
	waitFor(t, "an incremental update request", func() bool {
		return bytes.Contains(bmc.sent(), framebufferUpdateRequest(true, 16, 16))
	})
}

// readFirstAspeedFrame reads the first frame of internal/aspeed's recorded C6220 stream (u32
// length, then 0x03, mode420, luma, chroma, 4 reserved bytes, u16 width, u16 height and the
// bitstream) and the digest Aspeed's reference decoder produced for it.
func readFirstAspeedFrame(t *testing.T) (w, h uint16, mode420, luma, chroma byte, bitstream []byte, want string) {
	t.Helper()
	data, err := os.ReadFile("../aspeed/testdata/c6220-pattern-1024x768.bin")
	if err != nil {
		t.Fatal(err)
	}
	n := binary.BigEndian.Uint32(data)
	msg := data[4 : 4+n]
	f, err := os.Open("../aspeed/testdata/c6220-pattern-1024x768.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("empty golden file")
	}
	return binary.BigEndian.Uint16(msg[8:10]), binary.BigEndian.Uint16(msg[10:12]),
		msg[1], msg[2], msg[3], msg[12:], strings.TrimSpace(sc.Text())
}

// An ATEN 0x57 rectangle wraps the same bitstream a Dell BMC's Aspeed engine sends, so a frame
// of the recorded C6220 stream must decode to exactly what Aspeed's reference produced for it.
func TestAST2100MatchesTheAspeedReference(t *testing.T) {
	w, h, mode420, luma, chroma, bitstream, want := readFirstAspeedFrame(t)
	subsampling := uint16(444)
	if mode420 == 1 {
		subsampling = 422
	}
	s := newSession(nil, nil, ServerInfo{Video: true, KeyboardMouse: true})
	sink := &recordingSink{}
	s.handleUpdate(FramebufferUpdate{Rects: []Rect{
		{W: w, H: h, Encoding: encodingAST2100, Data: ast2100Payload(luma, chroma, subsampling, bitstream)},
	}}, sink)
	frames, gotW, gotH, pix, _ := sink.snapshot()
	if frames != 1 || gotW != int(w) || gotH != int(h) {
		t.Fatalf("%d frames of %dx%d, want one of %dx%d", frames, gotW, gotH, w, h)
	}
	sum := sha256.Sum256(pix)
	if got := fmt.Sprintf("0 %dx%d %s", w, h, hex.EncodeToString(sum[:])); got != want {
		t.Fatalf("frame differs from the reference:\n got  %s\n want %s", got, want)
	}
}

func TestDecodeAST2100RejectsWhatItCannotDraw(t *testing.T) {
	tests := []struct {
		name string
		w, h int
		data []byte
	}{
		{"short header", 16, 16, []byte{4, 7, 1}},
		{"unknown subsampling", 16, 16, ast2100Payload(4, 7, 420, endOfFrame)},
		{"wider than the engine", 4096, 16, ast2100Payload(4, 7, 422, endOfFrame)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSession(nil, nil, ServerInfo{})
			s.frame = &frame{w: tt.w, h: tt.h}
			if err := s.decodeAST2100(tt.data); err == nil {
				t.Error("decoded, want an error")
			}
			if s.frame.pix != nil {
				t.Error("left a picture behind")
			}
		})
	}
}

// An update whose rectangles are 0x57 without data must not allocate a picture, as for Hermon.
func TestAST2100EmptyRectsDoNotAllocate(t *testing.T) {
	s := newSession(nil, nil, ServerInfo{Video: true, KeyboardMouse: true})
	sink := &recordingSink{}
	s.handleUpdate(FramebufferUpdate{Rects: []Rect{{W: 1920, H: 1200, Encoding: encodingAST2100}}}, sink)
	if s.frame != nil {
		t.Fatalf("frame = %+v, want none for an empty rect", s.frame)
	}
	if frames, _, _, _, _ := sink.snapshot(); frames != 0 {
		t.Fatalf("%d frames, want none", frames)
	}
}
