package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"godvr/internal/dvrip"
	"sync"
	"time"

	"github.com/deepch/vdk/codec"
	"github.com/deepch/vdk/codec/h264parser"

	"github.com/deepch/vdk/av"
)

var MuxerStreamWebRTC = NewMuxerWebRTC()

type MuxerWebRTCST struct {
	mutex   sync.RWMutex
	streams map[string]webRTCStreamsST
}
type webRTCStreamsST struct {
	codecData []av.CodecData
	fps       int
	sps       []byte
	pps       []byte
	clients   map[string]webRTCClientST
}
type webRTCClientST struct {
	c chan *av.Packet
}

//NewMuxerWebRTC func
func NewMuxerWebRTC() *MuxerWebRTCST {
	return &MuxerWebRTCST{streams: map[string]webRTCStreamsST{"camera1": webRTCStreamsST{clients: make(map[string]webRTCClientST)}}}
}

//List func
func (obj *MuxerWebRTCST) List() []string {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	var res []string
	for i := range obj.streams {
		res = append(res, i)
	}
	return res
}

//PortHTTP func
func (obj *MuxerWebRTCST) PortHTTP() string {
	return ":8083"
}

//CodecGet
func (obj *MuxerWebRTCST) CodecGet(name string) []av.CodecData {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	return obj.streams[name].codecData
}

//
func (obj *MuxerWebRTCST) ClientAdd(streamUUID string) (string, chan *av.Packet) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	clientUUID := pseudoUUID()
	ch := make(chan *av.Packet, 100)
	obj.streams[streamUUID].clients[clientUUID] = webRTCClientST{c: ch}
	return clientUUID, ch
}
func (obj *MuxerWebRTCST) ClientDelete(streamUUID string, clientUUID string) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	delete(obj.streams[streamUUID].clients, clientUUID)
}
func (obj *MuxerWebRTCST) Exists(name string) bool {
	return true
}
func (obj *MuxerWebRTCST) GetICECredential() string {
	return ""
}
func (obj *MuxerWebRTCST) GetICEUsername() string {
	return ""
}
func (obj *MuxerWebRTCST) GetICEServers() []string {
	return []string{}
}
func (obj *MuxerWebRTCST) GetWebRTCPortMax() uint16 {
	return 0
}
func (obj *MuxerWebRTCST) GetWebRTCPortMin() uint16 {
	return 0
}
func (obj *MuxerWebRTCST) WriteFrame(name string, frame *dvrip.Frame) {
	obj.mutex.Lock()
	obj.mutex.Unlock()
	tmp, _ := obj.streams[name]
	if frame.Meta.Type == "G711A" {
		//log.Println("Audio")
		for _, client := range obj.streams[name].clients {
			if len(client.c) < 100 {
				if len(tmp.codecData) == 1 {
					tmp.CodecUpdatePCMAlaw()
				}
				if len(tmp.codecData) == 2 {
					client.c <- &av.Packet{Duration: time.Duration(len(frame.Data)) * time.Second / time.Duration(8000), Idx: 1, Data: frame.Data}
				}
			}
		}
	} else {
		packetsRaw, _ := h264parser.SplitNALUs(frame.Data)
		for _, i2 := range packetsRaw {
			naluType := i2[0] & 0x1f
			switch {
			case naluType >= 1 && naluType <= 5:
				if naluType == 5 {
					tmp.fps = frame.Meta.FPS
				}
				if tmp.fps != 0 {
					for _, client := range obj.streams[name].clients {
						if len(client.c) < 100 {
							client.c <- &av.Packet{Duration: time.Duration(1000/tmp.fps) * time.Millisecond, Idx: 0, IsKeyFrame: naluType == 5, Data: append(binSize(len(i2)), i2...)}
						}
					}
				}
			case naluType == 7:
				tmp.CodecUpdateSPS(i2)
			case naluType == 8:
				tmp.CodecUpdatePPS(i2)
			}
		}
	}
	obj.streams[name] = tmp
}

//CodecUpdateSPS func
func (obj *webRTCStreamsST) CodecUpdateSPS(val []byte) {
	if bytes.Compare(val, obj.sps) == 0 {
		return
	}
	obj.sps = val
	if len(obj.pps) == 0 {
		return
	}
	codecData, err := h264parser.NewCodecDataFromSPSAndPPS(val, obj.pps)
	if err != nil {
		return
	}
	if len(obj.codecData) > 0 {
		for i, i2 := range obj.codecData {
			if i2.Type().IsVideo() {
				obj.codecData[i] = codecData
			}
		}
	} else {
		obj.codecData = append(obj.codecData, codecData)
	}
}

func (obj *webRTCStreamsST) CodecUpdatePPS(val []byte) {
	if bytes.Compare(val, obj.pps) == 0 {
		return
	}
	obj.pps = val
	if len(obj.sps) == 0 {
		return
	}
	codecData, err := h264parser.NewCodecDataFromSPSAndPPS(obj.sps, val)
	if err != nil {
		return
	}
	if len(obj.codecData) > 0 {
		for i, i2 := range obj.codecData {
			if i2.Type().IsVideo() {
				obj.codecData[i] = codecData
			}
		}
	} else {
		obj.codecData = append(obj.codecData, codecData)
	}
}
func (obj *webRTCStreamsST) CodecUpdatePCMAlaw() {
	CodecData := codec.NewPCMAlawCodecData()
	obj.codecData = append(obj.codecData, CodecData)
}

//binSize func
func binSize(val int) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(val))
	return buf
}

//pseudoUUID
func pseudoUUID() (uuid string) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}
	uuid = fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	return
}
