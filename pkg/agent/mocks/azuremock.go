package mocks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ModelContents = "Model Contents"

type MockAzureDownloader struct {
	modelDir      string
	modelName     string
	containerName string
	prefix        string
}

func (m *MockAzureDownloader) SetDownloaderValues(modelDir string, modelName string, container string, prefix string) {
	m.modelDir = modelDir
	m.modelName = modelName
	m.containerName = container
	m.prefix = prefix
}

func (m *MockAzureDownloader) GetAllObjects() ([]string, error) {
	return []string{
		filepath.Join(m.prefix, "1", "model.onnx"),
		filepath.Join(m.prefix, "config.pbtxt"),
	}, nil
}

func (m *MockAzureDownloader) Download(blobsNames []string) error {
	for _, blobName := range blobsNames {
		err := m.DownloadSingle(blobName)
		if err != nil {
			return fmt.Errorf("failed to download blob '%v': %v", blobName, err)
		}
	}

	return nil
}

func (m *MockAzureDownloader) DownloadSingle(blobName string) error {
	relativeFilePath := strings.TrimPrefix(blobName, m.prefix)
	filePath := filepath.Join(m.modelDir, m.modelName, relativeFilePath)
	err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
	if err != nil {
		return err
	}
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	_, err = file.WriteString(ModelContents)
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

type MockEmptyAzureDownloader struct {
	MockAzureDownloader
}

func (m *MockEmptyAzureDownloader) GetAllObjects() ([]string, error) {
	return []string{}, nil
}
