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

func (f *File) parse() (os.Error) {
	info, err := f.Stat()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	fmt.Printf("File size: %v \n", info.Size)
	f.size = info.Size

	// Loop through top-level Boxes
	boxes := readBoxes(f, int64(0), f.size)
	for box := range boxes {
		switch box.Name() {
		case "ftyp":
			f.ftyp = &FtypBox{ Box:box }
			f.ftyp.parse()
		case "moov":
			f.moov = &MoovBox{ Box:box }
			f.moov.parse()
		case "mdat":
			f.mdat = box
		default:
			fmt.Printf("Unhandled Box: %v \n", box.Name())
		}
	}
	return nil
}

func readBoxes(f *File, start int64, n int64) (boxes chan *Box) {
	boxes = make(chan *Box, 100)
	go func() {
		for offset := start; offset < start + n; {
			size, name := f.ReadBoxAt(offset)
			fmt.Printf("Box found:\nType: %v \nSize (bytes): %v \n", name, size)

			box := &Box {
				name:		name,
				size:		int64(size),
				start:	offset,
				file:		f,
			}
			boxes <- box
			offset += int64(size)
		}
		close(boxes)
	} ()
	return boxes
}

func readSubBoxes(f *File, start int64, n int64) (boxes chan *Box) {
	return readBoxes(f, start + BOX_HEADER_SIZE, n - BOX_HEADER_SIZE)
}

type File struct {
	*os.File
	ftyp *FtypBox
	moov *MoovBox
	mdat *Box
	size int64
}

func (f *File) ReadBoxAt(offset int64) (boxSize uint32, boxType string) {
	// Get Box size
	buf := f.ReadBytesAt(BOX_HEADER_SIZE, offset)
	boxSize = binary.BigEndian.Uint32(buf[0:4])
	offset += BOX_HEADER_SIZE
	// Get Box name
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

type BoxInt interface {
	Name() string
	File() *File
	Size() int64
	Start() int64
	parse() (os.Error)
}

type Box struct {
	name string
	size, start int64
	file *File
}

func (b *Box) Name() (string) { return b.name }

func (b *Box) Size() (int64) { return b.size }

func (b *Box) File() (*File) { return b.file }

func (b *Box) Start() (int64) { return b.start }

func (b *Box) parse() (os.Error) {
	fmt.Println("Default parser called; skip parsing.")
	return nil
}

func (b *Box) ReadBoxData() ([]byte) {
	if b.Size() <= BOX_HEADER_SIZE {
		return nil
	}
	return b.File().ReadBytesAt(b.Size() - BOX_HEADER_SIZE, b.Start() + BOX_HEADER_SIZE)
}

type FtypBox struct {
	*Box
	major_brand, minor_version string
	compatible_brands []string
}

func (b *FtypBox) parse() (os.Error) {
	data := b.ReadBoxData()
	b.major_brand, b.minor_version = string(data[0:4]), string(data[4:8])
	if len(data) > 8 {
		for i := 8; i < len(data); i += 4 {
			b.compatible_brands = append(b.compatible_brands, string(data[i:i+4]))
		}
	}
	return nil
}

type MoovBox struct {
	*Box
	mvhd *MvhdBox
	iods *IodsBox
}

func (b *MoovBox) parse() (os.Error) {
	boxes := readSubBoxes(b.File(), b.Start(), b.Size())
	for subBox := range boxes {
		switch subBox.Name() {
		case "mvhd":
			b.mvhd = &MvhdBox{ Box:subBox }
			b.mvhd.parse()
		case "iods":
			b.iods = &IodsBox{ Box:subBox }
			b.iods.parse()
		default:
			fmt.Printf("Unhandled Box: %v \n", subBox.Name())
		}
	}
	return nil
}

type MvhdBox struct {
	*Box
	version, creation_time, modification_time, timescale, duration, next_track_id uint32
	rate []byte
	volume []byte
	other_data []byte
}

func (b *MvhdBox) parse() (os.Error) {
	data := b.ReadBoxData()
	b.version = binary.BigEndian.Uint32(data[0:4])
	b.creation_time = binary.BigEndian.Uint32(data[4:8])
	b.modification_time = binary.BigEndian.Uint32(data[8:12])
	b.timescale = binary.BigEndian.Uint32(data[12:16])
	b.duration = binary.BigEndian.Uint32(data[16:20])
	b.rate = data[20:24]
	b.volume = data[24:26]
	b.other_data = data[26:]
	return nil
}

type IodsBox struct {
	*Box
	data []byte
}

func (b *IodsBox) parse() (os.Error) {
	b.data = b.ReadBoxData()
	return nil
}

type ContainerBox interface {
	ReadSubBoxes() (n int, err os.Error)
	HandleSubBox() (*Box, func(*Box))
}

type LeafBox interface {
	ReadData() (n int, err os.Error)
	ParseData() (n int, err os.Error)
	ReadAndParseData() (n int, err os.Error)
}
