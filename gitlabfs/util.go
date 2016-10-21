package gitlabfs

import (
	"archive/zip"
	"io/ioutil"
	"os"
)

func UnlinkedTempFile(dir, prefix string) (f *os.File, err error) {
	f, err = ioutil.TempFile(dir, prefix)
	if err == nil {
		defer os.Remove(f.Name())
	}
	return f, err
}

type ZipFileReader struct {
	f *os.File
	zip.Reader
}

func ZipReaderFromFile(f *os.File) (*ZipFileReader, error) {
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	zipr, err := zip.NewReader(f, fi.Size())
	if err != nil {
		return nil, err
	}

	return &ZipFileReader{
		f:      f,
		Reader: *zipr,
	}, nil
}

func (z *ZipFileReader) Close() {
	z.f.Close()
}
