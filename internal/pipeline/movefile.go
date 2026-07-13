package pipeline

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// moveFile relocates src to dest. It tries a plain rename first, which is
// atomic and cheap on the same filesystem; /watch and /library are
// typically separate Docker volume mounts though, so a plain os.Rename
// commonly fails cross-device. In that case it falls back to copying into
// a temp file alongside dest, renaming into place, then removing src.
func moveFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("creating destination dir: %w", err)
	}

	if err := os.Rename(src, dest); err == nil {
		return nil
	}

	if err := copyFile(src, dest); err != nil {
		return err
	}
	return os.Remove(src)
}

func copyFile(src, dest string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".happysorter-tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	if _, err = io.Copy(tmp, in); err != nil {
		return fmt.Errorf("copying: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err = os.Rename(tmpPath, dest); err != nil {
		return fmt.Errorf("renaming temp file into place: %w", err)
	}
	return nil
}

// uniquePath appends a numeric suffix if dest already exists, so a move
// never silently clobbers a same-named file already in a review folder.
func uniquePath(dest string) string {
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return dest
	}
	ext := filepath.Ext(dest)
	base := dest[:len(dest)-len(ext)]
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
