package log

import (
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRotateFile(t *testing.T) {

	stdoutWriter := ioutil.Discard //os.Stdout
	Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	tmpDir := os.TempDir()
	if !strings.HasSuffix(tmpDir, "/") {
		tmpDir = tmpDir + "/"
	}

	opt := &FileRotateOptions{RotateBytes: 100, Path: tmpDir + "rotate.log"}
	fr, err := NewFileRotate(opt)
	if err != nil {
		t.Error(err)
	}

	for i := 0; i < 5; i++ {
		var toWrite = "String with data to log added the UNIX TIMESTAMP " + strconv.Itoa(int(time.Now().UnixNano()))
		n, errw := fr.Write([]byte(toWrite))
		if errw != nil {
			t.Error(errw)
		}
		if n != len([]byte(toWrite)) {
			t.Error("Amount of written bytes is invalid")
		}
		time.Sleep(time.Duration(100) * time.Millisecond)
	}

}
