// Copyright 2016 Google Inc. All Rights Reserved.
//
// Distributed under MIT license.
// See file LICENSE for detail or copy at https://opensource.org/licenses/MIT

package brotli

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/andybalholm/brotli/matchfinder"
	"github.com/xyproto/randomstring"
)

func checkCompressedData(compressedData, wantOriginalData []byte) error {
	uncompressed, err := Decode(compressedData)
	if err != nil {
		return fmt.Errorf("brotli decompress failed: %v", err)
	}
	if !bytes.Equal(uncompressed, wantOriginalData) {
		if len(wantOriginalData) != len(uncompressed) {
			return fmt.Errorf(""+
				"Data doesn't uncompress to the original value.\n"+
				"Length of original: %v\n"+
				"Length of uncompressed: %v",
				len(wantOriginalData), len(uncompressed))
		}
		for i := range wantOriginalData {
			if wantOriginalData[i] != uncompressed[i] {
				return fmt.Errorf(""+
					"Data doesn't uncompress to the original value.\n"+
					"Original at %v is %v\n"+
					"Uncompressed at %v is %v",
					i, wantOriginalData[i], i, uncompressed[i])
			}
		}
	}
	return nil
}

func TestEncoderNoWrite(t *testing.T) {
	out := bytes.Buffer{}
	e := NewWriterOptions(&out, WriterOptions{Quality: 5})
	if err := e.Close(); err != nil {
		t.Errorf("Close()=%v, want nil", err)
	}
	// Check Write after close.
	if _, err := e.Write([]byte("hi")); err == nil {
		t.Errorf("No error after Close() + Write()")
	}
}

func TestEncoderEmptyWrite(t *testing.T) {
	out := bytes.Buffer{}
	e := NewWriterOptions(&out, WriterOptions{Quality: 5})
	n, err := e.Write([]byte(""))
	if n != 0 || err != nil {
		t.Errorf("Write()=%v,%v, want 0, nil", n, err)
	}
	if err := e.Close(); err != nil {
		t.Errorf("Close()=%v, want nil", err)
	}
}
func TestIssue22(t *testing.T) {
	f, err := os.Open("testdata/issue22.gz")
	if err != nil {
		t.Fatalf("Error opening test data file: %v", err)
	}
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("Error creating gzip reader: %v", err)
	}

	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("Error reading test data: %v", err)
	}

	if len(data) != 2851073 {
		t.Fatalf("Wrong length for test data: got %d, want 2851073", len(data))
	}

	for level := BestSpeed; level <= BestCompression; level++ {
		out := bytes.Buffer{}
		e := NewWriterOptions(&out, WriterOptions{Quality: level})
		n, err := e.Write(data)
		if err != nil {
			t.Errorf("Error compressing data: %v", err)
		}
		if int(n) != len(data) {
			t.Errorf("Write() n=%v, want %v", n, len(data))
		}
		if err := e.Close(); err != nil {
			t.Errorf("Close Error after writing %d bytes: %v", n, err)
		}
		if err := checkCompressedData(out.Bytes(), data); err != nil {
			t.Errorf("Error decompressing data at level %d: %v", level, err)
		}
	}
}

func TestWriterV2(t *testing.T) {
	for level := BestSpeed; level <= BestCompression; level++ {
		// Test basic encoder usage.
		input := []byte("<html><body><H1>Hello world</H1></body></html>")
		out := bytes.Buffer{}
		e := NewWriterV2(&out, level)
		in := bytes.NewReader([]byte(input))
		n, err := io.Copy(e, in)
		if err != nil {
			t.Errorf("Copy Error: %v", err)
		}
		if int(n) != len(input) {
			t.Errorf("Copy() n=%v, want %v", n, len(input))
		}
		if err := e.Close(); err != nil {
			t.Errorf("Close Error after copied %d bytes: %v", n, err)
		}
		if err := checkCompressedData(out.Bytes(), input); err != nil {
			t.Error(err)
		}

		out2 := bytes.Buffer{}
		e.Reset(&out2)
		n2, err := e.Write(input)
		if err != nil {
			t.Errorf("Write error after Reset: %v", err)
		}
		if n2 != len(input) {
			t.Errorf("Write() after Reset n=%d, want %d", n2, len(input))
		}
		if err := e.Close(); err != nil {
			t.Errorf("Close error after Reset (copied %d) bytes: %v", n2, err)
		}
		if !bytes.Equal(out.Bytes(), out2.Bytes()) {
			t.Error("Compressed data after Reset doesn't equal first time")
		}
	}
}

func TestEncoderStreams(t *testing.T) {
	// Test that output is streamed.
	// Adjust window size to ensure the encoder outputs at least enough bytes
	// to fill the window.
	const lgWin = 16
	windowSize := int(math.Pow(2, lgWin))
	input := make([]byte, 8*windowSize)
	rand.Read(input)
	out := bytes.Buffer{}
	e := NewWriterOptions(&out, WriterOptions{Quality: 11, LGWin: lgWin})
	halfInput := input[:len(input)/2]
	in := bytes.NewReader(halfInput)

	n, err := io.Copy(e, in)
	if err != nil {
		t.Errorf("Copy Error: %v", err)
	}

	// We've fed more data than the sliding window size. Check that some
	// compressed data has been output.
	if out.Len() == 0 {
		t.Errorf("Output length is 0 after %d bytes written", n)
	}
	if err := e.Close(); err != nil {
		t.Errorf("Close Error after copied %d bytes: %v", n, err)
	}
	if err := checkCompressedData(out.Bytes(), halfInput); err != nil {
		t.Error(err)
	}
}

func TestEncoderLargeInput(t *testing.T) {
	for level := BestSpeed; level <= BestCompression; level++ {
		input := make([]byte, 1000000)
		rand.Read(input)
		out := bytes.Buffer{}
		e := NewWriterOptions(&out, WriterOptions{Quality: level})
		in := bytes.NewReader(input)

		n, err := io.Copy(e, in)
		if err != nil {
			t.Errorf("Copy Error: %v", err)
		}
		if int(n) != len(input) {
			t.Errorf("Copy() n=%v, want %v", n, len(input))
		}
		if err := e.Close(); err != nil {
			t.Errorf("Close Error after copied %d bytes: %v", n, err)
		}
		if err := checkCompressedData(out.Bytes(), input); err != nil {
			t.Error(err)
		}

		out2 := bytes.Buffer{}
		e.Reset(&out2)
		n2, err := e.Write(input)
		if err != nil {
			t.Errorf("Write error after Reset: %v", err)
		}
		if n2 != len(input) {
			t.Errorf("Write() after Reset n=%d, want %d", n2, len(input))
		}
		if err := e.Close(); err != nil {
			t.Errorf("Close error after Reset (copied %d) bytes: %v", n2, err)
		}
		if !bytes.Equal(out.Bytes(), out2.Bytes()) {
			t.Error("Compressed data after Reset doesn't equal first time")
		}
	}
}

func TestEncoderFlush(t *testing.T) {
	input := make([]byte, 1000)
	rand.Read(input)
	out := bytes.Buffer{}
	e := NewWriterOptions(&out, WriterOptions{Quality: 5})
	in := bytes.NewReader(input)
	_, err := io.Copy(e, in)
	if err != nil {
		t.Fatalf("Copy Error: %v", err)
	}
	if err := e.Flush(); err != nil {
		t.Fatalf("Flush(): %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("0 bytes written after Flush()")
	}
	decompressed := make([]byte, 1000)
	reader := NewReader(bytes.NewReader(out.Bytes()))
	n, err := reader.Read(decompressed)
	if n != len(decompressed) || err != nil {
		t.Errorf("Expected <%v, nil>, but <%v, %v>", len(decompressed), n, err)
	}
	if !bytes.Equal(decompressed, input) {
		t.Errorf(""+
			"Decompress after flush: %v\n"+
			"%q\n"+
			"want:\n%q",
			err, decompressed, input)
	}
	if err := e.Close(); err != nil {
		t.Errorf("Close(): %v", err)
	}
}

type readerWithTimeout struct {
	io.Reader
}

func (r readerWithTimeout) Read(p []byte) (int, error) {
	type result struct {
		n   int
		err error
	}
	ch := make(chan result)
	go func() {
		n, err := r.Reader.Read(p)
		ch <- result{n, err}
	}()
	select {
	case result := <-ch:
		return result.n, result.err
	case <-time.After(5 * time.Second):
		return 0, fmt.Errorf("read timed out")
	}
}

func TestDecoderStreaming(t *testing.T) {
	pr, pw := io.Pipe()
	writer := NewWriterOptions(pw, WriterOptions{Quality: 5, LGWin: 20})
	reader := readerWithTimeout{NewReader(pr)}
	defer func() {
		go ioutil.ReadAll(pr) // swallow the "EOF" token from writer.Close
		if err := writer.Close(); err != nil {
			t.Errorf("writer.Close: %v", err)
		}
	}()

	ch := make(chan []byte)
	errch := make(chan error)
	go func() {
		for {
			segment, ok := <-ch
			if !ok {
				return
			}
			if n, err := writer.Write(segment); err != nil || n != len(segment) {
				errch <- fmt.Errorf("write=%v,%v, want %v,%v", n, err, len(segment), nil)
				return
			}
			if err := writer.Flush(); err != nil {
				errch <- fmt.Errorf("flush: %v", err)
				return
			}
		}
	}()
	defer close(ch)

	segments := [...][]byte{
		[]byte("first"),
		[]byte("second"),
		[]byte("third"),
	}
	for k, segment := range segments {
		t.Run(fmt.Sprintf("Segment%d", k), func(t *testing.T) {
			select {
			case ch <- segment:
			case err := <-errch:
				t.Fatalf("write: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatalf("timed out")
			}
			wantLen := len(segment)
			got := make([]byte, wantLen)
			if n, err := reader.Read(got); err != nil || n != wantLen || !bytes.Equal(got, segment) {
				t.Fatalf("read[%d]=%q,%v,%v, want %q,%v,%v", k, got, n, err, segment, wantLen, nil)
			}
		})
	}
}

func TestReader(t *testing.T) {
	content := bytes.Repeat([]byte("hello world!"), 10000)
	encoded, _ := Encode(content, WriterOptions{Quality: 5})
	r := NewReader(bytes.NewReader(encoded))
	var decodedOutput bytes.Buffer
	n, err := io.Copy(&decodedOutput, r)
	if err != nil {
		t.Fatalf("Copy(): n=%v, err=%v", n, err)
	}
	if got := decodedOutput.Bytes(); !bytes.Equal(got, content) {
		t.Errorf(""+
			"Reader output:\n"+
			"%q\n"+
			"want:\n"+
			"<%d bytes>",
			got, len(content))
	}

	r.Reset(bytes.NewReader(encoded))
	decodedOutput.Reset()
	n, err = io.Copy(&decodedOutput, r)
	if err != nil {
		t.Fatalf("After Reset: Copy(): n=%v, err=%v", n, err)
	}
	if got := decodedOutput.Bytes(); !bytes.Equal(got, content) {
		t.Errorf("After Reset: "+
			"Reader output:\n"+
			"%q\n"+
			"want:\n"+
			"<%d bytes>",
			got, len(content))
	}

}

func TestDecode(t *testing.T) {
	content := bytes.Repeat([]byte("hello world!"), 10000)
	encoded, _ := Encode(content, WriterOptions{Quality: 5})
	decoded, err := Decode(encoded)
	if err != nil {
		t.Errorf("Decode: %v", err)
	}
	if !bytes.Equal(decoded, content) {
		t.Errorf(""+
			"Decode content:\n"+
			"%q\n"+
			"want:\n"+
			"<%d bytes>",
			decoded, len(content))
	}
}

func TestQuality(t *testing.T) {
	content := bytes.Repeat([]byte("hello world!"), 10000)
	for q := 0; q < 12; q++ {
		encoded, _ := Encode(content, WriterOptions{Quality: q})
		decoded, err := Decode(encoded)
		if err != nil {
			t.Errorf("Decode: %v", err)
		}
		if !bytes.Equal(decoded, content) {
			t.Errorf(""+
				"Decode content:\n"+
				"%q\n"+
				"want:\n"+
				"<%d bytes>",
				decoded, len(content))
		}
	}
}

func TestDecodeFuzz(t *testing.T) {
	// Test that the decoder terminates with corrupted input.
	content := bytes.Repeat([]byte("hello world!"), 100)
	rnd := rand.New(rand.NewSource(0))
	encoded, err := Encode(content, WriterOptions{Quality: 5})
	if err != nil {
		t.Fatalf("Encode(<%d bytes>, _) = _, %s", len(content), err)
	}
	if len(encoded) == 0 {
		t.Fatalf("Encode(<%d bytes>, _) produced empty output", len(content))
	}
	for i := 0; i < 100; i++ {
		enc := append([]byte{}, encoded...)
		for j := 0; j < 5; j++ {
			enc[rnd.Intn(len(enc))] = byte(rnd.Intn(256))
		}
		Decode(enc)
	}
}

func TestDecodeTrailingData(t *testing.T) {
	content := bytes.Repeat([]byte("hello world!"), 100)
	encoded, _ := Encode(content, WriterOptions{Quality: 5})
	_, err := Decode(append(encoded, 0))
	if err == nil {
		t.Errorf("Expected 'excessive input' error")
	}
}

// writeSingleSymbolPrefixCode writes a simple prefix code (RFC 7932, section
// 3.4) that contains only symbol, so decoding the symbol takes no bits.
func writeSingleSymbolPrefixCode(w *bitWriter, alphabetBits uint, symbol uint64) {
	w.writeBits(2, 1) // HSKIP = 1: simple prefix code
	w.writeBits(2, 0) // NSYM - 1
	w.writeBits(alphabetBits, symbol)
}

// zeroOutputCommandStream returns a stream with a single meta-block of
// 1<<24 insert-and-copy commands that produce no output: each one refers to
// static dictionary word 0 of length 4 with transform 34 (OmitFirst4).
// With a single block type, a meta-block can hold only 1<<24 commands and
// 1<<24 distances, so decoding the next command needs a block switch.
// With commandBlockTypes == 2, the first command block is longer than 1<<24,
// so the distance block switch is reached first.
func zeroOutputCommandStream(commandBlockTypes int, padding int) []byte {
	const numCommands = 1 << 24
	w := &bitWriter{dst: make([]byte, 0, numCommands*14/8+64+padding)}
	w.writeBits(1, 0)  // WBITS: 16
	w.writeBits(1, 1)  // ISLAST
	w.writeBits(1, 0)  // ISLASTEMPTY
	w.writeBits(2, 0)  // MNIBBLES: 4
	w.writeBits(16, 0) // MLEN - 1
	w.writeBits(1, 0)  // NBLTYPESL: 1
	if commandBlockTypes == 1 {
		w.writeBits(1, 0) // NBLTYPESI: 1
	} else {
		w.writeBits(1, 1)
		w.writeBits(3, 0)                     // NBLTYPESI: 2
		writeSingleSymbolPrefixCode(w, 2, 0)  // block type code
		writeSingleSymbolPrefixCode(w, 5, 25) // block count code: 16625 + 24 bits
		w.writeBits(24, numCommands+10-16625) // first block count
	}
	w.writeBits(1, 0) // NBLTYPESD: 1
	w.writeBits(2, 0) // NPOSTFIX
	w.writeBits(4, 0) // NDIRECT
	w.writeBits(2, 0) // context mode of literal block type 0
	w.writeBits(1, 0) // NTREESL: 1
	w.writeBits(1, 0) // NTREESD: 1
	writeSingleSymbolPrefixCode(w, 8, 'a')
	for i := 0; i < commandBlockTypes; i++ {
		// Command 130: no literals, copy length 4, explicit distance.
		writeSingleSymbolPrefixCode(w, 10, 130)
	}
	// Distance code 42: 32765 + 14 extra bits.
	writeSingleSymbolPrefixCode(w, 6, 42)
	for i := 0; i < numCommands; i++ {
		// At position 0, distance 34817 is dictionary reference 34816:
		// word 0 of length 4 with transform 34, which outputs nothing.
		w.writeBits(14, 34817-32765)
	}
	w.jumpToByteBoundary()
	return append(w.dst, make([]byte, padding)...)
}

// longLiteralRunStream returns a stream whose last meta-block holds two
// insert-and-copy commands with 1<<24 - 3 and 22594 literals. With a single
// block type, a meta-block can hold only 1<<24 literals, so the fourth literal
// of the second command needs a block switch. A preceding one-byte
// uncompressed meta-block keeps the ring buffer from wrapping right before it.
// With contextMap set, the literals are decoded with two prefix codes selected
// by the literal context instead of a single one.
func longLiteralRunStream(contextMap bool, padding int) []byte {
	w := &bitWriter{}
	w.writeBits(1, 1) // WBITS: 24
	w.writeBits(3, 7)

	w.writeBits(1, 0)  // ISLAST
	w.writeBits(2, 0)  // MNIBBLES: 4
	w.writeBits(16, 0) // MLEN - 1
	w.writeBits(1, 1)  // ISUNCOMPRESSED
	w.jumpToByteBoundary()
	w.dst = append(w.dst, 'x')

	w.writeBits(1, 1)        // ISLAST
	w.writeBits(1, 0)        // ISLASTEMPTY
	w.writeBits(2, 2)        // MNIBBLES: 6
	w.writeBits(24, 1<<24-1) // MLEN - 1
	w.writeBits(3, 0)        // NBLTYPESL, NBLTYPESI, NBLTYPESD: 1
	w.writeBits(2, 0)        // NPOSTFIX
	w.writeBits(4, 0)        // NDIRECT
	w.writeBits(2, 0)        // context mode of literal block type 0: LSB6
	if contextMap {
		w.writeBits(1, 1)
		w.writeBits(3, 0) // NTREESL: 2
		w.writeBits(1, 0) // RLEMAX: 0
		w.writeBits(2, 1) // HSKIP = 1: simple prefix code
		w.writeBits(2, 1) // NSYM - 1
		w.writeBits(1, 0)
		w.writeBits(1, 1)
		for i := 0; i < 1<<literalContextBits; i++ {
			w.writeBits(1, uint64(i&1))
		}
		w.writeBits(1, 0) // IMTF
	} else {
		w.writeBits(1, 0) // NTREESL: 1
	}
	w.writeBits(1, 0) // NTREESD: 1
	writeSingleSymbolPrefixCode(w, 8, 'a')
	if contextMap {
		writeSingleSymbolPrefixCode(w, 8, 'b')
	}
	// Command 504: 22594 + 24 extra bits literals, copy length 2, explicit distance.
	writeSingleSymbolPrefixCode(w, 10, 504)
	// Distance code 0: the last distance, 4.
	writeSingleSymbolPrefixCode(w, 6, 0)
	w.writeBits(24, 1<<24-3-22594)
	w.writeBits(24, 0)
	w.jumpToByteBoundary()
	return append(w.dst, make([]byte, padding)...)
}

func TestDecodeInvalidBlockSwitch(t *testing.T) {
	// A category with a single block type has an implicit block length of
	// 1<<24, and a stream that runs past it is invalid. The decoder must report
	// this instead of ignoring the failed block switch; for commands that used
	// to loop forever without consuming input.
	// The padding keeps enough input available for the fast decoding path.
	for _, test := range []struct {
		name   string
		stream func() []byte
	}{
		{"Command", func() []byte { return zeroOutputCommandStream(1, 64) }},
		{"CommandAtEndOfInput", func() []byte { return zeroOutputCommandStream(1, 0) }},
		{"Distance", func() []byte { return zeroOutputCommandStream(2, 64) }},
		{"Literal", func() []byte { return longLiteralRunStream(false, 64) }},
		{"LiteralAtEndOfInput", func() []byte { return longLiteralRunStream(false, 0) }},
		{"LiteralContextMap", func() []byte { return longLiteralRunStream(true, 64) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream := test.stream()
			done := make(chan error, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						done <- fmt.Errorf("panic: %v", r)
					}
				}()
				_, err := io.Copy(io.Discard, NewReader(bytes.NewReader(stream)))
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil || err.Error() != "brotli: BLOCK_SWITCH" {
					t.Errorf("Decode: got error %v, want brotli: BLOCK_SWITCH", err)
				}
			case <-time.After(time.Minute):
				t.Errorf("Decode did not finish within a minute")
			}
		})
	}
}

func TestEncodeDecode(t *testing.T) {
	for _, test := range []struct {
		data    []byte
		repeats int
	}{
		{nil, 0},
		{[]byte("A"), 1},
		{[]byte("<html><body><H1>Hello world</H1></body></html>"), 10},
		{[]byte("<html><body><H1>Hello world</H1></body></html>"), 1000},
	} {
		t.Logf("case %q x %d", test.data, test.repeats)
		input := bytes.Repeat(test.data, test.repeats)
		encoded, err := Encode(input, WriterOptions{Quality: 5})
		if err != nil {
			t.Errorf("Encode: %v", err)
		}
		// Inputs are compressible, but may be too small to compress.
		if maxSize := len(input)/2 + 20; len(encoded) >= maxSize {
			t.Errorf(""+
				"Encode returned %d bytes, want <%d\n"+
				"Encoded=%q",
				len(encoded), maxSize, encoded)
		}
		decoded, err := Decode(encoded)
		if err != nil {
			t.Errorf("Decode: %v", err)
		}
		if !bytes.Equal(decoded, input) {
			var want string
			if len(input) > 320 {
				want = fmt.Sprintf("<%d bytes>", len(input))
			} else {
				want = fmt.Sprintf("%q", input)
			}
			t.Errorf(""+
				"Decode content:\n"+
				"%q\n"+
				"want:\n"+
				"%s",
				decoded, want)
		}
	}
}

func TestErrorReset(t *testing.T) {
	compress := func(input []byte) []byte {
		var buf bytes.Buffer
		writer := new(Writer)
		writer.Reset(&buf)
		writer.Write(input)
		writer.Close()

		return buf.Bytes()
	}

	corruptReader := func(reader *Reader) {
		buf := bytes.NewBuffer([]byte("trash"))
		reader.Reset(buf)
		_, err := io.ReadAll(reader)
		if err == nil {
			t.Fatalf("successively decompressed invalid input")
		}
	}

	decompress := func(input []byte, reader *Reader) []byte {
		buf := bytes.NewBuffer(input)
		reader.Reset(buf)
		output, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("failed to decompress data %s", err.Error())
		}

		return output
	}

	source := []byte("text")

	compressed := compress(source)
	reader := new(Reader)
	corruptReader(reader)
	decompressed := decompress(compressed, reader)
	if string(source) != string(decompressed) {
		t.Fatalf("decompressed data does not match original state")
	}
}

// Encode returns content encoded with Brotli.
func Encode(content []byte, options WriterOptions) ([]byte, error) {
	var buf bytes.Buffer
	writer := NewWriterOptions(&buf, options)
	_, err := writer.Write(content)
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	return buf.Bytes(), err
}

// Decode decodes Brotli encoded data.
func Decode(encodedData []byte) ([]byte, error) {
	r := NewReader(bytes.NewReader(encodedData))
	return ioutil.ReadAll(r)
}

func BenchmarkEncodeLevels(b *testing.B) {
	opticks, err := ioutil.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		b.Fatal(err)
	}

	for level := BestSpeed; level <= BestCompression; level++ {
		b.Run(fmt.Sprintf("%d", level), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(opticks)))
			for i := 0; i < b.N; i++ {
				w := NewWriterLevel(ioutil.Discard, level)
				w.Write(opticks)
				w.Close()
			}
		})
	}
}

func BenchmarkEncodeLevelsReset(b *testing.B) {
	opticks, err := ioutil.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		b.Fatal(err)
	}

	for level := BestSpeed; level <= BestCompression; level++ {
		buf := new(bytes.Buffer)
		w := NewWriterLevel(buf, level)
		w.Write(opticks)
		w.Close()
		b.Run(fmt.Sprintf("%d", level), func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(float64(len(opticks))/float64(buf.Len()), "ratio")
			b.SetBytes(int64(len(opticks)))
			for i := 0; i < b.N; i++ {
				w.Reset(ioutil.Discard)
				w.Write(opticks)
				w.Close()
			}
		})
	}
}

func BenchmarkEncodeLevelsResetV2(b *testing.B) {
	opticks, err := ioutil.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		b.Fatal(err)
	}

	for level := BestSpeed; level <= 9; level++ {
		buf := new(bytes.Buffer)
		w := NewWriterV2(buf, level)
		w.Write(opticks)
		w.Close()
		b.Run(fmt.Sprintf("%d", level), func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(float64(len(opticks))/float64(buf.Len()), "ratio")
			b.SetBytes(int64(len(opticks)))
			for i := 0; i < b.N; i++ {
				w.Reset(ioutil.Discard)
				w.Write(opticks)
				w.Close()
			}
		})
	}
}

func BenchmarkDecodeLevels(b *testing.B) {
	opticks, err := ioutil.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		b.Fatal(err)
	}

	for level := BestSpeed; level <= BestCompression; level++ {
		buf := new(bytes.Buffer)
		w := NewWriterLevel(buf, level)
		w.Write(opticks)
		w.Close()
		compressed := buf.Bytes()
		b.Run(fmt.Sprintf("%d", level), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(opticks)))
			for i := 0; i < b.N; i++ {
				io.Copy(ioutil.Discard, NewReader(bytes.NewReader(compressed)))
			}
		})
	}
}

func test(t *testing.T, filename string, m matchfinder.MatchFinder, blockSize int) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	b := new(bytes.Buffer)
	w := &matchfinder.Writer{
		Dest:        b,
		MatchFinder: m,
		Encoder:     &Encoder{},
		BlockSize:   blockSize,
	}
	w.Write(data)
	w.Close()
	compressed := b.Bytes()
	sr := NewReader(bytes.NewReader(compressed))
	decompressed, err := ioutil.ReadAll(sr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decompressed, data) {
		t.Fatal("decompressed output doesn't match")
	}
}

func benchmark(b *testing.B, filename string, m matchfinder.MatchFinder, blockSize int) {
	b.StopTimer()
	b.ReportAllocs()
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(len(data)))
	buf := new(bytes.Buffer)
	w := &matchfinder.Writer{
		Dest:        buf,
		MatchFinder: m,
		Encoder:     &Encoder{},
		BlockSize:   blockSize,
	}
	w.Write(data)
	w.Close()
	b.ReportMetric(float64(len(data))/float64(buf.Len()), "ratio")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		w.Reset(ioutil.Discard)
		w.Write(data)
		w.Close()
	}
}

func TestEncodeM4(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 18, DistanceBitCost: 66}, 1<<16)
}

func TestEncodeM4Chain256(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 18, DistanceBitCost: 66, ChainLength: 256}, 1<<16)
}

func BenchmarkEncodeM4(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, DistanceBitCost: 66}, 1<<16)
}

func TestEncodeM4Chain1(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 18, ChainLength: 1, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain1(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 1, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain2(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 2, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain4(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 4, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain8(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 8, HashLen: 5, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain16(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 16, HashLen: 5, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain32(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 32, HashLen: 5, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain64(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 64, HashLen: 5, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain128(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 128, HashLen: 5, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain256(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 256, HashLen: 5, DistanceBitCost: 66}, 1<<16)
}

func TestEncodePathfinder(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 18}, 1<<16)
}

func TestEncodePathfinderChain256(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 18, ChainLength: 256}, 1<<16)
}

func BenchmarkEncodePathfinder(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20}, 1<<16)
}

func TestEncodePathfinderChain1(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 18, ChainLength: 1}, 1<<16)
}

func BenchmarkEncodePathfinderChain1(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 1}, 1<<16)
}

func BenchmarkEncodePathfinderChain2(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 2}, 1<<16)
}

func BenchmarkEncodePathfinderChain4(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 4}, 1<<16)
}

func BenchmarkEncodePathfinderChain8(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 8, HashLen: 5}, 1<<16)
}

func BenchmarkEncodePathfinderChain16(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 16, HashLen: 5}, 1<<16)
}

func BenchmarkEncodePathfinderChain32(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 32, HashLen: 5}, 1<<16)
}

func BenchmarkEncodePathfinderChain64(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 64, HashLen: 5}, 1<<16)
}

func BenchmarkEncodePathfinderChain128(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 128, HashLen: 5}, 1<<16)
}

func BenchmarkEncodePathfinderChain256(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 256, HashLen: 4, MinLength: 3}, 1<<16)
}

func TestEncodeM0(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", matchfinder.M0{}, 1<<16)
}

func BenchmarkEncodeM0(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", matchfinder.M0{}, 1<<16)
}

func TestEncodeM0Lazy(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", matchfinder.M0{Lazy: true}, 1<<16)
}

func BenchmarkEncodeM0Lazy(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", matchfinder.M0{Lazy: true}, 1<<16)
}

func TestIssue51(t *testing.T) {
	for i := 65536; i <= 65536*4; i += 65536 {
		t.Run("compress data length: "+strconv.Itoa(i)+"bytes", func(t *testing.T) {
			dataStr := randomstring.HumanFriendlyString(i)
			dataBytes := []byte(dataStr)
			buf := bytes.Buffer{}
			w := NewWriterV2(&buf, 4)

			n, err := w.Write(dataBytes)
			if err != nil {
				t.Fatalf("Error while compressing data: %v", err)
			}
			if n != len(dataBytes) {
				t.Fatalf("Bytes written (%d) != len(databytes) (%d)", n, len(dataBytes))
			}
			err = w.Close()
			if err != nil {
				t.Fatalf("Error closing writer: %v", err)
			}

			r := NewReader(&buf)
			dst := make([]byte, len(dataBytes)+100)
			p := dst
			total := 0
			for {
				n1, err1 := r.Read(p)
				if err1 != nil {
					if err1 != io.EOF {
						t.Fatal(err1)
					}
					break
				}
				total += n1
				p = p[n1:]
			}
			if !bytes.Equal(dst[:total], dataBytes) {
				t.Fatal("Decompressed bytes don't match")
			}
		})
	}
}

func TestIssue58(t *testing.T) {
	content := []byte("---\nthis-is-not-brotli: \"it is actually yaml\"")
	input := bytes.NewBuffer(content)

	r := NewReader(input)

	buf, err := io.ReadAll(r)
	if err == nil {
		t.Fatalf("expected error, got none and read:\n%x\n%s\n%v", buf, buf, buf)
	}
}
