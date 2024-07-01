package storage

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("parseAzureUri", func() {
	It("should parse valid Azure URI", func() {
		scenarios := []struct {
			uri   string
			parts azureUriParts
		}{
			{
				uri: "azure://myStorageAccount.blob.core.windows.net/myContainer/myVirtualDir",
				parts: azureUriParts{
					serviceUrl:    "https://myStorageAccount.blob.core.windows.net",
					containerName: "myContainer",
					virtualDir:    "myVirtualDir",
				},
			},
			{
				uri: "azure://myStorageAccount.blob.core.windows.net/myContainer/myVirtualDir/",
				parts: azureUriParts{
					serviceUrl:    "https://myStorageAccount.blob.core.windows.net",
					containerName: "myContainer",
					virtualDir:    "myVirtualDir",
				},
			},
			{
				uri: "azure://myStorageAccount.blob.core.windows.net/myContainer/this/is/virtualDir",
				parts: azureUriParts{
					serviceUrl:    "https://myStorageAccount.blob.core.windows.net",
					containerName: "myContainer",
					virtualDir:    "this/is/virtualDir",
				},
			},
			{
				uri: "azure://myStorageAccount.blob.core.windows.net/myContainer/this/is/virtualDir/",
				parts: azureUriParts{
					serviceUrl:    "https://myStorageAccount.blob.core.windows.net",
					containerName: "myContainer",
					virtualDir:    "this/is/virtualDir",
				},
			},
		}

		for _, scenario := range scenarios {
			parts, err := parseAzureUri(scenario.uri)
			Expect(err).To(BeNil())
			Expect(parts).To(Equal(scenario.parts))
		}
	})

	It("should return an error for invalid Azure URI", func() {
		scenarios := []string{
			"invalid-uri",
			"azure://myStorageAccount.blob.core.windows.net",
			"azure://myStorageAccount.blob.core.windows.net/myContainer",
			"azure://myStorageAccount.blob.core.windows.net/myContainer/",
		}

		for _, uri := range scenarios {
			_, err := parseAzureUri(uri)
			Expect(err).To(HaveOccurred())
		}
	})
})

func TestAzureUriParsing(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Azure URI Parsing Suite")
}
