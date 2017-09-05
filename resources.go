// Package resources provides unfancy resources embedding with Go.
package resources

import (
	"bytes"
	"fmt"
	"go/format"
	"io"
	"log"
	"os"
	"strings"
	"text/template"
)

// File mimicks the os.File and http.File interface.
type File interface {
	io.Reader
	Stat() (os.FileInfo, error)
}

// New creates a new Package.
func New() *Package {
	return &Package{
		Config: Config{
			Pkg:     "resources",
			Var:     "FS",
			Declare: true,
		},
		Files: make(map[string]File),
	}
}

// Config defines some details about the output file
type Config struct {
	Pkg     string // Pkg holds the package name
	Var     string // Var holds the variable name for the virtual filesystem
	Tag     string // Tag may hold an optional build tag, unless empty
	Declare bool   // Declare controls if the Var should be declared as well
	Format  bool   // Format controls, whether gofmt should be applied to the output
}

// A Package describes a collection of files and how they should be tranformed
// to an output.
type Package struct {
	Config
	Files map[string]File
}

// Add a file to the package at the give path.
func (p *Package) Add(path string, file File) error {
	p.Files[path] = file
	return nil
}

// AddFile is a helper function that adds the files from the path into the
// package under the path file.
func (p *Package) AddFile(path string, file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	return p.Add(path, f)
}

// Build compiles the package and writes it into an io.Writer.
func (p *Package) Build(out io.Writer) error {
	return pkg.Execute(out, p)
}

// Write builds the package (via Build) and writes the output the the file
// given by the path argument.
func (p *Package) Write(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			log.Panicf("Failed to close file: %s", err)
		}
	}()

	if !p.Format {
		return p.Build(f)
	}

	buf := &bytes.Buffer{}
	if e := p.Build(buf); e != nil {
		return e
	}

	fmted, e := format.Source(buf.Bytes())
	if e != nil {
		return e
	}
	_, e = f.Write(fmted)
	return e
}

var (
	// Template
	pkg *template.Template

	// BlockWidth allows to adjust the number of bytes per line in the generated file
	BlockWidth = 12
)

func reader(input io.Reader, indent int) (string, error) {
	var (
		buff      bytes.Buffer
		err       error
		curblock  = 0
		linebreak = "\n" + strings.Repeat("\t", indent)
	)

	b := make([]byte, BlockWidth)

	for n, e := input.Read(b); e == nil; n, e = input.Read(b) {
		for i := 0; i < n; i++ {
			_, e = fmt.Fprintf(&buff, "0x%02x,", b[i])
			if e != nil {
				err = e
				break
			}
			curblock++
			if curblock < BlockWidth {
				buff.WriteRune(' ')
				continue
			}
			buff.WriteString(linebreak)
			curblock = 0
		}
	}

	return buff.String(), err
}

func init() {
	pkg = template.Must(template.New("file").Funcs(template.FuncMap{"reader": reader}).Parse(fileTemplate))
	pkg = template.Must(pkg.New("pkg").Parse(pkgTemplate))
}

const fileTemplate = `File{
				data: []byte{
					{{ reader . 5 }}
				},
				fi: FileInfo{
					name:    "{{ .Stat.Name }}",
					size:    {{ .Stat.Size }},
					modTime: time.Unix(0, {{ .Stat.ModTime.UnixNano }}),
					isDir:   {{ .Stat.IsDir }},
				},
			}`

const pkgTemplate = `{{ if .Tag }}// +build {{ .Tag }}

{{ end }}// Package {{ .Pkg }} is generated by github.com/omeid/go-resources
package {{ .Pkg }}

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileSystem is an http.FileSystem implementation.
type FileSystem struct {
	files map[string]File
}

// String returns the content of the file as string.
func (fs *FileSystem) String(name string) (string, bool) {
	if filepath.Separator != '/' && strings.IndexRune(name, filepath.Separator) >= 0 ||
		strings.Contains(name, "\x00") {
		return "", false
	}

	file, ok := fs.files[name]

	if !ok {
		return "", false
	}

	return string(file.data), true
}

// Open implements http.FileSystem.Open
func (fs *FileSystem) Open(name string) (http.File, error) {
	if filepath.Separator != '/' && strings.IndexRune(name, filepath.Separator) >= 0 ||
		strings.Contains(name, "\x00") {
		return nil, errors.New("http: invalid character in file path")
	}
	file, ok := fs.files[name]
	if !ok {
		files := []os.FileInfo{}
		for path, file := range fs.files {
			if strings.HasPrefix(path, name) {
				fi := file.fi
				files = append(files, &fi)
			}
		}

		if len(files) == 0 {
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

// File implements http.File
type File struct {
	*bytes.Reader
	data []byte
	fi   FileInfo
}

// Close is a noop-closer.
func (f *File) Close() error {
	return nil
}

// Readdir implements http.File.Readdir
func (f *File) Readdir(count int) ([]os.FileInfo, error) {
	return nil, os.ErrNotExist
}

// Stat implements http.Stat.Readdir
func (f *File) Stat() (os.FileInfo, error) {
	return &f.fi, nil
}

// FileInfo implements the os.FileInfo interface.
type FileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
	sys     interface{}

	files []os.FileInfo
}

// Name implements os.FileInfo.Name
func (f *FileInfo) Name() string {
	return f.name
}

// Size implements os.FileInfo.Size
func (f *FileInfo) Size() int64 {
	return f.size
}

// Mode implements os.FileInfo.Mode
func (f *FileInfo) Mode() os.FileMode {
	return f.mode
}

// ModTime implements os.FileInfo.ModTime
func (f *FileInfo) ModTime() time.Time {
	return f.modTime
}

// IsDir implements os.FileInfo.IsDir
func (f *FileInfo) IsDir() bool {
	return f.isDir
}

// Readdir implements os.FileInfo.Readdir
func (f *FileInfo) Readdir(count int) ([]os.FileInfo, error) {
	return f.files, nil
}

// Sys returns the underlying value.
func (f *FileInfo) Sys() interface{} {
	return f.sys
}

{{ if .Declare }}var {{ .Var }} *FileSystem{{ end }}

func init() {
	{{ .Var }} = &FileSystem{
		files: map[string]File{
			{{range $path, $file := .Files }}"/{{ $path }}": {{ template "file" $file }},{{ end }}
		},
	}
}
`
