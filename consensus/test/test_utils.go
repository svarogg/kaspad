package test

import (
	"compress/bzip2"
	"encoding/binary"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/wire"
	"io"
	"os"
	"strings"
)

// LoadBlocks reads files containing kaspa gzipped block data from disk
// and returns them as an array of util.Block.
func LoadBlocks(filename string) (blocks []*util.Block, err error) {
	var network = wire.Mainnet
	var dr io.Reader
	var fi io.ReadCloser

	fi, err = os.Open(filename)
	if err != nil {
		return
	}

	if strings.HasSuffix(filename, ".bz2") {
		dr = bzip2.NewReader(fi)
	} else {
		dr = fi
	}
	defer fi.Close()

	var block *util.Block

	err = nil
	for height := uint64(0); err == nil; height++ {
		var rintbuf uint32
		err = binary.Read(dr, binary.LittleEndian, &rintbuf)
		if err == io.EOF {
			// hit end of file at expected offset: no warning
			height--
			err = nil
			break
		}
		if err != nil {
			break
		}
		if rintbuf != uint32(network) {
			break
		}
		err = binary.Read(dr, binary.LittleEndian, &rintbuf)
		blocklen := rintbuf

		rbytes := make([]byte, blocklen)

		// read block
		dr.Read(rbytes)

		block, err = util.NewBlockFromBytes(rbytes)
		if err != nil {
			return
		}
		blocks = append(blocks, block)
	}

	return
}
