package log

import (
	"io/ioutil"
	"os"
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

	opt := &FileRotateOptions{MaxBytes: 100, Path: tmpDir + "rotate.log", BackupCount: 2}
	fr, err := NewFileRotate(opt)
	if err != nil {
		t.Error(err)
	}

	var toWrite = "String with data to log added the UNIX TIMESTAMP " + time.Now().String()
	n, errw := fr.write([]byte(toWrite))
	if errw != nil {
		t.Error(errw)
	}

	if n != len([]byte(toWrite)) {
		t.Error("Amount of written bytes is invalid")
	}

	var shouldRotateBytes = "String with data to force rotation at log file " + time.Now().String()
	if !fr.shouldRotate(int64(len(shouldRotateBytes))) {
		t.Error("The log file should rotate due MaxBytes condition")
	} else {
		if err = fr.rotate(); err != nil {
			t.Error(err)
		}
	}

	toWrite = "String with data to log added the UNIX TIMESTAMP " + time.Now().String()
	if fr.shouldRotate(int64(len(toWrite))) {
		t.Error("The log file should not rotate at this point")
	}
	n, errw = fr.Write([]byte(toWrite))
	if errw != nil {
		t.Error(errw)
	}
}
