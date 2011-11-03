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

	file, err := os.OpenFile(path, os.O_RDONLY, 0400)
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

	// Make sure we have all 3 required boxes
	if f.ftyp == nil || f.moov == nil || f.mdat == nil {
		return os.NewError("Missing a required box (ftyp, moov, or mdat)")
	}

	// Build chunk & sample tables
	fmt.Println("Building trak tables...")
	if err = f.buildTrakTables(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	fmt.Println("Chunk and Sample tables built.")

	return nil
}

func (f *File) buildTrakTables() (os.Error) {
	for _, trak := range f.moov.traks {
		trak.chunks = make([]Chunk, trak.mdia.minf.stbl.stco.entry_count)
		for i, offset := range trak.mdia.minf.stbl.stco.chunk_offset {
			trak.chunks[i].offset = offset
		}

		sample_num := uint32(1)
		next_chunk_id := 1
		for i := 0; i < int(trak.mdia.minf.stbl.stsc.entry_count); i++ {
			if i + 1 < int(trak.mdia.minf.stbl.stsc.entry_count) {
				next_chunk_id = int(trak.mdia.minf.stbl.stsc.first_chunk[i+1])
			} else
			{
				next_chunk_id = len(trak.chunks)
			}
			first_chunk_id := trak.mdia.minf.stbl.stsc.first_chunk[i]
			n_samples := trak.mdia.minf.stbl.stsc.samples_per_chunk[i]
			sdi := trak.mdia.minf.stbl.stsc.sample_description_index[i]
			for j := int(first_chunk_id-1); j < next_chunk_id; j++ {
				trak.chunks[j].sample_count = n_samples
				trak.chunks[j].sample_description_index = sdi
				trak.chunks[j].start_sample = sample_num
				sample_num += n_samples
			}
		}

		sample_count := int(trak.mdia.minf.stbl.stsz.sample_count)
		trak.samples = make([]Sample, sample_count)
		sample_size := trak.mdia.minf.stbl.stsz.sample_size
		for i := 0; i < sample_count; i++ {
			if sample_size == uint32(0) {
				trak.samples[i].size = trak.mdia.minf.stbl.stsz.entry_size[i]
			} else
			{
				trak.samples[i].size = sample_size
			}
		}

		// Calculate file offset for each sample
		sample_id := 0
		for i := 0; i < len(trak.chunks); i++ {
			sample_offset := trak.chunks[i].offset
			for j := 0; j < int(trak.chunks[i].sample_count); j++ {
				sample_offset += trak.samples[sample_id].size
				sample_id++
			}
		}

		// Calculate decoding time for each sample
		sample_id, sample_time := 0, uint32(0)
		for i := 0; i < int(trak.mdia.minf.stbl.stts.entry_count); i++ {
			sample_duration := trak.mdia.minf.stbl.stts.sample_delta[i]
			for j := 0; j < int(trak.mdia.minf.stbl.stts.sample_count[i]); j++ {
				trak.samples[sample_id].start_time = sample_time
				trak.samples[sample_id].duration = sample_duration
				sample_time += sample_duration
				sample_id++
			}
		}
		// Calculate decoding to composition time offset, if ctts table exists
		if trak.mdia.minf.stbl.ctts != nil {
			sample_id = 0
			for i := 0; i < int(trak.mdia.minf.stbl.ctts.entry_count); i++ {
				count := int(trak.mdia.minf.stbl.ctts.sample_count[i])
				cto := trak.mdia.minf.stbl.ctts.sample_offset[i]
				for j := 0; j < count; j++ {
					trak.samples[sample_id].cto = cto
					sample_id++
				}
			}
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
	if _, error := f.ReadAt(buf, offset); error != nil {
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
	parse() os.Error
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
	fmt.Printf("Default parser called; skip parsing. (%v)\n", b.name)
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
	traks []*TrakBox
	udta *UdtaBox
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
		case "trak":
			trak := &TrakBox{ Box:subBox }
			trak.parse()
			b.traks = append(b.traks, trak)
		case "udta":
			b.udta = &UdtaBox{ Box:subBox }
			b.udta.parse()
		default:
			fmt.Printf("Unhandled Moov Sub-Box: %v \n", subBox.Name())
		}
	}
	return nil
}

type MvhdBox struct {
	*Box
	version uint8
	flags [3]byte
	creation_time, modification_time, timescale, duration, next_track_id uint32
	rate Fixed32
	volume Fixed16
	other_data []byte
}

func (b *MvhdBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{data[1], data[2], data[3]}
	b.creation_time = binary.BigEndian.Uint32(data[4:8])
	b.modification_time = binary.BigEndian.Uint32(data[8:12])
	b.timescale = binary.BigEndian.Uint32(data[12:16])
	b.duration = binary.BigEndian.Uint32(data[16:20])
	b.rate, err = MakeFixed32(data[20:24])
	if err != nil {
		return err
	}
	b.volume, err = MakeFixed16(data[24:26])
	if err != nil {
		return err
	}
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

type TrakBox struct {
	*Box
	tkhd *TkhdBox
	mdia *MdiaBox
	edts *EdtsBox
	chunks []Chunk
	samples []Sample
}

func (b *TrakBox) parse() (os.Error) {
	boxes := readSubBoxes(b.File(), b.Start(), b.Size())
	for subBox := range boxes {
		switch subBox.Name() {
		case "tkhd":
			b.tkhd = &TkhdBox{ Box:subBox }
			b.tkhd.parse()
		case "mdia":
			b.mdia = &MdiaBox{ Box:subBox }
			b.mdia.parse()
		case "edts":
			b.edts = &EdtsBox{ Box:subBox }
			b.edts.parse()
		default:
			fmt.Printf("Unhandled Trak Sub-Box: %v \n", subBox.Name())
		}
	}
	return nil
}

type TkhdBox struct {
	*Box
	version uint8
	flags [3]byte
	creation_time, modification_time, track_id, duration uint32
	layer, alternate_group uint16 // This should really be int16 but not sure how to parse
	volume Fixed16
	matrix []byte
	width, height Fixed32
}

func (b *TkhdBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.creation_time = binary.BigEndian.Uint32(data[4:8])
	b.modification_time = binary.BigEndian.Uint32(data[8:12])
	b.track_id = binary.BigEndian.Uint32(data[12:16])
	// Skip 4 bytes for reserved space (uint32)
	b.duration = binary.BigEndian.Uint32(data[20:24])
	// Skip 8 bytes for reserved space (2 uint32)
	b.layer = binary.BigEndian.Uint16(data[32:34])
	b.alternate_group = binary.BigEndian.Uint16(data[34:36])
	b.volume, err = MakeFixed16(data[36:38])
	if err != nil {
		return err
	}
	// Skip 2 bytes for reserved space (uint16)
	b.matrix = data[40:76]
	b.width, err = MakeFixed32(data[76:80])
	if err != nil {
		return err
	}
	b.height, err = MakeFixed32(data[80:84])
	if err != nil {
		return err
	}
	return nil
}

type EdtsBox struct {
	*Box
	elst *ElstBox
}

func (b *EdtsBox) parse() (err os.Error) {
	boxes := readSubBoxes(b.File(), b.Start(), b.Size())
	for subBox := range boxes {
		switch subBox.Name() {
		case "elst":
			b.elst = &ElstBox{ Box:subBox }
			err = b.elst.parse()
		default:
			fmt.Printf("Unhandled Edts Sub-Box: %v \n", subBox.Name())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

type ElstBox struct {
	*Box
	version uint8
	flags [3]byte
	entry_count uint32
	segment_duration, media_time []uint32
	media_rate_integer, media_rate_fraction []uint16 // This should really be int16 but not sure how to parse
}

func (b *ElstBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.entry_count = binary.BigEndian.Uint32(data[4:8])
	for i := 0; i < int(b.entry_count); i++ {
		sd := binary.BigEndian.Uint32(data[(8+12*i):(12+12*i)])
		mt := binary.BigEndian.Uint32(data[(12+12*i):(16+12*i)])
		mri := binary.BigEndian.Uint16(data[(16+12*i):(18+12*i)])
		mrf := binary.BigEndian.Uint16(data[(18+12*i):(20+12*i)])
		b.segment_duration = append(b.segment_duration, sd)
		b.media_time = append(b.media_time, mt)
		b.media_rate_integer = append(b.media_rate_integer, mri)
		b.media_rate_fraction = append(b.media_rate_fraction, mrf)
	}
	return nil
}

type MdiaBox struct {
	*Box
	mdhd *MdhdBox
	hdlr *HdlrBox
	minf *MinfBox
}

func (b *MdiaBox) parse() (os.Error) {
	boxes := readSubBoxes(b.File(), b.Start(), b.Size())
	for subBox := range boxes {
		switch subBox.Name() {
		case "mdhd":
			b.mdhd = &MdhdBox{ Box:subBox }
			b.mdhd.parse()
		case "hdlr":
			b.hdlr = &HdlrBox{ Box:subBox }
			b.hdlr.parse()
		case "minf":
			b.minf = &MinfBox{ Box:subBox }
			b.minf.parse()
		default:
			fmt.Printf("Unhandled Mdia Sub-Box: %v \n", subBox.Name())
		}
	}
	return nil
}

type MdhdBox struct {
	*Box
	version uint8
	flags [3]byte
	creation_time, modification_time, timescale, duration uint32
	language uint16 // Combine 1-bit padding w/ 15-bit language data
}

func (b *MdhdBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.creation_time = binary.BigEndian.Uint32(data[4:8])
	b.modification_time = binary.BigEndian.Uint32(data[8:12])
	b.timescale = binary.BigEndian.Uint32(data[12:16])
	b.duration = binary.BigEndian.Uint32(data[16:20])
	// language includes 1 padding bit
	b.language = binary.BigEndian.Uint16(data[20:22])
	return nil
}

type HdlrBox struct {
	*Box
	version uint8
	flags [3]byte
	pre_defined uint32
	handler_type, track_name string
}

func (b *HdlrBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.pre_defined = binary.BigEndian.Uint32(data[4:8])
	b.handler_type = string(data[8:12])
	// Skip 12 bytes for reserved space (3 uint32)
	b.track_name = string(data[24:])
	return nil
}

type MinfBox struct {
	*Box
	vmhd *VmhdBox
	smhd *SmhdBox
	stbl *StblBox
	dinf *DinfBox
	hdlr *HdlrBox
}

func (b *MinfBox) parse() (err os.Error) {
	boxes := readSubBoxes(b.File(), b.Start(), b.Size())
	for subBox := range boxes {
		switch subBox.Name() {
		case "vmhd":
			b.vmhd = &VmhdBox{ Box:subBox }
			err = b.vmhd.parse()
		case "smhd":
			b.smhd = &SmhdBox{ Box:subBox }
			err = b.smhd.parse()
		case "stbl":
			b.stbl = &StblBox{ Box:subBox }
			err = b.stbl.parse()
		case "dinf":
			b.dinf = &DinfBox{ Box:subBox }
			err = b.dinf.parse()
		case "hdlr":
			b.hdlr = &HdlrBox{ Box:subBox }
			err = b.hdlr.parse()
		default:
			fmt.Printf("Unhandled Minf Sub-Box: %v \n", subBox.Name())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

type VmhdBox struct {
	*Box
	version uint8
	flags [3]byte
	graphicsmode uint16
	opcolor [3]uint16
}

func (b *VmhdBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.graphicsmode = binary.BigEndian.Uint16(data[4:6])
	for i := 0; i < 3; i++ {
		b.opcolor[i] = binary.BigEndian.Uint16(data[(6+2*i):(8+2*i)])
	}
	return nil
}

type SmhdBox struct {
	*Box
	version uint8
	flags [3]byte
	balance uint16 // This should really be int16 but not sure how to parse
}

func (b *SmhdBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.balance = binary.BigEndian.Uint16(data[4:6])
	return nil
}

type StblBox struct {
	*Box
	stsd *StsdBox
	stts *SttsBox
	stss *StssBox
	stsc *StscBox
	stsz *StszBox
	stco *StcoBox
	ctts *CttsBox
}

func (b *StblBox) parse() (err os.Error) {
	boxes := readSubBoxes(b.File(), b.Start(), b.Size())
	for subBox := range boxes {
		switch subBox.Name() {
		case "stsd":
			b.stsd = &StsdBox{ Box:subBox }
			err = b.stsd.parse()
		case "stts":
			b.stts = &SttsBox{ Box:subBox }
			err = b.stts.parse()
		case "stss":
			b.stss = &StssBox{ Box:subBox }
			err = b.stss.parse()
		case "stsc":
			b.stsc = &StscBox{ Box:subBox }
			err = b.stsc.parse()
		case "stsz":
			b.stsz = &StszBox{ Box:subBox }
			err = b.stsz.parse()
		case "stco":
			b.stco = &StcoBox{ Box:subBox }
			err = b.stco.parse()
		case "ctts":
			b.ctts = &CttsBox{ Box:subBox }
			err = b.ctts.parse()
		default:
			fmt.Printf("Unhandled Stbl Sub-Box: %v \n", subBox.Name())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

type StsdBox struct {
	*Box
	version uint8
	flags [3]byte
	entry_count uint32
	other_data []byte
}

func (b *StsdBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.entry_count = binary.BigEndian.Uint32(data[4:8])
	b.other_data = data[8:]
	fmt.Println("stsd box parsing not yet finished")
	return nil
}

type SttsBox struct {
	*Box
	version uint8
	flags [3]byte
	entry_count uint32
	sample_count []uint32
	sample_delta []uint32
}

func (b *SttsBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.entry_count = binary.BigEndian.Uint32(data[4:8])
	for i := 0; i < int(b.entry_count); i++ {
		s_count := binary.BigEndian.Uint32(data[(8+8*i):(12+8*i)])
		s_delta := binary.BigEndian.Uint32(data[(12+8*i):(16+8*i)])
		b.sample_count = append(b.sample_count, s_count)
		b.sample_delta = append(b.sample_delta, s_delta)
	}
	return nil
}

type StssBox struct {
	*Box
	version uint8
	flags [3]byte
	entry_count uint32
	sample_number []uint32
}

func (b *StssBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.entry_count = binary.BigEndian.Uint32(data[4:8])
	for i := 0; i < int(b.entry_count); i++ {
		sample := binary.BigEndian.Uint32(data[(8+4*i):(12+4*i)])
		b.sample_number = append(b.sample_number, sample)
	}
	return nil
}

type StscBox struct {
	*Box
	version uint8
	flags [3]byte
	entry_count uint32
	first_chunk []uint32
	samples_per_chunk []uint32
	sample_description_index []uint32
}

func (b *StscBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.entry_count = binary.BigEndian.Uint32(data[4:8])
	for i := 0; i < int(b.entry_count); i++ {
		fc := binary.BigEndian.Uint32(data[(8+12*i):(12+12*i)])
		spc := binary.BigEndian.Uint32(data[(12+12*i):(16+12*i)])
		sdi := binary.BigEndian.Uint32(data[(16+12*i):(20+12*i)])
		b.first_chunk = append(b.first_chunk, fc)
		b.samples_per_chunk = append(b.samples_per_chunk, spc)
		b.sample_description_index = append(b.sample_description_index, sdi)
	}
	return nil
}

type StszBox struct {
	*Box
	version uint8
	flags [3]byte
	sample_size uint32
	sample_count uint32
	entry_size []uint32
}

func (b *StszBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.sample_size = binary.BigEndian.Uint32(data[4:8])
	b.sample_count = binary.BigEndian.Uint32(data[8:12])
	if b.sample_size == uint32(0) {
		for i := 0; i < int(b.sample_count); i++ {
			entry := binary.BigEndian.Uint32(data[(12+4*i):(16+4*i)])
			b.entry_size = append(b.entry_size, entry)
		}
	}
	return nil
}

type StcoBox struct {
	*Box
	version uint8
	flags [3]byte
	entry_count uint32
	chunk_offset []uint32
}

func (b *StcoBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.entry_count = binary.BigEndian.Uint32(data[4:8])
	for i := 0; i < int(b.entry_count); i++ {
		chunk := binary.BigEndian.Uint32(data[(8+4*i):(12+4*i)])
		b.chunk_offset = append(b.chunk_offset, chunk)
	}
	return nil
}

type CttsBox struct {
	*Box
	version uint8
	flags [3]byte
	entry_count uint32
	sample_count []uint32
	sample_offset []uint32
}

func (b *CttsBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.entry_count = binary.BigEndian.Uint32(data[4:8])
	for i := 0; i < int(b.entry_count); i++ {
		s_count := binary.BigEndian.Uint32(data[(8+8*i):(12+8*i)])
		s_offset := binary.BigEndian.Uint32(data[(12+8*i):(16+8*i)])
		b.sample_count = append(b.sample_count, s_count)
		b.sample_offset = append(b.sample_offset, s_offset)
	}
	return nil
}

type DinfBox struct {
	*Box
	dref *DrefBox
}

func (b *DinfBox) parse() (err os.Error) {
	boxes := readSubBoxes(b.File(), b.Start(), b.Size())
	for subBox := range boxes {
		switch subBox.Name() {
		case "dref":
			b.dref = &DrefBox{ Box:subBox }
			err = b.dref.parse()
		default:
			fmt.Printf("Unhandled Dinf Sub-Box: %v \n", subBox.Name())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

type DrefBox struct {
	*Box
	version uint8
	flags [3]byte
	entry_count uint32
	other_data []byte
}

func (b *DrefBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.entry_count = binary.BigEndian.Uint32(data[4:8])
	b.other_data = data[8:]
	fmt.Println("dref box parsing not yet finished")
	return nil
}

type UdtaBox struct {
	*Box
	meta *MetaBox
}

func (b *UdtaBox) parse() (err os.Error) {
	boxes := readSubBoxes(b.File(), b.Start(), b.Size())
	for subBox := range boxes {
		switch subBox.Name() {
		case "meta":
			b.meta = &MetaBox{ Box:subBox }
			err = b.meta.parse()
		default:
			fmt.Printf("Unhandled Udta Sub-Box: %v \n", subBox.Name())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

type MetaBox struct {
	*Box
	version uint8
	flags [3]byte
	hdlr *HdlrBox
}

func (b *MetaBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	boxes := readSubBoxes(b.File(), b.Start()+4, b.Size()-4)
	for subBox := range boxes {
		switch subBox.Name() {
		case "hdlr":
			b.hdlr = &HdlrBox{ Box:subBox }
			err = b.hdlr.parse()
		default:
			fmt.Printf("Unhandled Meta Sub-Box: %v \n", subBox.Name())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// An 8.8 Fixed Point Decimal notation
type Fixed16 uint16

func (f Fixed16) String() string {
	return fmt.Sprintf("%v", uint16(f) >> 8)
}

func MakeFixed16(bytes []byte) (Fixed16, os.Error) {
	if len(bytes) != 2 {
		return Fixed16(0), os.NewError("Invalid number of bytes for Fixed16. Need 2, got " + string(len(bytes)))
	}
	return Fixed16(binary.BigEndian.Uint16(bytes)), nil
}

// A 16.16 Fixed Point Decimal notation
type Fixed32 uint32

func (f Fixed32) String() string {
	return fmt.Sprintf("%v", uint32(f) >> 16)
}

func MakeFixed32(bytes []byte) (Fixed32, os.Error) {
	if len(bytes) != 4 {
		return Fixed32(0), os.NewError("Invalid number of bytes for Fixed32. Need 4, got " + string(len(bytes)))
	}
	return Fixed32(binary.BigEndian.Uint32(bytes)), nil
}

type Chunk struct {
	sample_description_index, start_sample, sample_count, offset uint32
}

type Sample struct {
	size, offset, start_time, duration, cto uint32
}
