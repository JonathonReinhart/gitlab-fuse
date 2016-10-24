package gitlabfs

import (
	"archive/zip"
	"io/ioutil"
	"os"
	"time"
)

func ConvertDosDateTime(dosdate, dostime uint16) time.Time {
	// https://msdn.microsoft.com/en-us/library/windows/desktop/ms724274.aspx
	// http://www.vsft.com/hal/dostime.htm

	year := int((dosdate & 0xFE00) >> 9) // bits 15:9
	month := int((dosdate & 0x1E0) >> 5) // bits 8:5
	day := int(dosdate & 0x1F)           // bits 4:0

	hour := int((dostime & 0xF800) >> 11) // bits 15:11
	min := int((dostime & 0x7E0) >> 5)    // bits 10:5
	sec := int(dostime & 0x1F)            // bits 4:0

	nsec := 0

	return time.Date(1980+year, time.Month(month), day, hour, min, sec*2, nsec, time.UTC)
}

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
