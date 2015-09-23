package main
import (
	"flag"
	"github.com/berndfo/goflv"
	"log"
	"io"
	"fmt"
	_ "expvar"
)

var flvFileName *string = flag.String("file", "", "FLV file")


func init() {
	flag.Parse()
}

// dump content of a FLV file. log header info for every tag, and an overall summary
func main() {
	
	flvFile, err := flv.OpenFile(*flvFileName)
	if err != nil {
		log.Println("Open FLV dump file error:", err)
		return
	}
	defer flvFile.Close()

	tagTypeCounters := make(map[byte]int)
	tagTypeLatestTimestamp := make(map[byte]int64)
	tagTypeLatestTimestamp[8] = -1
	tagTypeLatestTimestamp[9] = -1
	
	for {
		message := ""
		flvFile.ReadTag()

		header, data, err := flvFile.ReadTag()
		if err == io.EOF {
			log.Println("EOF")
			break
		}
		if err != nil {
			log.Println("flvFile.ReadTag() error:", err)
			break
		}
		
		addTagInfo := ""
		if header.TagType == 8 {
			format, sampleRate, size, stereo := flv.AudioMetaData(data)
			addTagInfo = fmt.Sprintf("format = %d, sampling rate = %d, size = %d, stereo = %t", format, sampleRate, size, stereo)
		} else if header.TagType == 9 {
			frameType, codec := flv.VideoMetaData(data)
			addTagInfo = fmt.Sprintf("frame type = %d, codec = %d", frameType, codec)
		}
		
		tagTypeCounters[header.TagType] = tagTypeCounters[header.TagType] + 1
		
		ts64 := int64(header.Timestamp)
		if ts64 <= tagTypeLatestTimestamp[header.TagType] {
			message = fmt.Sprintf("non-monotonic timestamp %d -> %d", tagTypeLatestTimestamp[header.TagType], header.Timestamp)
		}
		tagTypeLatestTimestamp[header.TagType] = ts64 
		
		log.Printf("tag: %+v, data length: %d, %s, %s", header, len(data), message, addTagInfo)
	}
	
	log.Printf("")
	log.Printf("tag stats: %v", tagTypeCounters)
}
