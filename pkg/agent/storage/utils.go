package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func Create(fileName string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(fileName), 0770); err != nil {
		return nil, err
	}
	return os.Create(fileName)
}

func RemoveDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		err = os.RemoveAll(filepath.Join(dir, name))
		if err != nil {
			return err
		}
	}
	// Remove empty dir
	if err := os.Remove(dir); err != nil {
		return fmt.Errorf("dir is unable to be deleted: %v", err)
	}
	return nil
}
