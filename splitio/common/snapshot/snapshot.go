package snapshot

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Snapshot type constants
const (
	_ = iota
	StorageBoltDB
)

// ErrNonexistantFile represents an error when the snapshot passed in to be decoded is missing
var ErrNonexistantFile = errors.New("cannot find snapshot file")

// ErrEncMetadata represents an error when encoding snapshot metadata
var ErrEncMetadata = errors.New("snapshot metadata cannot be encoded")

// ErrSnapshotSize represents an error when reading the snapshot size
var ErrSnapshotSize = errors.New("invalid snapshot size")

// ErrMetadataSizeRead represents an error when reading the metadata size
var ErrMetadataSizeRead = errors.New("snapshot metadata size cannot be decoded")

// ErrMetadataRead represents an error when metadata cannot be decoded
var ErrMetadataRead = errors.New("snapshot metadata cannot be decoded")

// Metadata represents the Snapshot metadata object
type Metadata struct {
	Version     uint64
	Storage     uint64
	SpecVersion string
}

// Snapshot represents a snapshot struct with metadata and data
type Snapshot struct {
	meta Metadata
	data []byte
}

// New returns an instance of Snapshot object with the parameter set
func New(meta Metadata, data []byte) (*Snapshot, error) {

	var b bytes.Buffer
	gw, err := gzip.NewWriterLevel(&b, gzip.BestSpeed)
	if err != nil {
		return nil, fmt.Errorf("error building gzip writer: %w", err)
	}
	gw.Write(data)
	gw.Close()

	return &Snapshot{meta: meta, data: b.Bytes()}, nil
}

// Meta returns a copy of the Snapshot Metadata object
func (s *Snapshot) Meta() Metadata {
	return s.meta
}

// Data returns the unzipped Snapshot data
func (s *Snapshot) Data() ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewBuffer(s.data))
	defer gz.Close()
	data, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("error reading gzip data: %w", err)
	}
	return data, nil
}

// Encode returns the bytes slice snapshot representation
// Snapshot Layout:
//
//				         |metadata-size|metadata|data|
//
//	        metadata-size: uint64 (8 bytes) specifies the amount of metadata bytes
//	        metadata: Gob encoded of Metadata struct
//	        data: Proxy data, byte slice. The Metadata have information about it, Storage, Gzipped and version.
func (s *Snapshot) Encode() ([]byte, error) {

	metaBytes, err := metaToBytes(s.meta)
	if err != nil {
		return nil, fmt.Errorf("%w | %s", ErrEncMetadata, err)
	}

	metaBytesLen, err := lenToBytes(int64(len(metaBytes)))
	if err != nil {
		return nil, fmt.Errorf("%w | %s", ErrEncMetadata, err)
	}

	totalBytes := len(metaBytesLen) + len(metaBytes) + len(s.data)
	var snapbytes = make([]byte, totalBytes, totalBytes)

	// copying metadata-size
	for i := 0; i < len(metaBytesLen); i++ {
		snapbytes[i] = metaBytesLen[i]
	}

	// copying metadata
	metadataOffset := len(metaBytesLen)
	for i := 0; i < len(metaBytes); i++ {
		snapbytes[metadataOffset+i] = metaBytes[i]
	}

	// copying data
	dataOffset := len(metaBytesLen) + len(metaBytes)
	for i := 0; i < len(s.data); i++ {
		snapbytes[dataOffset+i] = s.data[i]
	}

	return snapbytes, nil
}

// WriteDataToTmpFile writes the data field (unzipped) to a temporal file
func (s *Snapshot) WriteDataToTmpFile() (string, error) {
	tmpDir := os.TempDir()
	if !strings.HasSuffix(tmpDir, "/") {
		tmpDir = tmpDir + "/"
	}

	path := fmt.Sprintf("%ssplit.proxy.%s.data", tmpDir, strings.ReplaceAll(uuid.NewString(), "-", ""))
	return path, s.WriteDataToFile(path)
}

// WriteDataToFile writes the data field (unzipped) to a given file path
func (s *Snapshot) WriteDataToFile(path string) error {
	snapData, err := s.Data()
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(path, snapData, 0644)
	if err != nil {
		return err
	}

	return nil
}

// DecodeFromFile decodes a snapshot file from a given path
func DecodeFromFile(path string) (*Snapshot, error) {
	snapshotFilePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("error getting absolute path to snapshot: %w", err)
	}

	if !snapshotExists(snapshotFilePath) {
		return nil, ErrNonexistantFile
	}

	snapshotBytes, err := ioutil.ReadFile(snapshotFilePath)
	if err != nil {
		return nil, fmt.Errorf("error reading snapshot file")
	}

	return Decode(snapshotBytes)
}

// Decode decode a byte slice and returns the Snapshot object
func Decode(snap []byte) (*Snapshot, error) {

	if len(snap) < 8 {
		return nil, ErrSnapshotSize
	}

	metadataSize, err := bytesToUint64(snap[0:8])
	if err != nil {
		return nil, fmt.Errorf("%w | %s", ErrMetadataSizeRead, err)
	}

	if len(snap) < int(metadataSize) {
		return nil, ErrSnapshotSize
	}
	metadata, err := bytesToMetadata(snap[8 : int(metadataSize)+8])
	if err != nil {
		return nil, fmt.Errorf("%w | %s", ErrMetadataRead, err)
	}

	return &Snapshot{meta: *metadata, data: snap[8+int(metadataSize):]}, nil
}

func metaToBytes(meta Metadata) ([]byte, error) {
	var buff bytes.Buffer
	encErr := gob.NewEncoder(&buff).Encode(meta)
	if encErr != nil {
		return nil, encErr
	}

	return buff.Bytes(), nil
}

func bytesToMetadata(b []byte) (*Metadata, error) {
	var buff bytes.Buffer
	buff.Write(b)

	var meta = Metadata{}
	err := gob.NewDecoder(&buff).Decode(&meta)
	if err != nil {
		return nil, err
	}

	return &meta, nil
}

func lenToBytes(l int64) ([]byte, error) {
	var b bytes.Buffer
	length := uint64(l)
	err := binary.Write(&b, binary.LittleEndian, length)
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func bytesToUint64(bt []byte) (uint64, error) {
	var b bytes.Buffer
	b.Write(bt)

	var length uint64
	err := binary.Read(&b, binary.LittleEndian, &length)
	if err != nil {
		return 0, err
	}

	return length, nil
}

func snapshotExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
