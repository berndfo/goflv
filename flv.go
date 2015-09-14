package flv

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"log"
)

type File struct {
	file              *os.File
	name              string
	readOnly          bool
	size              int64
	headerBuf         []byte
	duration          float64
	lastTimestampSet  bool
	lastTimestamp     uint32
	firstTimestampSet bool
	firstTimestamp    uint32
}

type TagHeader struct {
	TagType   byte
	Encrypted bool
	DataSize  uint32
	Timestamp uint32
}

func CreateFile(name string) (flvFile *File, err error) {
	var file *os.File
	// Create file
	if file, err = os.Create(name); err != nil {
		return
	}
	// Write flv header
	if _, err = file.Write(HEADER_BYTES); err != nil {
		file.Close()
		return
	}

	// Sync to disk
	if err = file.Sync(); err != nil {
		file.Close()
		return
	}

	flvFile = &File{
		file:      file,
		name:      name,
		readOnly:  false,
		headerBuf: make([]byte, 11),
		duration:  0.0,
	}

	return
}

func OpenFile(name string) (flvFile *File, err error) {
	var file *os.File
	// Open file
	file, err = os.Open(name)
	if err != nil {
		return
	}

	var size int64
	if size, err = file.Seek(0, 2); err != nil {
		file.Close()
		return
	}
	if _, err = file.Seek(0, 0); err != nil {
		file.Close()
		return
	}

	flvFile = &File{
		file:      file,
		name:      name,
		readOnly:  true,
		size:      size,
		headerBuf: make([]byte, 11),
	}

	// Read flv header
	remain := HEADER_LEN
	flvHeader := make([]byte, remain)

	if _, err = io.ReadFull(file, flvHeader); err != nil {
		file.Close()
		return
	}
	if flvHeader[0] != 'F' ||
		flvHeader[1] != 'L' ||
		flvHeader[2] != 'V' {
		file.Close()
		return nil, errors.New("File format error")
	}

	return
}

// extracts meta data for an audio tag
// 
// Format:
// 0 = Linear PCM, platform endian
// 1 = ADPCM
// 2 = MP3
// 3 = Linear PCM, little endian 4 = Nellymoser 16 kHz mono 5 = Nellymoser 8 kHz mono 6 = Nellymoser
// 7 = G.711 A-law logarithmic PCM
// 8 = G.711 mu-law logarithmic PCM
// 9 = reserved
// 10 = AAC
// 11 = Speex
// 14 = MP3 8 kHz
// 15 = Device-specific sound
//
// Rate:
// 0 = 5.5 kHz
// 1 = 11 kHz
// 2 = 22 kHz
// 3 = 44 kHz
//
// Sample size: 
// 0 = 8-bit samples
// 1 = 16-bit samples
func AudioMetaData(data []byte) (format int, sampleRate int, size int, stereo bool) {
	if len(data) < 1 {
		return -1, -1, -1, false
	}
	byt := data[0]
	format = int(byt >> 4 & 0x0f)
	sampleRate = int(byt >> 2 & 0x03)
	size = int(byt >> 1 & 0x01)
	stereo = (byt & 0x01) > 0
	
	return
}

// extracts meta data for an video tag
// 
// Frame type:
// 1 = key frame (for AVC, a seekable frame)
// 2 = inter frame (for AVC, a non-seekable frame)
// 3 = disposable inter frame (H.263 only)
// 4 = generated key frame (reserved for server use only) 
// 5 = video info/command frame
// 
// Codec:
// 2 = Sorenson H.263
// 3 = Screen video
// 4 = On2 VP6
// 5 = On2 VP6 with alpha channel 
// 6 = Screen video version 2
// 7 = AVC
func VideoMetaData(data []byte) (frameType int, codec int) {
	if len(data) < 1 {
		return -1, -1
	}
	byt := data[0]
	frameType = int(byt >> 4 & 0x0f)
	codec = int(byt & 0x0f)
	
	return
}

func (flvFile *File) Close() {
	flvFile.file.Close()
}

// Data with audio header
func (flvFile *File) WriteAudioTag(data []byte, timestamp uint32) (err error) {
	//log.Printf("write audio ts = %d", timestamp)
	return flvFile.WriteTag(data, AUDIO_TAG, timestamp)
}

// Data with video header
func (flvFile *File) WriteVideoTag(data []byte, timestamp uint32) (err error) {
	//log.Printf("write video ts = %d", timestamp)
	return flvFile.WriteTag(data, VIDEO_TAG, timestamp)
}

// Write tag
func (flvFile *File) WriteTag(data []byte, tagType byte, timestamp uint32) (err error) {
	if !flvFile.firstTimestampSet {
		flvFile.firstTimestampSet = true
		flvFile.firstTimestamp = timestamp
		log.Printf("FLV: firstTimestamp set to %d", timestamp)
	}
	if timestamp < flvFile.lastTimestamp && flvFile.lastTimestampSet {
		log.Printf("FLV: ts correction needed? tag timestamp %d < lastTimestamp %d", timestamp, flvFile.lastTimestamp)
		// TODO this is questionable. do tags always come in order, especially video vs. audio?
		// timestamp = flvFile.lastTimestamp
	} else {
		flvFile.lastTimestamp = timestamp
		flvFile.lastTimestampSet = true
	}
	timestamp -= flvFile.firstTimestamp // normalize to "start at zero"
	duration := float64(timestamp) / 1000.0 // from msec to seconds
	if flvFile.duration < duration {
		flvFile.duration = duration
	}
	binary.BigEndian.PutUint32(flvFile.headerBuf[3:7], timestamp)
	flvFile.headerBuf[7] = flvFile.headerBuf[3]
	binary.BigEndian.PutUint32(flvFile.headerBuf[:4], uint32(len(data)))
	flvFile.headerBuf[0] = tagType
	// Write data
	if _, err = flvFile.file.Write(flvFile.headerBuf); err != nil {
		return
	}

	//tmpBuf := make([]byte, 4)
	//// Write tag header
	//if _, err = flvFile.file.Write([]byte{tagType}); err != nil {
	//	return
	//}

	//// Write tag size
	//binary.BigEndian.PutUint32(tmpBuf, uint32(len(data)))
	//if _, err = flvFile.file.Write(tmpBuf[1:]); err != nil {
	//	return
	//}

	//// Write timestamp
	//binary.BigEndian.PutUint32(tmpBuf, timestamp)
	//if _, err = flvFile.file.Write(tmpBuf[1:]); err != nil {
	//	return
	//}
	//if _, err = flvFile.file.Write(tmpBuf[:1]); err != nil {
	//	return
	//}

	//// Write stream ID
	//if _, err = flvFile.file.Write([]byte{0, 0, 0}); err != nil {
	//	return
	//}

	// Write data
	if _, err = flvFile.file.Write(data); err != nil {
		return
	}

	// Write previous tag size
	if err = binary.Write(flvFile.file, binary.BigEndian, uint32(len(data)+11)); err != nil {
		return
	}

	// Sync to disk
	//if err = flvFile.file.Sync(); err != nil {
	//	return
	//}
	return
}

func (flvFile *File) SetDuration(duration float64) {
	flvFile.duration = duration
}

func (flvFile *File) Sync() (err error) {
	// Update duration on MetaData
	if _, err = flvFile.file.Seek(DURATION_OFFSET, 0); err != nil {
		return
	}
	if err = binary.Write(flvFile.file, binary.BigEndian, flvFile.duration); err != nil {
		return
	}
	if _, err = flvFile.file.Seek(0, 2); err != nil {
		return
	}

	err = flvFile.file.Sync()
	return
}
func (flvFile *File) Size() (size int64) {
	size = flvFile.size
	return
}

func (flvFile *File) ReadTag() (header *TagHeader, data []byte, err error) {
	tmpBuf := make([]byte, 4)
	header = &TagHeader{}
	// Read tag header
	if _, err = io.ReadFull(flvFile.file, tmpBuf[3:]); err != nil {
		return
	}
	header.Encrypted = tmpBuf[3] & 0x20 > 0
	header.TagType = tmpBuf[3] & 0x1F 

	// Read tag size
	if _, err = io.ReadFull(flvFile.file, tmpBuf[1:]); err != nil {
		return
	}
	header.DataSize = uint32(tmpBuf[1])<<16 | uint32(tmpBuf[2])<<8 | uint32(tmpBuf[3])

	// Read timestamp
	if _, err = io.ReadFull(flvFile.file, tmpBuf); err != nil {
		return
	}
	header.Timestamp = uint32(tmpBuf[3])<<24 + uint32(tmpBuf[0])<<16 + uint32(tmpBuf[1])<<8 + uint32(tmpBuf[2])

	// Read stream ID
	if _, err = io.ReadFull(flvFile.file, tmpBuf[1:]); err != nil {
		return
	}

	// Read data
	data = make([]byte, header.DataSize)
	if _, err = io.ReadFull(flvFile.file, data); err != nil {
		return
	}

	// Read previous tag size
	if _, err = io.ReadFull(flvFile.file, tmpBuf); err != nil {
		return
	}

	return
}

func (flvFile *File) IsFinished() bool {
	pos, err := flvFile.file.Seek(0, 1)
	return (err != nil) || (pos >= flvFile.size)
}
func (flvFile *File) LoopBack() {
	flvFile.file.Seek(HEADER_LEN, 0)
}
func (flvFile *File) FilePath() string {
	return flvFile.name
}
