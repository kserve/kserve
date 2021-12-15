package storage

import (
	"io/ioutil"
	"os"
	"path"
	"syscall"
	"testing"

	"github.com/onsi/gomega"
)

func TestCreate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// This would get called in StartPullerAndProcessModels
	syscall.Umask(0)

	tmpDir, _ := ioutil.TempDir("", "test-create-")
	defer os.RemoveAll(tmpDir)

	folderPath := path.Join(tmpDir, "foo")
	filePath := path.Join(folderPath, "bar.txt")
	f, err := Create(filePath)
	defer f.Close()

	g.Expect(err).To(gomega.BeNil())
	g.Expect(folderPath).To(gomega.BeADirectory())

	info, _ := os.Stat(folderPath)
	mode := info.Mode()
	expectedMode := os.FileMode(0777)
	g.Expect(mode.Perm()).To(gomega.Equal(expectedMode))
}
