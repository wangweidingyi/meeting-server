package protocol

import (
	"encoding/binary"
	"errors"
)

var udpMagic = [4]byte{'M', 'T', 'N', 'G'}

const udpPacketVersion = 1

type AudioSourceType uint8

const (
	AudioSourceMixed          AudioSourceType = 1
	AudioSourceMicrophone     AudioSourceType = 2
	AudioSourceSystemLoopback AudioSourceType = 3
)

type UDPAudioPacket struct {
	Version     uint8
	SourceType  AudioSourceType
	SessionID   string
	Sequence    uint64
	StartedAtMS uint64
	DurationMS  uint32
	Payload     []byte
}

func (p UDPAudioPacket) Encode() ([]byte, error) {
	sessionIDBytes := []byte(p.SessionID)
	if len(sessionIDBytes) > int(^uint16(0)) {
		return nil, errors.New("session id is too long")
	}

	size := 4 + 1 + 1 + 2 + len(sessionIDBytes) + 8 + 8 + 4 + 4 + len(p.Payload)
	buffer := make([]byte, size)
	cursor := 0

	copy(buffer[cursor:], udpMagic[:])
	cursor += len(udpMagic)
	buffer[cursor] = p.versionOrDefault()
	cursor++
	buffer[cursor] = byte(p.SourceType)
	cursor++
	binary.BigEndian.PutUint16(buffer[cursor:], uint16(len(sessionIDBytes)))
	cursor += 2
	copy(buffer[cursor:], sessionIDBytes)
	cursor += len(sessionIDBytes)
	binary.BigEndian.PutUint64(buffer[cursor:], p.Sequence)
	cursor += 8
	binary.BigEndian.PutUint64(buffer[cursor:], p.StartedAtMS)
	cursor += 8
	binary.BigEndian.PutUint32(buffer[cursor:], p.DurationMS)
	cursor += 4
	binary.BigEndian.PutUint32(buffer[cursor:], uint32(len(p.Payload)))
	cursor += 4
	copy(buffer[cursor:], p.Payload)

	return buffer, nil
}

func DecodeUDPAudioPacket(raw []byte) (UDPAudioPacket, error) {
	cursor := 0

	magic, err := readExact(raw, &cursor, len(udpMagic))
	if err != nil {
		return UDPAudioPacket{}, err
	}
	if string(magic) != string(udpMagic[:]) {
		return UDPAudioPacket{}, errors.New("invalid udp audio packet magic")
	}

	version, err := readU8(raw, &cursor)
	if err != nil {
		return UDPAudioPacket{}, err
	}

	sourceTypeValue, err := readU8(raw, &cursor)
	if err != nil {
		return UDPAudioPacket{}, err
	}

	sourceType := AudioSourceType(sourceTypeValue)
	switch sourceType {
	case AudioSourceMixed, AudioSourceMicrophone, AudioSourceSystemLoopback:
	default:
		return UDPAudioPacket{}, errors.New("unsupported audio source type")
	}

	sessionIDLength, err := readU16(raw, &cursor)
	if err != nil {
		return UDPAudioPacket{}, err
	}

	sessionIDBytes, err := readExact(raw, &cursor, int(sessionIDLength))
	if err != nil {
		return UDPAudioPacket{}, err
	}

	sequence, err := readU64(raw, &cursor)
	if err != nil {
		return UDPAudioPacket{}, err
	}

	startedAtMS, err := readU64(raw, &cursor)
	if err != nil {
		return UDPAudioPacket{}, err
	}

	durationMS, err := readU32(raw, &cursor)
	if err != nil {
		return UDPAudioPacket{}, err
	}

	payloadLength, err := readU32(raw, &cursor)
	if err != nil {
		return UDPAudioPacket{}, err
	}

	payload, err := readExact(raw, &cursor, int(payloadLength))
	if err != nil {
		return UDPAudioPacket{}, err
	}

	return UDPAudioPacket{
		Version:     version,
		SourceType:  sourceType,
		SessionID:   string(sessionIDBytes),
		Sequence:    sequence,
		StartedAtMS: startedAtMS,
		DurationMS:  durationMS,
		Payload:     append([]byte(nil), payload...),
	}, nil
}

func (p UDPAudioPacket) ToMixedAudioPacket() MixedAudioPacket {
	return MixedAudioPacket{
		SessionID:   p.SessionID,
		Sequence:    p.Sequence,
		StartedAtMS: p.StartedAtMS,
		DurationMS:  p.DurationMS,
		Payload:     append([]byte(nil), p.Payload...),
	}
}

func (p UDPAudioPacket) versionOrDefault() uint8 {
	if p.Version == 0 {
		return udpPacketVersion
	}

	return p.Version
}

func readExact(raw []byte, cursor *int, length int) ([]byte, error) {
	end := *cursor + length
	if end > len(raw) {
		return nil, errors.New("unexpected end of udp audio packet")
	}

	slice := raw[*cursor:end]
	*cursor = end
	return slice, nil
}

func readU8(raw []byte, cursor *int) (uint8, error) {
	value, err := readExact(raw, cursor, 1)
	if err != nil {
		return 0, err
	}

	return value[0], nil
}

func readU16(raw []byte, cursor *int) (uint16, error) {
	value, err := readExact(raw, cursor, 2)
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint16(value), nil
}

func readU32(raw []byte, cursor *int) (uint32, error) {
	value, err := readExact(raw, cursor, 4)
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint32(value), nil
}

func readU64(raw []byte, cursor *int) (uint64, error) {
	value, err := readExact(raw, cursor, 8)
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint64(value), nil
}
