/*
Copyright 2021 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/kserve/kserve/pkg/credentials/azure"
)

type AzureProvider struct {
	Downloader AzureDownloader
}

type AzureDownloader interface {
	SetDownloaderValues(modelDir string, modelName string, container string, prefix string)
	GetAllObjects() ([]string, error)
	Download(blobsNames []string) error
	DownloadSingle(blobName string) error
}

var _ Provider = (*AzureProvider)(nil)

func (a *AzureProvider) DownloadModel(modelDir string, modelName string, storageUri string) error {
	// Parse Azure URI
	uriParts, err := parseAzureUri(storageUri)
	if err != nil {
		return fmt.Errorf("failed to parse Azure URI: %v", err)
	}

	if a.Downloader == nil {
		log.Info("Initializing Azure downloader")
		a.Downloader, err = newDefaultDownloader(uriParts.serviceUrl)
		if err != nil {
			return fmt.Errorf("failed to initialize Azure downloader: %v", err)
		}
	}

	a.Downloader.SetDownloaderValues(modelDir, modelName, uriParts.containerName, uriParts.virtualDir)
	blobs, err := a.Downloader.GetAllObjects()
	if err != nil {
		return fmt.Errorf("failed to get blobs: %v", err)
	}
	if len(blobs) == 0 {
		return fmt.Errorf("no blobs found")
	}
	if err := a.Downloader.Download(blobs); err != nil {
		return fmt.Errorf("failed to download blobs: %v", err)
	}
	return nil
}

func newDefaultDownloader(serviceUrl string) (AzureDownloader, error) {
	var err error
	var client *azblob.Client
	if _, ok := os.LookupEnv(azure.AzureClientSecret); ok {
		// Load Azure Credentials from environment variables
		credential, err := azidentity.NewEnvironmentCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to load Azure credentials: %v", err)
		}
		client, err = azblob.NewClient(serviceUrl, credential, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create Azure client with credentials: %v", err)
		}
	} else {
		client, err = azblob.NewClientWithNoCredential(serviceUrl, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create Azure client with no credentials: %v", err)
		}
	}

	return &defaultAzureDownloader{
		client: client,
	}, nil
}

// ParseAzureUri is a method that parses the Azure URI and sets the azureUriParts.
// The Azure URI should follow this specific format: azure://<serviceUrl>/<containerName>/<virtualDir>/.
//
// Here's a breakdown of the URI components:
//   - <serviceUrl>: The base URL of your Azure storage account. For example, 'myStorageAccount.blob.core.windows.net'
//   - <containerName>: The specific container name in the Azure storage account. For example, 'myContainer'
//   - <virtualDir>: Path with virtual directories. For example, 'this/is/virtualDir/'
//
// Putting it all together, an example Azure storageURI would look like this:
// 'azure://myStorageAccount.blob.core.windows.net/myContainer/this/is/virtualDir/'
func parseAzureUri(uri string) (azureUriParts, error) {
	if !strings.HasPrefix(uri, string(AZURE)) {
		return azureUriParts{}, fmt.Errorf("invalid Azure URI: '%s'. The URI must start with '%s'", uri, string(AZURE))
	}

	uri = strings.TrimPrefix(uri, string(AZURE))
	uri = strings.TrimSuffix(uri, "/")
	tokens := strings.SplitN(uri, "/", 3)
	if len(tokens) < 3 {
		return azureUriParts{}, fmt.Errorf("invalid Azure URI: '%s'. The URI must be in the format 'azure://<serviceUrl>/<containerName>/<virtualDir>'", uri)
	}

	serviceUrl := tokens[0]
	containerName := tokens[1]
	virtualDir := ""
	if len(tokens) == 3 {
		virtualDir = tokens[2]
	}

	return azureUriParts{
		serviceUrl:    string(HTTPS) + serviceUrl,
		containerName: containerName,
		virtualDir:    virtualDir,
	}, nil
}

// azureUriParts should follow this specific format: azure://<serviceUrl>/<containerName>/<virtualDir>/
type azureUriParts struct {
	serviceUrl    string
	containerName string
	virtualDir    string
}

type defaultAzureDownloader struct {
	modelDir      string
	modelName     string
	containerName string
	virtualDir    string
	client        *azblob.Client
}

var _ AzureDownloader = (*defaultAzureDownloader)(nil)

func (d *defaultAzureDownloader) SetDownloaderValues(modelDir string, modelName string, container string, prefix string) {
	d.modelDir = modelDir
	d.modelName = modelName
	d.containerName = container
	d.virtualDir = prefix
}

func (d *defaultAzureDownloader) GetAllObjects() ([]string, error) {
	var results []string
	pager := d.client.NewListBlobsFlatPager(d.containerName, &container.ListBlobsFlatOptions{Prefix: &d.virtualDir})
	for pager.More() {
		page, err := pager.NextPage(context.TODO())
		if err != nil {
			return nil, err
		}
		for _, blob := range page.Segment.BlobItems {
			results = append(results, *blob.Name)
		}
	}

	return results, nil
}

func (d *defaultAzureDownloader) Download(blobsNames []string) error {
	for _, blobName := range blobsNames {
		err := d.DownloadSingle(blobName)
		if err != nil {
			return fmt.Errorf("failed to download blob '%v': %v", blobName, err)
		}
	}

	return nil
}

func (d *defaultAzureDownloader) DownloadSingle(blobName string) error {
	relativeFilePath := strings.TrimPrefix(blobName, d.virtualDir)
	filePath := filepath.Join(d.modelDir, d.modelName, relativeFilePath)

	if FileExists(filePath) {
		// File got corrupted or is mid-download :(
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("file is unable to be deleted: %w", err)
		}
	}

	file, err := Create(filePath)
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Error(err, "file is unable to be closed")
		}
	}(file)
	if err != nil {
		return fmt.Errorf("file is already created: %w", err)
	}
	_, err = d.client.DownloadFile(context.TODO(), d.containerName, blobName, file, &azblob.DownloadFileOptions{})
	if err != nil {
		return fmt.Errorf("file is unable to be downloaded: %w", err)
	}
	return nil
}
