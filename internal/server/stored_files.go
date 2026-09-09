package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// os.Root confines file resolution even if a local symlink is inserted into
// storage. Client filenames never participate in this path.
func serveStoredFile(w http.ResponseWriter, r *http.Request, directory, name string) {
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, "/\\") {
		writeError(w, 404, "FILE_NOT_FOUND", "The stored file is unavailable.")
		return
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		writeError(w, 404, "FILE_NOT_FOUND", "The stored file is unavailable.")
		return
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, 404, "FILE_NOT_FOUND", "The stored file is unavailable.")
		return
	}
	file, err := root.Open(name)
	if err != nil {
		writeError(w, 404, "FILE_NOT_FOUND", "The stored file is unavailable.")
		return
	}
	defer file.Close()
	http.ServeContent(w, r, name, info.ModTime(), file)
}
