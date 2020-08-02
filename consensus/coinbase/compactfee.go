package coinbase // CompactFeeData is a specialized data type to store a compact list of fees
import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
)

// The following functions relate to storing and retrieving fee data from the database
// inside a block.
// Every transaction gets a single uint64 value, stored as a plain binary list.
// The transactions are ordered the same way they are ordered inside the block, making it easy
// to traverse every transaction in a block and extract its fee.
//
// compactFeeFactory is used to create such a list.
// compactFeeIterator is used to iterate over such a list.

type CompactFeeData []byte

func (cfd CompactFeeData) Len() int {
	return len(cfd) / 8
}

type compactFeeFactory struct {
	buffer *bytes.Buffer
	writer *bufio.Writer
}

func NewCompactFeeFactory() *compactFeeFactory {
	buffer := bytes.NewBuffer([]byte{})
	return &compactFeeFactory{
		buffer: buffer,
		writer: bufio.NewWriter(buffer),
	}
}

func (cfw *compactFeeFactory) Add(txFee uint64) error {
	return binary.Write(cfw.writer, binary.LittleEndian, txFee)
}

func (cfw *compactFeeFactory) Data() (CompactFeeData, error) {
	err := cfw.writer.Flush()

	return cfw.buffer.Bytes(), err
}

type compactFeeIterator struct {
	reader io.Reader
}

func (cfd CompactFeeData) iterator() *compactFeeIterator {
	return &compactFeeIterator{
		reader: bufio.NewReader(bytes.NewBuffer(cfd)),
	}
}

func (cfr *compactFeeIterator) next() (uint64, error) {
	var txFee uint64

	err := binary.Read(cfr.reader, binary.LittleEndian, &txFee)

	return txFee, err
}
