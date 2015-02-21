package resources

import (
	"bytes"
	"fmt"
	"io"
	"text/template"
)

var pkg *template.Template

func reader(input io.Reader) (string, error) {

	var (
		buff       bytes.Buffer
		err        error
		blockwidth int = 12
		curblock   int = 0
	)

	b := make([]byte, blockwidth)

	for n, err := input.Read(b); err == nil; n, err = input.Read(b) {
		for i := 0; i < n; i++ {
			fmt.Fprintf(&buff, "0x%02x,", b[i])
			curblock++
			if curblock < blockwidth {
				continue
			}
			buff.Write([]byte{'\n'})
			buff.Write([]byte{'\t', '\t'})
			curblock = 0
		}
	}

	return buff.String(), err
}

func init() {

	pkg = template.Must(template.New("file").Funcs(template.FuncMap{"reader": reader}).Parse(` File{
	  data: []byte{
	{{ reader . }} 
  },
  fi: FileInfo {
	name:    "{{ .Stat.Name }}", 
    size:    {{ .Stat.Size }},
	modTime: time.Unix({{ .Stat.ModTime.Unix }},{{ .Stat.ModTime.UnixNano }}),
    isDir:   {{ .Stat.IsDir }},
  },
}`))

	pkg = template.Must(pkg.New("pkg").Parse(`{{ if .Tag }}// +build {{ .Tag }} 

{{ end }}//Generated by github.com/omeid/slurp/resources
package {{ .Pkg }}

import (
  "net/http"
  "time"
  "bytes"
  "os"
  "strings"
)


{{ if .Declare }}
var {{ .Var }} http.FileSystem
{{ end }}

// Helper functions for easier file access.
func Open(name string) (http.File, error) {
	return {{ .Var }}.Open(name)
}

// http.FileSystem implementation.
type FileSystem struct {
	files map[string]File
}

func (fs *FileSystem) Open(name string) (http.File, error) {
	file, ok := fs.files[name]
	if !ok {
		files := []os.FileInfo{}
		for path, file := range fs.files {
			if strings.HasPrefix(path, name) {
				s, _ := file.Stat()
				files = append(files, s)
			}
		}

		if len(files) < 0 {
			return nil, os.ErrNotExist
		}

		//We have a directory.
		return &File{
		  fi: FileInfo{
				isDir: true,
				files: files,
			}}, nil
	}
	file.Reader = bytes.NewReader(file.data)
	return &file, nil
}

type File struct {
	*bytes.Reader
	data []byte
	fi FileInfo
}

// A noop-closer.
func (f *File) Close() error {
	return nil
}

func (f *File) Readdir(count int) ([]os.FileInfo, error) {
  return nil, os.ErrNotExist
}


func (f *File) Stat() (os.FileInfo, error) {
  return &f.fi, nil
}

type FileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
	sys     interface{}
	
	files []os.FileInfo
}

func (f *FileInfo) Name() string {
	return f.name
}
func (f *FileInfo) Size() int64 {
	return f.size
}

func (f *FileInfo) Mode() os.FileMode {
	return f.mode
}

func (f *FileInfo) ModTime() time.Time {
	return f.modTime
}

func (f *FileInfo) IsDir() bool {
	return f.isDir
}

func (f *FileInfo) Readdir(count int) ([]os.FileInfo, error) {
	return f.files, nil
}

func (f *FileInfo) Sys() interface{} {
	return f.sys
}


func init() {
  {{ .Var }} = &FileSystem{
		files: map[string]File{
		  {{range $path, $file := .Files }} "/{{ $path }}": {{ template "file" $file }}, {{ end }}
		},
	  }
}
`))
}
