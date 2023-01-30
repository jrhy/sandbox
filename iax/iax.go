package iax

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"sync"
	"time"
	"unicode"
)

type CallAction int

const (
	_ = CallAction(iota)
	callAccept
	CallReject
)

type Callee interface {
	AcceptOrReject() CallAction
}

// Frame types
const (
	FrameTypeDTMF         = 0x01
	FrameTypeVoice        = 0x02
	FrameTypeControl      = 0x04
	FrameTypeIAX          = 0x06
	FrameTypeComfortNoise = 0x0a
)

// IAX frame subclasses
const (
	IAXSubclassNEW       = 0x01
	IAXSubclassPING      = 0x02
	IAXSubclassPONG      = 0x03
	IAXSubclassACK       = 0x04
	IAXSubclassHANGUP    = 0x05
	IAXSubclassREJECT    = 0x06
	IAXSubclassACCEPT    = 0x07
	IAXSubclassAUTHREQ   = 0x08
	IAXSubclassAUTHREP   = 0x09
	IAXSubclassINVAL     = 0x0a
	IAXSubclassLAGRQ     = 0x0b
	IAXSubclassLAGRP     = 0x0c
	IAXSubclassREGREQ    = 0x0d
	IAXSubclassREGAUTH   = 0x0e
	IAXSubclassREGACK    = 0x0f
	IAXSubclassREGREJ    = 0x10
	IAXSubclassREGREL    = 0x11
	IAXSubclassVNAK      = 0x12
	IAXSubclassDPREQ     = 0x13
	IAXSubclassDPREP     = 0x14
	IAXSubclassDIAL      = 0x15
	IAXSubclassTXREQ     = 0x16 // Transfer request
	IAXSubclassTXCNT     = 0x17 // Transfer connect
	IAXSubclassTXACC     = 0x18 // Transfer accept
	IAXSubclassTXREADY   = 0x19 // Transfer ready
	IAXSubclassTXREL     = 0x1a // Transfer release
	IAXSubclassTXREJ     = 0x1b // Transfer reject
	IAXSubclassQUELCH    = 0x1c // Halt audio/video [media] transmission
	IAXSubclassUNQUELCH  = 0x1d // Resume audio/video [media] transmission
	IAXSubclassPOKE      = 0x1e // Poke request
	IAXSubclassMWI       = 0x20 // Message waiting indication
	IAXSubclassUNSUPPORT = 0x21 // Unsupported message

)

// InformationElements
const (
	IECALLED_NUMBER  = 0x01 // Number/extension being called
	IECALLING_NUMBER = 0x02 // Calling number
	IECALLING_ANI    = 0x03 // Calling number ANI for billing
	IECALLING_NAME   = 0x04 // Name of caller
	IECALLED_CONTEXT = 0x05 // Context for number
	IEUSERNAME       = 0x06 // Username (peer or user) for authentication
	IEPASSWORD       = 0x07 // Password for authentication
	IECAPABILITY     = 0x08 // Actual CODEC capability
	IEFORMAT         = 0x09 // Desired CODEC format
	IELANGUAGE       = 0x0a // Desired language
	IEVERSION        = 0x0b // Protocol version
	IEADSICPE        = 0x0c // CPE ADSI capability
	IEDNID           = 0x0d // Originally dialed DNID
	IEAUTHMETHODS    = 0x0e // Authentication method(s)
	IECHALLENGE      = 0x0f // Challenge data for MD5/RSA
	IEMD5_RESULT     = 0x10 // MD5 challenge result
	IERSA_RESULT     = 0x11 // RSA challenge result
	IEAPPARENT_ADDR  = 0x12 // Apparent address of peer
	IEREFRESH        = 0x13 // When to refresh registration
	IEDPSTATUS       = 0x14 // Dialplan status
	IECALLNO         = 0x15 // Call number of peer
	IECAUSE          = 0x16 // Cause
	IEIAX_UNKNOWN    = 0x17 // Unknown IAX command
	IEMSGCOUNT       = 0x18 // How many messages waiting
	IEAUTOANSWER     = 0x19 // Request auto-answering
	IEMUSICONHOLD    = 0x1a // Request musiconhold with QUELCH
	IETRANSFERID     = 0x1b // Transfer Request Identifier
	IERDNIS          = 0x1c // Referring DNIS
	IEDATETIME       = 0x1f // Date/Time
	IECALLINGPRES    = 0x26 // Calling presentation
	IECALLINGTON     = 0x27 // Calling type of number
	IECALLINGTNS     = 0x28 // Calling transit network select
	IESAMPLINGRATE   = 0x29 // Supported sampling rates
	IECAUSECODE      = 0x2a // Hangup cause
	IEENCRYPTION     = 0x2b // Encryption format
	IEENCKEY         = 0x2c // Reserved for future Use
	IECODEC_PREFS    = 0x2d // CODEC Negotiation
	IERR_JITTER      = 0x2e // Received jitter, as in RFC 3550
	IERR_LOSS        = 0x2f // Received loss, as in RFC 3550
	IERR_PKTS        = 0x30 // Received frames
	IERR_DELAY       = 0x31 // Max playout delay for received frames in ms
	IERR_DROPPED     = 0x32 // Dropped frames (presumably by jitter buffer)
	IERR_OOO         = 0x33 // Frames received Out of Order
	IEOSPTOKEN       = 0x34 // OSP Token Block
)

const ackLimit = 5

type Server struct {
	Address string
}

type User struct {
	Username string
	Password string
}

type Source uint16

type Peer struct {
	Server             Server
	Debug              func(string, ...interface{})
	conn               net.Conn
	user               User
	registrationSource Source
	callMutex          sync.Mutex
	call               map[Source]*Call
	callByDest         map[Source]*Call
	Callee             Callee
}

func NewPeer(addr string, user User) *Peer {
	return &Peer{
		Server:     Server{Address: addr},
		user:       user,
		call:       map[Source]*Call{},
		callByDest: map[Source]*Call{},
	}
}

type InformationElement struct {
	IE         uint8
	DataLength uint8
}
type WireFullFrame struct {
	FSource   uint16
	RDest     uint16
	Timestamp uint32
	OSeqno    uint8
	ISeqno    uint8
	FrameType uint8
	CSubclass uint8
}
type FullFrameHeader struct {
	F         bool
	Source    uint16
	R         bool
	Dest      uint16
	Timestamp uint32
	OSeqno    uint8
	ISeqno    uint8
	FrameType uint8
	C         bool
	Subclass  uint8
}
type FullFrame struct {
	FullFrameHeader
	IAX   IAXFramePart
	DTMF  string
	Voice []byte
}

type ControlFrame struct {
	FullFrameHeader
}
type IAXFrame struct {
	FullFrameHeader
	IAX IAXFramePart
}

type IAXFramePart struct {
	CallingName     string
	CallingNumber   string
	CalledNumber    string
	CodecPrefs      string
	Username        string
	AuthMethods     uint16
	Challenge       string
	Refresh         uint16
	ApparentAddr    string
	DateTime        string
	Format          uint32
	SamplingRate    uint16
	Capability      uint32
	Cause           string
	HangupCauseCode uint8
	//CallingPres   uint8
	//CallingTon    uint8
	Version   uint16
	MD5Result string
}

type Call struct {
	peer               *Peer
	replies            chan FullFrame
	source             Source
	start              time.Time
	OSeqno             uint8
	mutex              sync.Mutex
	ISeqno             uint8
	dest               uint16
	refreshBefore      time.Time
	lastACK            FullFrame
	ackTries           int
	lastAudioTimestamp uint32
}

func (c *Call) sequenceFF(h FullFrameHeader) FullFrameHeader {
	if c.start.IsZero() {
		c.start = time.Now()
	}
	h.Timestamp = uint32(time.Now().Sub(c.start).Milliseconds())
	h.Source = uint16(c.source)
	if h.FrameType == FrameTypeVoice {
		// decide if we can send a mini frame
		c.mutex.Lock()
		if true && c.lastAudioTimestamp != uint32(0) && c.lastAudioTimestamp>>16 == h.Timestamp>>16 {
			h.F = false
		} else {
			h.F = true
			c.lastAudioTimestamp = h.Timestamp
		}
		c.mutex.Unlock()
	}
	if !h.F {
		return h
	}
	h.Dest = uint16(c.dest)
	h.OSeqno = c.OSeqno
	if h.FrameType != FrameTypeIAX || h.Subclass != IAXSubclassACK {
		c.OSeqno++
	}
	c.mutex.Lock()
	h.ISeqno = c.ISeqno
	c.mutex.Unlock()
	return h
}

func (p *Peer) sendFrame(f FullFrame) error {
	pb := &bytes.Buffer{}
	var r uint16
	if f.R {
		r = 0x8000
	}

	var err error
	if f.F {
		err = binary.Write(pb, binary.BigEndian,
			WireFullFrame{
				FrameType: f.FrameType,
				FSource:   0x8000 | f.Source&0x7fff,
				RDest:     r | f.Dest&0x7fff,
				CSubclass: f.Subclass,
				Timestamp: f.Timestamp,
				OSeqno:    f.OSeqno,
				ISeqno:    f.ISeqno,
			})
	} else {
		if f.FrameType != FrameTypeVoice {
			return errors.New("mini frame should be voice type")
		}
		err = binary.Write(pb, binary.BigEndian,
			struct {
				FSource   uint16
				Timestamp uint16
			}{
				FSource:   f.Source & 0x7fff,
				Timestamp: uint16(f.Timestamp),
			})
	}
	if err != nil {
		return err
	}
	switch f.FrameType {
	case FrameTypeIAX:
		if f.IAX.Username != "" {
			if err = writeIEString(pb, IEUSERNAME, f.IAX.Username); err != nil {
				return err
			}
		}
		if f.IAX.MD5Result != "" {
			if err = writeIEString(pb, IEMD5_RESULT, f.IAX.MD5Result); err != nil {
				return err
			}
		}
		if f.IAX.Refresh != 0 {
			if err = writeIEBinary(pb, IEREFRESH, 2, &f.IAX.Refresh); err != nil {
				return err
			}
		}
		if f.IAX.Format != 0 {
			if err = writeIEBinary(pb, IEFORMAT, 4, &f.IAX.Format); err != nil {
				return err
			}
		}
		if f.IAX.Capability != 0 {
			if err = writeIEBinary(pb, IECAPABILITY, 4, &f.IAX.Capability); err != nil {
				return err
			}
		}
	case FrameTypeControl:
	case FrameTypeDTMF:
	case FrameTypeVoice:
		pb.Write(f.Voice)
	default:
		return fmt.Errorf("unhandled frameType 0x%x", f.FrameType)
	}

	if f.F {
		p.debug(">>>>> %v\n", f)
	}
	_, err = p.conn.Write(pb.Bytes())
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

func writeIEString(pb io.Writer, ieType uint8, s string) error {
	if err := binary.Write(pb, binary.BigEndian,
		InformationElement{
			IE:         ieType,
			DataLength: uint8(len(s)),
		}); err != nil {
		return err
	}
	if _, err := pb.Write([]byte(s)); err != nil {
		return err
	}
	return nil
}

func writeIEBinary(pb io.Writer, ieType uint8, expectedLen int, i interface{}) error {
	if err := binary.Write(pb, binary.BigEndian,
		InformationElement{
			IE:         ieType,
			DataLength: uint8(expectedLen),
		}); err != nil {
		return err
	}
	tmp := &bytes.Buffer{}
	if err := binary.Write(tmp, binary.BigEndian, i); err != nil {
		return err
	}
	if tmp.Len() != expectedLen {
		return fmt.Errorf("should have written %d, actually wrote %d", expectedLen, tmp.Len())
	}
	if _, err := pb.Write(tmp.Bytes()); err != nil {
		return err
	}
	return nil
}

func NewIAXFrame(subclass uint8, iax IAXFramePart) FullFrame {
	return FullFrame{
		FullFrameHeader: FullFrameHeader{
			FrameType: FrameTypeIAX,
			F:         true,
			Subclass:  subclass,
		},
		IAX: iax,
	}
}

func NewControlFrame(subclass uint8) FullFrame {
	return FullFrame{
		FullFrameHeader: FullFrameHeader{
			FrameType: FrameTypeControl,
			F:         true,
			Subclass:  subclass,
		},
	}
}

func (p *Peer) RegisterAndServe() error {
	var err error

	p.conn, err = net.Dial("udp", p.Server.Address)
	if err != nil {
		return fmt.Errorf("dial %s: %w", p.Server.Address, err)
	}
	defer p.conn.Close()

	readErr := make(chan error)
	go p.readPackets(readErr)

	for {
		c := Call{
			source:  newSource(),
			peer:    p,
			replies: make(chan FullFrame, 1),
		}
		p.callMutex.Lock()
		p.call[c.source] = &c
		p.registrationSource = c.source
		p.callMutex.Unlock()
		ff, err := c.transmitUntilReply(NewIAXFrame(
			IAXSubclassREGREQ,
			IAXFramePart{
				Username: p.user.Username,
				Refresh:  300,
			}))
		if err != nil {
			return err
		}

		if ff.C {
			return errors.New("not handling 'C'")
		}
		if ff.Subclass != IAXSubclassREGAUTH {
			return fmt.Errorf("expected REGAUTH, got %v", ff)
		}
		if ff.IAX.Challenge == "" {
			return errors.New("got REGAUTH with no challenge")
		}

		if string(ff.IAX.Username) != c.peer.user.Username {
			return fmt.Errorf("server replied for another user '%s'", ff.IAX.Username)
		}
		if ff.IAX.AuthMethods&0x02 == 0 {
			return fmt.Errorf("server auth methods 0x%x do not include MD5", ff.IAX.AuthMethods)
		}

		h := md5.New()
		_, err = io.WriteString(h, ff.IAX.Challenge)
		if err != nil {
			return fmt.Errorf("challenge writeString: %w", err)
		}
		_, err = io.WriteString(h, c.peer.user.Password)
		if err != nil {
			return fmt.Errorf("challenge writeString: %w", err)
		}
		md5Result := hex.EncodeToString(h.Sum(nil))

		ff, err = c.transmitUntilReply(NewIAXFrame(
			IAXSubclassREGREQ,
			IAXFramePart{
				Username:  p.user.Username,
				MD5Result: md5Result,
				Refresh:   300,
			}))
		if err != nil {
			return err
		}

		if !ff.F || ff.FrameType != FrameTypeIAX || ff.Subclass != IAXSubclassREGACK {
			return fmt.Errorf("received unexpected response; wanted REGACK: %v", ff)
		}

		if err = c.ACKIAX(ff); err != nil {
			return err
		}

		select {
		case err = <-readErr:
			return err
		case <-time.After(c.GetRefresh()):
			p.debug("time to register\n")
			continue
		}
	}
}

func (c *Call) ACKIAX(ff FullFrame) error {
	c.mutex.Lock()
	ack := FullFrame{
		FullFrameHeader: FullFrameHeader{
			FrameType: FrameTypeIAX,
			Timestamp: ff.Timestamp,
			F:         true,
			R:         ff.R,
			Subclass:  IAXSubclassACK,
			Source:    uint16(c.source),
			Dest:      uint16(c.dest),
			ISeqno:    c.ISeqno,
			OSeqno:    c.OSeqno,
		},
	}
	//c.OSeqno++
	c.lastACK = ack
	c.ackTries = 0
	c.mutex.Unlock()
	return c.peer.sendFrame(ack)
}

func decodeFullFrame(b []byte) (FullFrame, error) {
	var ff FullFrame

	r := bytes.NewReader(b)
	var fsource uint16
	err := binary.Read(r, binary.BigEndian, &fsource)
	if err != nil {
		return FullFrame{}, fmt.Errorf("read fsource: %w", err)
	}

	if fsource&0x8000 != 0 {
		ff.F = true
		ff.Source = fsource & 0x7fff
		rest := struct {
			RDest     uint16
			Timestamp uint32
			OSeqno    uint8
			ISeqno    uint8
			FrameType uint8
			CSubclass uint8
		}{}
		err := binary.Read(r, binary.BigEndian, &rest)
		if err != nil {
			return FullFrame{}, fmt.Errorf("read fsource: %w", err)
		}
		ff.R = rest.RDest&0x8000 != 0
		ff.Dest = rest.RDest & 0x7fff
		ff.Timestamp = rest.Timestamp
		ff.OSeqno = rest.OSeqno
		ff.ISeqno = rest.ISeqno
		ff.FrameType = rest.FrameType
		ff.C = rest.CSubclass&0x80 != 0
		ff.Subclass = rest.CSubclass & 0x7f
	} else {
		ff.Source = fsource & 0x7fff
		var smallTimestamp uint16
		err := binary.Read(r, binary.BigEndian, &smallTimestamp)
		if err != nil {
			return FullFrame{}, fmt.Errorf("read fsource: %w", err)
		}
		ff.Timestamp = uint32(smallTimestamp)
		ff.FrameType = FrameTypeVoice
	}

	switch ff.FrameType {
	case FrameTypeVoice:
		ff.Voice = make([]byte, 1500)
		i, _ := r.Read(ff.Voice)
		ff.Voice = ff.Voice[:i]
	case FrameTypeControl:
		return ff, nil
	case FrameTypeIAX:
	case FrameTypeDTMF:
		ff.DTMF = string([]byte{ff.Subclass & 0xff})
		return ff, nil
	default:
		return ff, nil //fmt.Errorf("unhandled frame type 0x%x", ff.FrameType)
	}

	for {
		var ie InformationElement
		err = binary.Read(r, binary.BigEndian, &ie)
		if err == io.EOF {
			return ff, nil
		}
		if err != nil {
			return ff, fmt.Errorf("read ie: %w", err)
		}
		//fmt.Printf("decoding 0x%x\n", ie.IE)
		switch ie.IE {
		case IEUSERNAME:
			ff.IAX.Username = readString(r, ie.DataLength)
		case IEAUTHMETHODS:
			readBigEndian(r, ie.DataLength, &ff.IAX.AuthMethods)
		case IECHALLENGE:
			ff.IAX.Challenge = readString(r, ie.DataLength)
		case IEREFRESH:
			readBigEndian(r, ie.DataLength, &ff.IAX.Refresh)
		case IEAPPARENT_ADDR:
			dbuf := make([]byte, ie.DataLength)
			_, err = r.Read(dbuf)
			if err != nil {
				return ff, fmt.Errorf("read apparent addr: %w", err)
			}
			if dbuf[0] != 0x02 || dbuf[1] != 0x00 {
				return ff, fmt.Errorf("apparent addr has unknown AF 0x%x", (uint16(dbuf[0])<<8)+uint16(dbuf[1]))
			}
			ff.IAX.ApparentAddr = net.IP(dbuf[4:8]).String()
		case IECODEC_PREFS:
			ff.IAX.CodecPrefs = readString(r, ie.DataLength)
		case IECALLED_NUMBER:
			ff.IAX.CalledNumber = readString(r, ie.DataLength)
		case IECALLING_NUMBER:
			ff.IAX.CallingNumber = readString(r, ie.DataLength)
		case IECALLING_NAME:
			ff.IAX.CallingName = readString(r, ie.DataLength)
		case IEFORMAT:
			readBigEndian(r, ie.DataLength, &ff.IAX.Format)
		case IESAMPLINGRATE:
			readBigEndian(r, ie.DataLength, &ff.IAX.SamplingRate)
		case IECAPABILITY:
			readBigEndian(r, ie.DataLength, &ff.IAX.Capability)
		case IECAUSE:
			ff.IAX.Cause = readString(r, ie.DataLength)
		case IECAUSECODE:
			readBigEndian(r, ie.DataLength, &ff.IAX.HangupCauseCode)
		case IEVERSION:
			readBigEndian(r, ie.DataLength, &ff.IAX.Version)
		case IECALLINGTNS, IECALLINGPRES, IECALLINGTON,
			IELANGUAGE, IEDATETIME,
			0, 54, 55, 56, IEADSICPE, 101:
			io.CopyN(ioutil.Discard, r, int64(ie.DataLength))
		default:
			fmt.Printf("skipping unknown IE %+v\n", ie)
			io.CopyN(ioutil.Discard, r, int64(ie.DataLength))
		}
	}
}

func newSource() Source {
	return Source(randUint16() & 0x7fff)
}

func randUint16() uint16 {
	var b [2]byte
	_, err := rand.Read(b[:])
	if err != nil {
		panic("cannot seed math/rand package with cryptographically secure random number generator")
	}
	return binary.LittleEndian.Uint16(b[:])
}

func readString(r io.Reader, l uint8) string {
	if l == 0 {
		return ""
	}
	dbuf := make([]byte, l)
	_, err := r.Read(dbuf)
	if err != nil {
		panic(err)
	}
	res := strings.TrimFunc(string(dbuf), func(r rune) bool {
		return !unicode.IsGraphic(r)
	})
	//fmt.Printf("decoded bytes to string %s\n", res)
	return res
}

func readBigEndian(r io.Reader, l uint8, a interface{}) {
	dbuf := make([]byte, l)
	_, err := r.Read(dbuf)
	must(err)
	must(binary.Read(bytes.NewReader(dbuf), binary.BigEndian, a))
	//fmt.Printf("decoded bytes to %+v\n", a)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func (p *Peer) readPackets(readErr chan error) {
	var ff FullFrame
	rep := make([]byte, 1500)
	var n uint32 = 0
	for {
		n = (n + 1) % 100000
		i, err := p.conn.Read(rep)
		if err != nil {
			readErr <- fmt.Errorf("read %05d: %w", n, err)
			return
		}
		ff, err = decodeFullFrame(rep[:i])
		if err != nil {
			p.debug("%05d decode full frame: %v\n", n, err)
			continue
		}
		if ff.F {
			p.debug("%05d %v\n", n, ff)
		}
		p.callMutex.Lock()
		var call *Call
		if ff.Dest != uint16(0) {
			call = p.call[Source(ff.Dest)]
		} else {
			call = p.callByDest[Source(ff.Source)]
		}
		if call == nil {
			if ff.FrameType != FrameTypeIAX || ff.Subclass != IAXSubclassNEW {
				p.debug("%05d sending INVAL reply for unknown call\n", n)
				p.callMutex.Unlock()
				p.sendFrame(FullFrame{
					FullFrameHeader: FullFrameHeader{
						F:         true,
						FrameType: FrameTypeIAX,
						Source:    0,
						Dest:      ff.Source,
						Subclass:  IAXSubclassINVAL,
					},
				})
				continue
			}
			c := Call{
				source:  newSource(),
				peer:    p,
				replies: make(chan FullFrame, 1),
				dest:    ff.Source,
				ISeqno:  1,
			}
			p.call[c.source] = &c
			p.callByDest[Source(c.dest)] = &c
			p.debug("added call for source 0x%x\n", c.source)
			p.callMutex.Unlock()
			go handleIncomingCall(n, ff, &c)
			continue
		}
		p.callMutex.Unlock()
		call.mutex.Lock()
		/*
			if ff.F && ff.OSeqno != call.ISeqno && ff.FrameType != FrameTypeVoice {
				call.mutex.Unlock()
				if call.OSeqno == call.lastACK.FullFrameHeader.OSeqno+1 {
					if call.ackTries > ackLimit {
						p.debug("%05d not retransmitting ACK (limit reached)\n", n)
						continue
					}
					if call.ackTries > 0 {
						p.debug("%05d retransmitting ACK\n", n)
					} else {
						p.debug("%05d %v\n", n, call.lastACK)
					}
					p.sendFrame(call.lastACK)
					call.ackTries++
					continue
				}
				p.debug("%05d skipping reply for already-received frame\n", n)
				continue
			}*/
		if call.dest == 0 {
			call.dest = ff.Source
		}
		if ff.OSeqno >= call.ISeqno+1 {
			// we lost something; await retransmission
			call.mutex.Unlock()
			p.debug("we lost someting; await retransmission\n")
			continue
		}
		if ff.F && (ff.FrameType != FrameTypeIAX || ff.Subclass != IAXSubclassACK) {
			call.ISeqno++
		}
		call.mutex.Unlock()
		if ff.IAX.Refresh != 0 {
			call.checkRefresh(ff.IAX.Refresh)
		}
		call.replies <- ff
	}
}

func (c *Call) checkRefresh(s uint16) {
	c.mutex.Lock()
	proposedRefresh := time.Now().Add(time.Duration(s) * time.Second)
	if c.refreshBefore.IsZero() {
		c.peer.debug("setting refresh time to %d\n", s)
		c.refreshBefore = proposedRefresh
	} else if c.refreshBefore.After(proposedRefresh) {
		c.peer.debug("updating refresh time to sooner time, %d\n", s)
		c.refreshBefore = proposedRefresh
	}
	c.mutex.Unlock()
}

func (c *Call) transmitUntilReply(ff FullFrame) (FullFrame, error) {
	return c.transmitUntilReplyChan(ff, c.replies)
}

func (c *Call) transmitUntilReplyChan(ff FullFrame, replies chan FullFrame) (FullFrame, error) {
	if !ff.F {
		return FullFrame{}, errors.New("mini-frame won't be retransmitted or acknowledged; invalid invocation of transmitUntilReply")
	}
	ff.FullFrameHeader = c.sequenceFF(ff.FullFrameHeader)
	if ff.IAX.Refresh != 0 {
		c.checkRefresh(ff.IAX.Refresh)
	}
	err := c.peer.sendFrame(ff)
	if err != nil {
		return FullFrame{}, fmt.Errorf("transmit: %w", err)
	}
	for {
		select {
		case r := <-replies:
			return r, nil
		case <-time.After(time.Second):
			ff.R = true
			err = c.peer.sendFrame(ff)
			if err != nil {
				return FullFrame{}, fmt.Errorf("retransmit: %w", err)
			}
		}
	}
}

func IAXSubclassName(s uint8) string {
	switch s {
	case IAXSubclassNEW:
		return "NEW"
	case IAXSubclassPING:
		return "PING"
	case IAXSubclassPONG:
		return "PONG"
	case IAXSubclassACK:
		return "ACK"
	case IAXSubclassHANGUP:
		return "HANGUP"
	case IAXSubclassREJECT:
		return "REJECT"
	case IAXSubclassACCEPT:
		return "ACCEPT"
	case IAXSubclassAUTHREQ:
		return "AUTHREQ"
	case IAXSubclassAUTHREP:
		return "AUTHREP"
	case IAXSubclassINVAL:
		return "INVAL"
	case IAXSubclassLAGRQ:
		return "LAGRQ"
	case IAXSubclassLAGRP:
		return "LAGRP"
	case IAXSubclassREGREQ:
		return "REGREQ"
	case IAXSubclassREGAUTH:
		return "REGAUTH"
	case IAXSubclassREGACK:
		return "REGACK"
	case IAXSubclassREGREJ:
		return "REGREJ"
	case IAXSubclassREGREL:
		return "REGREL"
	case IAXSubclassVNAK:
		return "VNAK"
	case IAXSubclassDPREQ:
		return "DPREQ"
	case IAXSubclassDPREP:
		return "DPREP"
	case IAXSubclassDIAL:
		return "DIAL"
	case IAXSubclassTXREQ:
		return "TXREQ"
	case IAXSubclassTXCNT:
		return "TXCNT"
	case IAXSubclassTXACC:
		return "TXACC"
	case IAXSubclassTXREADY:
		return "TXREADY"
	case IAXSubclassTXREL:
		return "TXREL"
	case IAXSubclassTXREJ:
		return "TXREJ"
	case IAXSubclassQUELCH:
		return "QUELCH"
	case IAXSubclassUNQUELCH:
		return "UNQUELCH"
	case IAXSubclassPOKE:
		return "POKE"
	case IAXSubclassMWI:
		return "MWI"
	case IAXSubclassUNSUPPORT:
		return "UNSUPPORT"
	}
	return fmt.Sprintf("iaxsc=0x%x", s)
}

func ControlSubclassString(s uint8) string {
	switch s {
	case ControlHANGUP:
		return "HANGUP"
	case ControlRINGING:
		return "RINGING"
	case ControlANSWER:
		return "ANSWER"
	case ControlBUSY:
		return "BUSY"
	case ControlCONGESTION:
		return "CONGESTION"
	case ControlPROGRESS:
		return "PROGRESS"
	case ControlHOLD:
		return "HOLD"
	case ControlUNHOLD:
		return "UNHOLD"
	}
	return fmt.Sprintf("csc=0x%x", s)
}

func (ff FullFrame) String() string {
	r := " "
	if ff.R {
		r = "R"
	}
	res := fmt.Sprintf("%04x->%04x%si:%d,o:%d ", ff.Source, ff.Dest, r, ff.ISeqno,
		ff.OSeqno)
	switch ff.FrameType {
	case FrameTypeIAX:
		res += IAXSubclassName(ff.Subclass)
	case FrameTypeControl:
		res += "Control " + ControlSubclassString(ff.Subclass)
	case FrameTypeVoice:
		res += fmt.Sprintf("Voice sc=0x%x %d", ff.Subclass, len(ff.Voice))
	case FrameTypeDTMF:
		res += "DTMF '" + ff.DTMF + "'"
	default:
		res += fmt.Sprintf("ft=0x%x", ff.FrameType)
	}
	add := func(s, name string) string {
		if s == "" {
			return ""
		}
		return " " + name + `:"` + s + `"`
	}
	res += add(ff.IAX.Username, "Username")
	res += add(ff.IAX.Challenge, "Challenge")
	if ff.IAX.AuthMethods != 0 {
		res += add(fmt.Sprintf("0x%x", ff.IAX.AuthMethods), "AuthMethods")
	}
	if ff.IAX.Refresh != 0 {
		res += add(fmt.Sprintf("%d", ff.IAX.Refresh), "Refresh")
	}
	res += add(ff.IAX.CallingName, "CallingName")
	res += add(ff.IAX.CalledNumber, "CalledNumber")
	res += add(ff.IAX.CodecPrefs, "CodecPrefs")
	if ff.IAX.Format != 0 {
		res += add(fmt.Sprintf("0x%x", ff.IAX.Format), "Format")
	}
	if ff.IAX.Capability != 0 {
		res += add(fmt.Sprintf("0x%x", ff.IAX.Capability), "Capability")
	}
	if ff.IAX.HangupCauseCode != 0 {
		res += add(fmt.Sprintf("0x%x", ff.IAX.HangupCauseCode), "HangupCauseCode")
	}
	if ff.IAX.SamplingRate != 0 {
		res += add(fmt.Sprintf("%d", ff.IAX.SamplingRate), "SamplingRate")
	}
	res += add(ff.IAX.DateTime, "DateTime")
	res += add(ff.IAX.MD5Result, "MD5Result")
	res += add(ff.IAX.CallingNumber, "CallingNumber")
	res += add(ff.IAX.Cause, "Cause")
	return res
}

const IAXMediaFormatMuLaw = uint32(0x00000004)

func handleIncomingCall(n uint32, ff FullFrame, c *Call) {
	defer func() {
		c.peer.debug("taking down mapping for call with source %04x\n", c.source)
		c.peer.callMutex.Lock()
		close(c.replies)
		delete(c.peer.call, c.source)
		if c.dest != uint16(0) {
			delete(c.peer.call, Source(c.dest))
		}
		c.peer.callMutex.Unlock()
	}()

	if ff.IAX.CalledNumber != "s" {
		c.peer.debug("%05d rejecting call to \"%s\"\n", n, ff.IAX.CalledNumber)
		c.transmitUntilReply(NewIAXFrame(IAXSubclassREJECT, IAXFramePart{}))
		return
	}
	sync := make(chan FullFrame, 1)
	done := make(chan struct{}, 1)
	var ignoreACK bool
	go func() {
		for f := range c.replies {
			switch f.FrameType {
			case FrameTypeIAX:
				switch f.Subclass {
				case IAXSubclassHANGUP:
					c.ACKIAX(f)
					done <- struct{}{}
					return
				case IAXSubclassLAGRQ:
					ff := NewIAXFrame(IAXSubclassLAGRP, IAXFramePart{})
					ff.FullFrameHeader = c.sequenceFF(ff.FullFrameHeader)
					ff.Timestamp = f.Timestamp
					c.peer.sendFrame(ff)
				case IAXSubclassPING:
					ff := NewIAXFrame(IAXSubclassPONG, IAXFramePart{})
					ff.FullFrameHeader = c.sequenceFF(ff.FullFrameHeader)
					ff.Timestamp = f.Timestamp
					c.peer.sendFrame(ff)

				case IAXSubclassACK:
					if ignoreACK {
						continue
					}
					c.mutex.Lock()
					if f.Timestamp == c.lastAudioTimestamp {
						c.mutex.Unlock()
						continue
					}
					c.mutex.Unlock()
					sync <- f
				}

			case FrameTypeControl, FrameTypeDTMF:
				c.peer.debug("got async, ACK\n")
				c.ACKIAX(f)
				c.peer.debug("done ACK\n")
			case FrameTypeVoice:
				if f.F {
					c.ACKIAX(f)
				}
				c.sendVoice(f.Voice)

			default:
				c.peer.debug("replying UNSUPPORT\n")
				ff := NewIAXFrame(IAXSubclassUNSUPPORT, IAXFramePart{})
				ff.FullFrameHeader = c.sequenceFF(ff.FullFrameHeader)
				ff.Timestamp = f.Timestamp
				c.peer.sendFrame(ff)
			}
		}
	}()
	if false {
		if _, err := c.transmitUntilReply(NewControlFrame(ControlRINGING)); err != nil {
			c.peer.debug("call error: %v\n", err)
			return
		}
	}

	action := CallReject
	if c.peer.Callee != nil {
		action = c.peer.Callee.AcceptOrReject()
	}

	switch action {
	case callAccept:
		if _, err := c.transmitUntilReplyChan(NewIAXFrame(IAXSubclassACCEPT, IAXFramePart{
			Format: IAXMediaFormatMuLaw}), sync); err != nil {
			c.peer.debug("call error: %v\n", err)
			return
		}
	case CallReject:
		if _, err := c.transmitUntilReplyChan(NewIAXFrame(IAXSubclassREJECT, IAXFramePart{}), sync); err != nil {
			c.peer.debug("call error: %v\n", err)
			return
		}
	}
	//} else if mode != ModeEcho {
	//if _, err := c.transmitUntilReply(NewControlFrame(ControlBUSY)); err != nil {
	//fmt.Printf("call error: %v\n", err)
	//return
	//}
	/*
		if _, err := c.transmitUntilReply(NewIAXFrame(IAXSubclassHANGUP, IAXFramePart{
			HangupCauseCode: HangupCauseBUSY,
		})); err != nil {
			c.peer.debug("call error: %v\n", err)
			return
		}
	*/
	//for {
	//	<-c.replies
	//}
	//} else //{
	//if _, err := c.transmitUntilReplyChan(NewIAXFrame(IAXSubclassACCEPT, IAXFramePart{
	//Format: IAXMediaFormatMuLaw}), sync); err != nil {
	//fmt.Printf("call error: %v\n", err)
	//return
	//}
	//if _, err := c.transmitUntilReplyChan(NewControlFrame(ControlANSWER), sync); err != nil {
	//fmt.Printf("call error: %v\n", err)
	//return
	//}
	//ignoreACK = true
	//}
	<-done
	fmt.Printf("DONE CALL\n")
}

const (
	ControlHANGUP     = 0x01
	ControlRINGING    = 0x03
	ControlANSWER     = 0x04
	ControlBUSY       = 0x05
	ControlCONGESTION = 0x08
	ControlPROGRESS   = 0x0e
	ControlHOLD       = 0x10
	ControlUNHOLD     = 0x11
)

const (
	HangupCauseNORMAL = 16
	HangupCauseBUSY   = 17
)

func (c *Call) sendVoice(data []byte) error {
	ff := FullFrame{
		FullFrameHeader: FullFrameHeader{
			FrameType: FrameTypeVoice,
			Subclass:  0x0004,
		},
		Voice: data,
	}
	ff.FullFrameHeader = c.sequenceFF(ff.FullFrameHeader)
	err := c.peer.sendFrame(ff)
	if err != nil {
		return fmt.Errorf("transmit: %w", err)
	}
	return nil
}

func (c *Call) GetRefresh() time.Duration {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.refreshBefore.Sub(time.Now())
}

func (p *Peer) debug(format string, args ...interface{}) {
	if p.Debug == nil {
		return
	}
	p.Debug(format, args...)
}
