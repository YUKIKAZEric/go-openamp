package openamp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
)

var (
	active = true
)

type rp_msg struct {
	preamble   uint32
	app        uint16
	reserve    uint16
	msgid      uint32
	payloadlen uint32
	payload    []byte
}

const (
	HEADLEN = 16
)

func Open(rpmsgDev string) *os.File {
	var err error
	if rpmsgDev == "" {
		log.Println("No rpmsg device!")
		return nil
	}
	rfFd, err := os.OpenFile(rpmsgDev, os.O_RDWR, os.ModeExclusive)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("rf port '%s' opened \n", rpmsgDev)

	active = true

	return rfFd
}

func Close(rfFd *os.File) {
	active = false
	if rfFd != nil {
		rfFd.Close()
	}
}

func locatePreamble(b []byte) int {
	p := -1
	for i := 0; i < len(b)-4; i = i + 1 {
		if b[i+0] == 0xaf && b[i+1] == 0xbe && b[i+2] == 0xaf && b[i+3] == 0xbe {
			p = i
		}
	}

	return p
}

func RecvMsg(rfFd *os.File, recvFunc func(chl uint16, payload []byte)) {
	var msg *rp_msg
	var cutted []byte
	var minLen int

	readbuf := make([]byte, 2048, 2048)
	for active {
		minLen = 1
		if msg == nil {
			minLen = HEADLEN
		}
		r, err := io.ReadAtLeast(rfFd, readbuf, minLen)
		if err != nil {
			fmt.Println(err)
			continue
		}
		b := append(cutted, readbuf[0:r]...)
		cutted = []byte{}

		if msg == nil {
			p := locatePreamble(b)
			if p < 0 || len(b[p:]) < HEADLEN {
				cutted = b
				continue
			}

			msg = &rp_msg{}

			r := bytes.NewReader(b[p : p+HEADLEN])
			binary.Read(r, binary.LittleEndian, &msg.preamble)
			binary.Read(r, binary.LittleEndian, &msg.app)
			binary.Read(r, binary.LittleEndian, &msg.reserve)
			binary.Read(r, binary.LittleEndian, &msg.msgid)
			binary.Read(r, binary.LittleEndian, &msg.payloadlen)
			c := b[p+HEADLEN:]
			if len(c) <= int(msg.payloadlen) {
				msg.payload = c
			} else {
				msg.payload = c[0:msg.payloadlen]
				cutted = c[msg.payloadlen:]
				//completed
			}

		} else {
			left := int(msg.payloadlen) - len(msg.payload)
			if len(b) > left {
				msg.payload = append(msg.payload, b[0:left]...)
				cutted = b[left:]
				//completed
			} else {
				msg.payload = append(msg.payload, b[0:]...)
			}

		}

		if len(msg.payload) >= int(msg.payloadlen) {
			recvFunc(msg.app, msg.payload)
			msg = nil
		}
	}
}

func SendMsg(rfFd *os.File, chl uint16, payload []byte) {
	msg := rp_msg{
		preamble:   0xbeafbeaf,
		app:        chl,
		reserve:    0xffff,
		payloadlen: uint32(len(payload)),
		payload:    payload,
	}

	buf := new(bytes.Buffer)

	binary.Write(buf, binary.LittleEndian, msg.preamble)
	binary.Write(buf, binary.LittleEndian, msg.app)
	binary.Write(buf, binary.LittleEndian, msg.reserve)
	binary.Write(buf, binary.LittleEndian, msg.msgid)
	binary.Write(buf, binary.LittleEndian, msg.payloadlen)
	binary.Write(buf, binary.LittleEndian, msg.payload)

	sendBytes := buf.Bytes()

	fragSize := 480
	total := int(len(sendBytes))
	remain := total
	offset := 0
	for remain > 0 {
		var toSend int
		if remain < fragSize {
			toSend = remain
		} else {
			toSend = fragSize
		}

		_, err := rfFd.Write(sendBytes[offset : offset+toSend])
		if err != nil {
			log.Println(err)
			return
		}

		remain = remain - toSend
		offset = offset + toSend
	}

	log.Printf("rf: %d bytes forwarded\n", len(sendBytes))
}
