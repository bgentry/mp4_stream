package mp4

import (
	"fmt"
	"os"
	"encoding/binary"
)

const (
	BOX_HEADER_SIZE = int64(8)
)

func Open(path string) (f *File, err os.Error) {
	// fmt.Println(flag.Args())
	fmt.Println(path)

	file, err := os.Open(path, os.O_RDONLY, 0400)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil, err
	}

	f = &File{
		File: file,
	}

	return f, f.parse()
}

func (f *File) parse() (err os.Error) {
	info, err := f.Stat()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	fmt.Printf("File size: %v \n", info.Size)
	f.size = info.Size

	// Loop through top-level Atoms
	for offset := int64(0); offset < f.size; {
		size, name := f.ReadBoxAt(offset)
		fmt.Printf("Atom found:\nType: %v \nSize (bytes): %v \n", name, size)

		box := Box {
			name:					name,
			size:					int64(size),
			offsetStart:	offset,
			file:					f,
		}
		switch name {
		case "ftyp":
			f.ftyp = &FtypBox{ Box:box }
			f.ftyp.ParseContents()
		case "moov":
			f.moov = &MoovBox{ Box:box }
			f.moov.ParseContents()
		case "mdat":
			f.mdat = &box
		}

		offset += int64(size)
	}

	return nil
}

type File struct {
	*os.File
	ftyp *FtypBox
	moov *MoovBox
	mdat *Box
	size int64
}

func (f *File) ReadBoxAt(offset int64) (boxSize uint32, boxType string) {
	// Get Atom size
	buf := f.ReadBytesAt(BOX_HEADER_SIZE, offset)
	boxSize = binary.BigEndian.Uint32(buf[0:4])
	offset += BOX_HEADER_SIZE
	// Get Atom name
	boxType = string(buf[4:8])
	return boxSize, boxType
}

func (f *File) ReadBytesAt(n int64, offset int64) (word []byte) {
	buf := make([]byte, n)
	_, error := f.ReadAt(buf, offset)
	if error != nil {
		fmt.Println(error)
		return
	}
	return buf
}

func (f *File) ReadBoxData(b BoxInt) ([]byte) {
	if b.Size() <= BOX_HEADER_SIZE {
		return nil
	}
	return f.ReadBytesAt(b.Size() - BOX_HEADER_SIZE, b.OffsetStart() + BOX_HEADER_SIZE)
}

type BoxInt interface {
	Name() string
	Size() int64
	OffsetStart() int64
	ParseContents() (os.Error)
}

type Box struct {
	name string
	size, offsetStart int64
	file *File
}

func (b *Box) Name() (string) { return b.name }

func (b *Box) Size() (int64) { return b.size }

func (b *Box) OffsetStart() (int64) { return b.offsetStart }

func (b *Box) ParseContents() (os.Error) {
	return nil
}

type FtypBox struct {
	Box
	major_brand, minor_version string
	compatible_brands []string
}

func (b *FtypBox) ParseContents() (os.Error) {
	data := b.file.ReadBoxData(b)
	b.major_brand, b.minor_version = string(data[0:4]), string(data[4:8])
	if len(data) > 8 {
		for i := 8; i < len(data); i += 4 {
			b.compatible_brands = append(b.compatible_brands, string(data[i:i+4]))
		}
	}
	return nil
}

type MoovBox struct {
	Box
}

func (b *MoovBox) ParseContents() (os.Error) {
	return nil
}

type ContainerBox interface {
	ReadSubBoxes() (n int, err os.Error)
}

type LeafBox interface {
	ReadData() (n int, err os.Error)
	ParseData() (n int, err os.Error)
	ReadAndParseData() (n int, err os.Error)
}
