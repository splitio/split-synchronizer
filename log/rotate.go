package log

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

type FileRotateOptions struct {
	RotateBytes int64
	Compress    bool
	Path        string
}

type FileRotate struct {
	fl      *os.File
	fm      *sync.Mutex
	options *FileRotateOptions
}

func NewFileRotate(opt *FileRotateOptions) (*FileRotate, error) {

	fileWriter, err := os.OpenFile(opt.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("Error opening log file: %s \n", err.Error())
		return nil, err
	}

	fl := &FileRotate{fl: fileWriter, fm: &sync.Mutex{}, options: opt}
	return fl, nil
}

func (f *FileRotate) shouldRotate(bytesToAdd int64) bool {
	fi, err := f.fl.Stat()
	if err != nil {
		fmt.Println("Error getting stats of file")
		return false
	}

	if fi.Size()+bytesToAdd >= f.options.RotateBytes {
		return true
	}

	return false
}

func (f *FileRotate) rotate() error {

	f.fl.Close()

	newName := f.options.Path + "." + strconv.Itoa(int(time.Now().UnixNano()))
	err := os.Rename(f.options.Path, newName)
	if err != nil {
		fmt.Printf("Error rotating log file: %s \n", err.Error())
		return err
	}

	f.fl, err = os.OpenFile(f.options.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("Error reopening log file: %s \n", err.Error())
		return err
	}

	return nil
}

func (f *FileRotate) Write(p []byte) (n int, err error) {
	f.fm.Lock()
	if f.shouldRotate(int64(len(p))) {
		f.rotate()
	}

	n, err = f.fl.Write(p)
	f.fm.Unlock()

	return n, err
}
