package filesystem

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"text/template"
)

// New returns an http.FileSystem that wraps the given filesystem. It passes through most calls,
// except if index.html is requested. In that case, the filesystem injects a script to start
// listening on a websocket on the given port. This websocket listens for messages from the server
// and reloads the page when it gets one.
func New(fs http.FileSystem, wsPort int) http.FileSystem {
	return webSocketInjectingFileSystem{
		fs:     fs,
		wsPort: wsPort,
	}
}

const indexHTMLTemplate = `{{ .Contents }}
<script>
	(() => {
		let socket = new WebSocket('ws:localhost:{{ .SocketPort }}');
		socket.addEventListener('message', (event) => {
			location.reload();
		});
	})();
</script>`

var templ = template.Must(template.New("script").Parse(indexHTMLTemplate))

type webSocketInjectingFileSystem struct {
	fs     http.FileSystem
	wsPort int
}

func (fs webSocketInjectingFileSystem) Open(name string) (http.File, error) {
	f, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}

	if !strings.HasSuffix(name, `index.html`) {
		return f, nil
	}

	bs, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(nil)
	err = templ.Execute(buf, struct {
		Contents   string
		SocketPort int
	}{
		Contents:   string(bs),
		SocketPort: fs.wsPort,
	})
	if err != nil {
		return nil, err
	}

	return webSocketInjectingFile{
		File:   f,
		Reader: bytes.NewReader(buf.Bytes()),
	}, nil
}

type webSocketInjectingFile struct {
	http.File
	*bytes.Reader
}

func (f webSocketInjectingFile) Read(p []byte) (int, error) {
	return f.Reader.Read(p)
}

func (f webSocketInjectingFile) Seek(offset int64, whence int) (int64, error) {
	return f.Reader.Seek(offset, whence)
}

type webSocketInjectingFileInfo struct {
	size int64
	fs.FileInfo
}

func (info webSocketInjectingFileInfo) Size() int64 {
	return info.size
}

func (f webSocketInjectingFile) Stat() (fs.FileInfo, error) {
	info, err := f.File.Stat()
	if err != nil {
		return nil, err
	}

	return webSocketInjectingFileInfo{
		FileInfo: info,
		size:     int64(f.Len()),
	}, nil
}
